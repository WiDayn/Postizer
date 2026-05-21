package http

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	stdhttp "net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"postizer/pkg/pluginrpc"
)

const (
	contentExportContentType = "application/zip"
	pluginDownloadTTL        = 4 * time.Hour
)

type contentExportSummary struct {
	Path        string
	Filename    string
	ContentType string
	Bytes       int64
	Posts       int
	Pages       int
	MediaItems  int
	MediaFiles  int
	GeneratedAt time.Time
}

func (s *Server) CreateContentExport(context.Context, *pluginrpc.CreateContentExportRequest) (*pluginrpc.CreateContentExportResponse, error) {
	summary, err := s.createContentExport()
	if err != nil {
		return nil, err
	}
	token, err := randomUploadToken()
	if err != nil {
		_ = os.Remove(summary.Path)
		return nil, err
	}
	s.storePluginDownload(token, pluginDownload{
		path:        summary.Path,
		filename:    summary.Filename,
		contentType: summary.ContentType,
		createdAt:   time.Now(),
	})
	return &pluginrpc.CreateContentExportResponse{
		Filename:    summary.Filename,
		ContentType: summary.ContentType,
		DownloadURL: "/admin/api/plugin-downloads/" + token,
		Bytes:       summary.Bytes,
		Posts:       summary.Posts,
		Pages:       summary.Pages,
		MediaItems:  summary.MediaItems,
		MediaFiles:  summary.MediaFiles,
	}, nil
}

func (s *Server) createContentExport() (contentExportSummary, error) {
	now := time.Now().UTC()
	tempFile, err := os.CreateTemp("", "postizer-export-*.zip")
	if err != nil {
		return contentExportSummary{}, err
	}
	summary := contentExportSummary{
		Path:        tempFile.Name(),
		Filename:    "postizer-export-" + now.Format("20060102-150405") + "-utc.zip",
		ContentType: contentExportContentType,
		GeneratedAt: now,
	}
	ok := false
	defer func() {
		if !ok {
			_ = tempFile.Close()
			_ = os.Remove(tempFile.Name())
		}
	}()

	zipWriter := zip.NewWriter(tempFile)
	posts, err := addDirectoryToZip(zipWriter, filepath.Join(s.contentRoot, "posts"), "content/posts")
	if err != nil {
		_ = zipWriter.Close()
		return contentExportSummary{}, err
	}
	summary.Posts = posts

	pages, err := addDirectoryToZip(zipWriter, filepath.Join(s.contentRoot, "pages"), "content/pages")
	if err != nil {
		_ = zipWriter.Close()
		return contentExportSummary{}, err
	}
	summary.Pages = pages

	if s.media != nil {
		summary.MediaItems = len(s.media.Items())
		mediaFiles, err := addDirectoryToZip(zipWriter, s.media.Root(), "media")
		if err != nil {
			_ = zipWriter.Close()
			return contentExportSummary{}, err
		}
		summary.MediaFiles = mediaFiles
	}

	if err := addExportManifest(zipWriter, summary); err != nil {
		_ = zipWriter.Close()
		return contentExportSummary{}, err
	}
	if err := zipWriter.Close(); err != nil {
		return contentExportSummary{}, err
	}
	if err := tempFile.Close(); err != nil {
		return contentExportSummary{}, err
	}
	info, err := os.Stat(summary.Path)
	if err != nil {
		return contentExportSummary{}, err
	}
	summary.Bytes = info.Size()
	ok = true
	return summary, nil
}

func addExportManifest(zipWriter *zip.Writer, summary contentExportSummary) error {
	body, err := json.MarshalIndent(map[string]any{
		"generated_at": summary.GeneratedAt.Format(time.RFC3339),
		"format":       "postizer-content-export-v1",
		"includes": []string{
			"content/posts",
			"content/pages",
			"media",
		},
		"counts": map[string]int{
			"posts":       summary.Posts,
			"pages":       summary.Pages,
			"media_items": summary.MediaItems,
			"media_files": summary.MediaFiles,
		},
	}, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return addBytesToZip(zipWriter, "manifest.json", body, summary.GeneratedAt)
}

func addDirectoryToZip(zipWriter *zip.Writer, root, zipPrefix string) (int, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return 0, nil
	}
	info, err := os.Stat(root)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if !info.IsDir() {
		return 0, fmt.Errorf("export root %s is not a directory", root)
	}

	count := 0
	err = filepath.WalkDir(root, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		relative, err := filepath.Rel(root, filePath)
		if err != nil {
			return err
		}
		zipName := path.Join(zipPrefix, filepath.ToSlash(relative))
		if err := addFileToZip(zipWriter, filePath, zipName, info); err != nil {
			return err
		}
		count++
		return nil
	})
	return count, err
}

func addFileToZip(zipWriter *zip.Writer, filePath, zipName string, info fs.FileInfo) error {
	input, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer input.Close()

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = zipName
	header.Method = zip.Deflate
	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(writer, input)
	return err
}

func addBytesToZip(zipWriter *zip.Writer, zipName string, body []byte, modified time.Time) error {
	header := &zip.FileHeader{
		Name:     zipName,
		Method:   zip.Deflate,
		Modified: modified,
	}
	header.SetMode(0644)
	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = writer.Write(body)
	return err
}

func (s *Server) pluginDownloadFile(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	download, ok := s.pluginDownload(r.PathValue("id"))
	if !ok {
		stdhttp.NotFound(w, r)
		return
	}
	if _, err := os.Stat(download.path); err != nil {
		stdhttp.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", fallbackString(download.contentType, contentExportContentType))
	w.Header().Set("Content-Disposition", `attachment; filename="`+attachmentFilename(download.filename)+`"`)
	w.Header().Set("Cache-Control", "no-store")
	stdhttp.ServeFile(w, r, download.path)
}

func (s *Server) storePluginDownload(token string, download pluginDownload) {
	s.pluginDownloadMu.Lock()
	defer s.pluginDownloadMu.Unlock()
	if s.pluginDownloads == nil {
		s.pluginDownloads = map[string]pluginDownload{}
	}
	for existingToken, existingDownload := range s.pluginDownloads {
		if time.Since(existingDownload.createdAt) <= pluginDownloadTTL {
			continue
		}
		_ = os.Remove(existingDownload.path)
		delete(s.pluginDownloads, existingToken)
	}
	s.pluginDownloads[token] = download
}

func (s *Server) pluginDownload(token string) (pluginDownload, bool) {
	s.pluginDownloadMu.Lock()
	defer s.pluginDownloadMu.Unlock()
	download, ok := s.pluginDownloads[strings.TrimSpace(token)]
	return download, ok
}

func attachmentFilename(filename string) string {
	filename = filepath.Base(strings.TrimSpace(filename))
	if filename == "" || filename == "." || filename == string(filepath.Separator) {
		filename = "postizer-export.zip"
	}
	replacer := strings.NewReplacer(`\`, "-", `"`, "'", "\r", "", "\n", "")
	return replacer.Replace(filename)
}
