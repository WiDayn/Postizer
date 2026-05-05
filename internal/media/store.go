package media

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	_ "github.com/deepteams/webp"
)

// Item 表示一个已经上传并可对外访问的媒体资源。
type Item struct {
	ID           string    `json:"id"`
	Path         string    `json:"path"`
	OriginalName string    `json:"original_name"`
	Alt          string    `json:"alt"`
	Caption      string    `json:"caption"`
	MIMEType     string    `json:"mime_type"`
	Width        int       `json:"width"`
	Height       int       `json:"height"`
	CreatedAt    time.Time `json:"created_at"`
}

// Update 描述前端允许修改的媒体元数据。
type Update struct {
	OriginalName string `json:"original_name"`
	Alt          string `json:"alt"`
	Caption      string `json:"caption"`
}

// Store 负责管理媒体文件与其元数据索引。
//
// 存储布局：
// - 原文件：{root}/public/{YYYY}/{MM}/{id}{ext}
// - 索引文件：{root}/index.json
type Store struct {
	root      string
	publicDir string
	indexPath string
	mu        sync.RWMutex
	items     []Item
}

// Open 打开一个媒体仓库，如果索引不存在则自动初始化。
func Open(root string) (*Store, error) {
	store := &Store{
		root:      root,
		publicDir: filepath.Join(root, "public"),
		indexPath: filepath.Join(root, "index.json"),
	}
	if err := os.MkdirAll(store.publicDir, 0755); err != nil {
		return nil, err
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

// PublicDir 返回 HTTP 静态文件服务应暴露的公开目录。
func (s *Store) PublicDir() string {
	return s.publicDir
}

// Items 返回按创建时间倒序排列的媒体列表副本。
func (s *Store) Items() []Item {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := append([]Item(nil), s.items...)
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	return items
}

// Item returns one media item by ID.
func (s *Store) Item(id string) (Item, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, item := range s.items {
		if item.ID == id {
			return item, true
		}
	}
	return Item{}, false
}

// SaveUpload 保存用户上传的文件，并生成可直接用于前端渲染的元数据。
func (s *Store) SaveUpload(file io.Reader, originalName string) (Item, error) {
	body, err := io.ReadAll(file)
	if err != nil {
		return Item{}, err
	}
	if len(body) == 0 {
		return Item{}, fmt.Errorf("empty media upload")
	}

	id, err := randomID()
	if err != nil {
		return Item{}, err
	}
	now := time.Now()
	ext := strings.ToLower(filepath.Ext(originalName))
	if ext == "" {
		ext = extensionFromMIME(http.DetectContentType(body))
	}
	if ext == "" {
		ext = ".bin"
	}

	relativeFile := filepath.Join(now.Format("2006"), now.Format("01"), id+ext)
	fullPath := filepath.Join(s.publicDir, relativeFile)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return Item{}, err
	}
	if err := os.WriteFile(fullPath, body, 0644); err != nil {
		return Item{}, err
	}

	item := Item{
		ID:           id,
		Path:         "/" + filepath.ToSlash(filepath.Join("media", relativeFile)),
		OriginalName: normalizeOriginalName(originalName, id+ext),
		Alt:          altFromName(originalName),
		MIMEType:     http.DetectContentType(body),
		CreatedAt:    now,
	}
	if item.MIMEType == "" {
		item.MIMEType = "application/octet-stream"
	}
	if width, height, ok := imageDimensions(body); ok {
		item.Width = width
		item.Height = height
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = append([]Item{item}, s.items...)
	if err := s.saveLocked(); err != nil {
		return Item{}, err
	}
	return item, nil
}

// Update 修改一个媒体项允许暴露给用户编辑的字段。
func (s *Store) Update(id string, payload Update) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.items {
		if s.items[i].ID != id {
			continue
		}
		if strings.TrimSpace(payload.OriginalName) != "" {
			s.items[i].OriginalName = strings.TrimSpace(payload.OriginalName)
		}
		s.items[i].Alt = strings.TrimSpace(payload.Alt)
		s.items[i].Caption = strings.TrimSpace(payload.Caption)
		if err := s.saveLocked(); err != nil {
			return Item{}, err
		}
		return s.items[i], nil
	}
	return Item{}, fmt.Errorf("media item %q not found", id)
}

// Delete 删除媒体项和其底层文件。
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.items {
		if s.items[i].ID != id {
			continue
		}
		item := s.items[i]
		s.items = append(s.items[:i], s.items[i+1:]...)
		if err := s.saveLocked(); err != nil {
			return err
		}
		_ = os.Remove(filepath.Join(s.publicDir, strings.TrimPrefix(item.Path, "/media/")))
		return nil
	}
	return fmt.Errorf("media item %q not found", id)
}

func (s *Store) load() error {
	body, err := os.ReadFile(s.indexPath)
	if os.IsNotExist(err) {
		s.items = nil
		return nil
	}
	if err != nil {
		return err
	}
	if len(body) == 0 {
		s.items = nil
		return nil
	}
	if err := json.Unmarshal(body, &s.items); err != nil {
		return err
	}
	changed, err := s.backfillMissingDimensions()
	if err != nil {
		return err
	}
	if changed {
		return s.saveLocked()
	}
	return nil
}

func (s *Store) backfillMissingDimensions() (bool, error) {
	changed := false
	for index := range s.items {
		item := &s.items[index]
		if item.Width > 0 && item.Height > 0 {
			continue
		}
		if !strings.HasPrefix(item.MIMEType, "image/") {
			continue
		}
		path, ok := s.itemFilePath(*item)
		if !ok {
			continue
		}
		body, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return false, err
		}
		width, height, ok := imageDimensions(body)
		if !ok {
			continue
		}
		item.Width = width
		item.Height = height
		changed = true
	}
	return changed, nil
}

func (s *Store) itemFilePath(item Item) (string, bool) {
	relative := strings.TrimPrefix(strings.TrimSpace(item.Path), "/media/")
	if relative == "" || relative == item.Path {
		return "", false
	}
	cleaned := filepath.Clean(filepath.FromSlash(relative))
	if cleaned == "." || cleaned == ".." || filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.Join(s.publicDir, cleaned), true
}

func imageDimensions(body []byte) (int, int, bool) {
	config, _, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		return 0, 0, false
	}
	return config.Width, config.Height, true
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(s.root, 0755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.indexPath, body, 0644)
}

func normalizeOriginalName(name, fallback string) string {
	name = strings.TrimSpace(filepath.Base(name))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return fallback
	}
	return name
}

func altFromName(name string) string {
	base := strings.TrimSuffix(normalizeOriginalName(name, "image"), filepath.Ext(name))
	base = strings.ReplaceAll(base, "-", " ")
	base = strings.ReplaceAll(base, "_", " ")
	base = strings.TrimSpace(base)
	if base == "" {
		return "Image"
	}
	return base
}

func randomID() (string, error) {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func extensionFromMIME(mime string) string {
	switch mime {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ""
	}
}
