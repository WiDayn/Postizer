package http

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"postizer/internal/appearance"
	"postizer/internal/media"
	"postizer/internal/site"
)

type Server struct {
	store               *site.Store
	appearance          *appearance.Catalog
	media               *media.Store
	contentRoot         string
	officialBundlesRoot string
	userContentRoot     string
	userBundlesRoot     string
	templates           *template.Template
	auth                authConfig
	mu                  sync.RWMutex
}

type ViewData struct {
	Title       string
	TitleKey    string
	Store       *site.Store
	Appearance  *appearance.Catalog
	Posts       []*site.Post
	Pages       []*site.Page
	Tags        []*site.Tag
	Post        *site.Post
	Page        *site.Page
	Tag         *site.Tag
	Media       []media.Item
	Home        bool
	ActiveAdmin string
	Error       string
	Remember    bool
}

type authConfig struct {
	user   string
	pass   string
	secret []byte
}

const (
	sessionCookieName       = "postizer_session"
	sessionDuration         = 8 * time.Hour
	rememberSessionDuration = 30 * 24 * time.Hour
)

func New(store *site.Store, mediaStore *media.Store, contentRoot string) (http.Handler, error) {
	s := &Server{
		store:               store,
		media:               mediaStore,
		contentRoot:         contentRoot,
		officialBundlesRoot: "official_bundles",
		userContentRoot:     contentRoot,
		userBundlesRoot:     filepath.Join(contentRoot, "bundles"),
		auth:                newAuthConfig(contentRoot),
	}
	if err := s.reloadRuntime(); err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.Handle("GET /static/", cache(http.StripPrefix("/static/", http.FileServer(http.Dir("web/static")))))
	mux.Handle("GET /media/", cache(http.StripPrefix("/media/", http.FileServer(http.Dir(mediaStore.PublicDir())))))
	mux.Handle("GET /packs/official/bundles/", cache(http.StripPrefix("/packs/official/bundles/", http.FileServer(http.Dir(s.officialBundlesRoot)))))
	mux.Handle("GET /packs/user/bundles/", cache(http.StripPrefix("/packs/user/bundles/", http.FileServer(http.Dir(s.userBundlesRoot)))))
	mux.HandleFunc("GET /", s.home)
	mux.HandleFunc("GET /archive", s.archive)
	mux.HandleFunc("GET /posts/{slug}", s.post)
	mux.HandleFunc("GET /pages/{slug}", s.page)
	mux.HandleFunc("GET /tags", s.tags)
	mux.HandleFunc("GET /tags/{slug}", s.tag)
	mux.HandleFunc("GET /search", s.search)
	mux.HandleFunc("GET /search-index.json", s.searchIndex)
	mux.HandleFunc("GET /feed.xml", s.feed)
	mux.HandleFunc("GET /sitemap.xml", s.sitemap)
	mux.HandleFunc("GET /admin/login", s.login)
	mux.HandleFunc("POST /admin/login", s.loginPost)
	mux.HandleFunc("GET /admin/logout", s.logout)
	mux.Handle("GET /admin", s.requireAdmin(http.HandlerFunc(s.adminDashboard)))
	mux.Handle("GET /admin/editor", s.requireAdmin(http.HandlerFunc(s.adminEditor)))
	mux.Handle("GET /admin/posts", s.requireAdmin(http.HandlerFunc(s.adminPosts)))
	mux.Handle("GET /admin/media", s.requireAdmin(http.HandlerFunc(s.adminMedia)))
	mux.Handle("GET /admin/appearance", s.requireAdmin(http.HandlerFunc(s.adminAppearance)))
	mux.Handle("GET /admin/plugins", s.requireAdmin(http.HandlerFunc(s.adminPlugins)))
	mux.Handle("GET /admin/settings", s.requireAdmin(http.HandlerFunc(s.adminSettings)))
	mux.Handle("GET /admin/api/posts", s.requireAdmin(http.HandlerFunc(s.listPosts)))
	mux.Handle("GET /admin/api/posts/{slug}", s.requireAdmin(http.HandlerFunc(s.getPost)))
	mux.Handle("POST /admin/api/posts", s.requireAdmin(http.HandlerFunc(s.savePost)))
	mux.Handle("POST /admin/api/preview", s.requireAdmin(http.HandlerFunc(s.previewMarkdown)))
	mux.Handle("GET /admin/api/media", s.requireAdmin(http.HandlerFunc(s.listMedia)))
	mux.Handle("POST /admin/api/media", s.requireAdmin(http.HandlerFunc(s.uploadMedia)))
	mux.Handle("PATCH /admin/api/media/{id}", s.requireAdmin(http.HandlerFunc(s.updateMedia)))
	mux.Handle("DELETE /admin/api/media/{id}", s.requireAdmin(http.HandlerFunc(s.deleteMedia)))
	mux.Handle("POST /admin/api/media/paste", s.requireAdmin(http.HandlerFunc(s.uploadMedia)))
	mux.Handle("POST /admin/api/home-image", s.requireAdmin(http.HandlerFunc(s.uploadHomeImage)))
	mux.Handle("DELETE /admin/api/home-image", s.requireAdmin(http.HandlerFunc(s.clearHomeImage)))
	mux.Handle("POST /admin/api/resource-packs", s.requireAdmin(http.HandlerFunc(s.uploadResourcePack)))
	mux.Handle("DELETE /admin/api/resource-packs/{type}/{id}", s.requireAdmin(http.HandlerFunc(s.deleteResourcePack)))
	mux.Handle("POST /admin/api/resource-packs/apply", s.requireAdmin(http.HandlerFunc(s.applyResourcePacks)))

	return timing(securityHeaders(mux)), nil
}

func loadTemplates(activeTheme appearance.Pack) (*template.Template, error) {
	templates, err := template.New("").Funcs(template.FuncMap{
		"date": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.Format("2006-01-02")
		},
		"postURL": func(p *site.Post) string { return "/posts/" + p.Slug },
		"pageURL": func(p *site.Page) string { return "/pages/" + p.Slug },
		"tagURL":  func(t *site.Tag) string { return "/tags/" + t.Slug },
		"mediaFigure": func(item media.Item) string {
			return mediaFigureMarkdown(item)
		},
		"defaultThemePackID": func() string {
			return appearance.DefaultThemePackID
		},
		"defaultThemeLocale": func(catalog *appearance.Catalog) string {
			return defaultThemeLocale(catalog)
		},
		"userPacks": func(catalog *appearance.Catalog) []appearance.Pack {
			return userInstalledPacks(catalog)
		},
		"packInUse": func(pack appearance.Pack, catalog *appearance.Catalog) bool {
			return packInUse(pack, catalog)
		},
		"packTypeMessageKey": func(pack appearance.Pack) string {
			return packTypeMessageKey(pack)
		},
		"localeLabel": func(code string) string {
			return appearance.LocaleLabel(code)
		},
		"msg": func(data ViewData, key string, fallback ...string) string {
			return messageFromViewData(data, key, fallback...)
		},
		"pageTitle": func(data ViewData) string {
			if strings.TrimSpace(data.TitleKey) == "" {
				return data.Title
			}
			return messageFromViewData(data, data.TitleKey, data.Title)
		},
		"toJSON": func(value any) template.HTML {
			body, err := json.Marshal(value)
			if err != nil {
				return template.HTML("{}")
			}
			return template.HTML(body)
		},
		"toScriptJSON": func(value any) template.JS {
			body, err := json.Marshal(value)
			if err != nil {
				return template.JS("{}")
			}
			return template.JS(body)
		},
	}).ParseGlob(filepath.Join("web", "templates", "*.html"))
	if err != nil {
		return nil, err
	}
	templates, err = templates.ParseGlob(filepath.Join("web", "templates", "*.xml"))
	if err != nil {
		return nil, err
	}
	if activeTheme.TemplateDir != "" {
		if err := parseThemeTemplates(templates, activeTheme.TemplateDir); err != nil {
			return nil, err
		}
	}
	return templates, nil
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	store := s.currentStore()
	s.render(w, "home.html", ViewData{Title: "Postizer", TitleKey: "title.home", Store: store, Posts: store.Posts, Pages: store.Pages, Tags: store.Tags, Home: true})
}

func (s *Server) archive(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	s.render(w, "archive.html", ViewData{Title: "Archive", TitleKey: "title.archive", Store: store, Posts: store.Posts, Pages: store.Pages, Tags: store.Tags})
}

func (s *Server) post(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	store := s.currentStore()
	post := store.PostsBySlug[slug]
	if post == nil {
		http.NotFound(w, r)
		return
	}
	s.render(w, "post.html", ViewData{Title: post.Title, Store: store, Post: post, Tags: store.Tags})
}

func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	store := s.currentStore()
	page := store.PagesBySlug[slug]
	if page == nil {
		http.NotFound(w, r)
		return
	}
	s.render(w, "page.html", ViewData{Title: page.Title, Store: store, Page: page, Tags: store.Tags})
}

func (s *Server) tags(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	s.render(w, "tags.html", ViewData{Title: "Tags", TitleKey: "title.tags", Store: store, Tags: store.Tags})
}

func (s *Server) tag(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	store := s.currentStore()
	tag := store.TagsBySlug[slug]
	if tag == nil {
		http.NotFound(w, r)
		return
	}
	s.render(w, "tag.html", ViewData{Title: tag.Title, Store: store, Tag: tag, Tags: store.Tags})
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	s.render(w, "search.html", ViewData{Title: "Search", TitleKey: "title.search", Store: store, Tags: store.Tags})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if s.isAdmin(r) {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	store := s.currentStore()
	s.render(w, "login.html", ViewData{Title: "Admin Login", TitleKey: "title.admin.login", Store: store, Remember: true})
}

func (s *Server) loginPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	user := r.FormValue("username")
	pass := r.FormValue("password")
	remember := r.FormValue("remember") == "on"
	if subtle.ConstantTimeCompare([]byte(user), []byte(s.auth.user)) != 1 ||
		subtle.ConstantTimeCompare([]byte(pass), []byte(s.auth.pass)) != 1 {
		store := s.currentStore()
		w.WriteHeader(http.StatusUnauthorized)
		s.render(w, "login.html", ViewData{Title: "Admin Login", TitleKey: "title.admin.login", Store: store, Error: "Invalid username or password", Remember: remember})
		return
	}
	http.SetCookie(w, s.sessionCookie(user, remember))
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/admin",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}

func (s *Server) adminDashboard(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	s.render(w, "admin_dashboard.html", ViewData{Title: "Dashboard", TitleKey: "title.admin.dashboard", Store: store, Posts: store.AllPosts, Pages: store.Pages, Tags: store.Tags, Media: s.media.Items(), ActiveAdmin: "dashboard"})
}

func (s *Server) adminEditor(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	s.render(w, "admin.html", ViewData{Title: "Editor", TitleKey: "title.admin.editor", Store: store, Media: s.media.Items(), ActiveAdmin: "editor"})
}

func (s *Server) adminPosts(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	s.render(w, "admin_posts.html", ViewData{Title: "Posts", TitleKey: "title.admin.posts", Store: store, Posts: store.AllPosts, ActiveAdmin: "posts"})
}

func (s *Server) adminMedia(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	s.render(w, "media.html", ViewData{Title: "Media", TitleKey: "title.admin.media", Store: store, Media: s.media.Items(), ActiveAdmin: "media"})
}

func (s *Server) adminAppearance(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	s.render(w, "admin_appearance.html", ViewData{Title: "Theme Packs", TitleKey: "title.admin.appearance", Store: store, ActiveAdmin: "appearance"})
}

func (s *Server) adminPlugins(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	s.render(w, "admin_plugins.html", ViewData{Title: "Plugin Packs", TitleKey: "title.admin.plugins", Store: store, ActiveAdmin: "plugins"})
}

func (s *Server) adminSettings(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	s.render(w, "admin_settings.html", ViewData{Title: "Settings", TitleKey: "title.admin.settings", Store: store, ActiveAdmin: "settings"})
}

func (s *Server) listPosts(w http.ResponseWriter, r *http.Request) {
	type postSummary struct {
		Title   string   `json:"title"`
		Slug    string   `json:"slug"`
		Date    string   `json:"date"`
		Updated string   `json:"updated"`
		Tags    []string `json:"tags"`
		Summary string   `json:"summary"`
		Draft   bool     `json:"draft"`
		URL     string   `json:"url"`
	}
	store := s.currentStore()
	posts := make([]postSummary, 0, len(store.AllPosts))
	for _, p := range store.AllPosts {
		posts = append(posts, postSummary{
			Title:   p.Title,
			Slug:    p.Slug,
			Date:    formatDate(p.Date),
			Updated: formatDate(p.Updated),
			Tags:    p.Tags,
			Summary: p.Summary,
			Draft:   p.Draft,
			URL:     "/posts/" + p.Slug,
		})
	}
	writeJSON(w, posts)
}

func (s *Server) getPost(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	store := s.currentStore()
	p := store.AllPostsBySlug[slug]
	if p == nil {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, site.PostDraft{
		Title:   p.Title,
		Slug:    p.Slug,
		Date:    formatDate(p.Date),
		Updated: formatDate(p.Updated),
		Tags:    p.Tags,
		Summary: p.Summary,
		Draft:   p.Draft,
		TOC:     p.TOC,
		Body:    p.Source,
	})
}

func (s *Server) savePost(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	var draft site.PostDraft
	if err := json.NewDecoder(r.Body).Decode(&draft); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	draft.Slug = site.NormalizeSlug(draft.Slug)
	if draft.Slug == "" {
		draft.Slug = site.NormalizeSlug(draft.Title)
	}
	if draft.Date == "" {
		draft.Date = time.Now().Format("2006-01-02")
	}
	if err := site.SavePost(s.contentRoot, draft); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.reloadRuntime(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"slug":  draft.Slug,
		"url":   "/posts/" + draft.Slug,
		"draft": draft.Draft,
	})
}

func (s *Server) previewMarkdown(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	var payload struct {
		Markdown string `json:"markdown"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	html, err := site.RenderMarkdown(payload.Markdown)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]string{"html": string(html)})
}

func (s *Server) listMedia(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.media.Items())
}

func (s *Server) uploadHomeImage(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	item, err := s.media.SaveUpload(file, header.Filename)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	settings := s.currentStore().Settings
	settings.HomeImage = site.HomeImage{
		Enabled: true,
		Src:     item.Path,
		Alt:     item.Alt,
	}
	if err := site.SaveSettings(s.contentRoot, settings); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.reloadRuntime(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, settings.HomeImage)
}

func (s *Server) clearHomeImage(w http.ResponseWriter, r *http.Request) {
	settings := s.currentStore().Settings
	settings.HomeImage = site.HomeImage{}
	if err := site.SaveSettings(s.contentRoot, settings); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.reloadRuntime(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]bool{"enabled": false})
}

func (s *Server) uploadResourcePack(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	body, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	installed, err := appearance.InstallPackZIP(bytes.NewReader(body), int64(len(body)), s.userContentRoot)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.reloadRuntime(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, installed)
}

func (s *Server) deleteResourcePack(w http.ResponseWriter, r *http.Request) {
	packType, ok := parseResourcePackType(r.PathValue("type"))
	if !ok {
		http.Error(w, "invalid resource pack type", http.StatusBadRequest)
		return
	}
	packID := strings.TrimSpace(r.PathValue("id"))

	currentAppearance := s.currentAppearance()
	pack, ok := findUserPack(currentAppearance, packType, packID)
	if !ok {
		http.Error(w, "user resource pack not found", http.StatusNotFound)
		return
	}
	pack.Active = packInUse(pack, currentAppearance)

	settings := s.currentStore().Settings
	nextSettings, changed := settingsAfterDeletingPack(settings, pack, defaultThemeLocale(currentAppearance))
	if changed {
		if err := site.SaveSettings(s.contentRoot, nextSettings); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if err := appearance.DeleteUserPack(s.userContentRoot, packType, packID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.reloadRuntime(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"deleted":          true,
		"pack_id":          pack.ID,
		"pack_type":        pack.Type,
		"settings_changed": changed,
	})
}

func (s *Server) applyResourcePacks(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var payload struct {
		ThemePack struct {
			PackID string `json:"pack_id"`
		} `json:"theme_pack"`
		ThemeLocale string   `json:"theme_locale"`
		PluginOrder []string `json:"plugin_order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	currentAppearance := s.currentAppearance()
	settings := s.currentStore().Settings
	settings.ThemePack = resolvedPackSelection(payload.ThemePack.PackID, appearance.DefaultThemePackID, currentAppearance.Themes)
	settings.PluginOrder = resolvedPluginOrder(payload.PluginOrder, currentAppearance.Plugins)
	settings.ThemeLocale = resolvedThemeLocale(payload.ThemeLocale, settings.ThemePack.PackID, currentAppearance.Themes)
	if err := site.SaveSettings(s.contentRoot, settings); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.reloadRuntime(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	currentAppearance = s.currentAppearance()
	writeJSON(w, map[string]any{
		"theme_pack": map[string]any{
			"enabled": currentAppearance.ThemeSelection.Enabled,
			"pack_id": currentAppearance.ThemeSelection.PackID,
			"active":  currentAppearance.ActiveTheme.ID,
		},
		"theme_locale": currentAppearance.ThemeLocale,
		"plugin_order": currentAppearance.PluginOrder,
	})
}

// resolvedPackSelection 把前端提交的资源包 ID 转换成安全的存储值。
// 如果提交了空值或未知值，会自动回退到默认包，避免 settings 中写入悬空选择。
func resolvedPackSelection(requestedID, defaultID string, packs []appearance.Pack) appearance.Selection {
	requestedID = strings.TrimSpace(requestedID)
	if requestedID == "" {
		requestedID = defaultID
	}
	if !packExists(packs, requestedID) {
		requestedID = defaultID
	}
	return appearance.Selection{
		Enabled: requestedID != defaultID,
		PackID:  requestedID,
	}
}

func resolvedThemeLocale(requestedLocale, themePackID string, themes []appearance.Pack) string {
	requestedLocale = strings.TrimSpace(requestedLocale)
	var themePack *appearance.Pack
	for i := range themes {
		if themes[i].ID != themePackID {
			continue
		}
		themePack = &themes[i]
		break
	}
	if themePack == nil {
		if requestedLocale == "" {
			return "en"
		}
		return requestedLocale
	}
	if requestedLocale == "" {
		return themePack.DefaultLocale
	}
	for _, localeCode := range themePack.Locales {
		if localeCode == requestedLocale {
			return requestedLocale
		}
	}
	return themePack.DefaultLocale
}

func resolvedPluginOrder(requested []string, packs []appearance.Pack) []string {
	available := map[string]bool{}
	for _, pack := range packs {
		available[pack.ID] = true
	}
	var order []string
	seen := map[string]bool{}
	for _, id := range requested {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] || !available[id] {
			continue
		}
		order = append(order, id)
		seen[id] = true
	}
	return order
}

func packExists(packs []appearance.Pack, id string) bool {
	for _, pack := range packs {
		if pack.ID == id {
			return true
		}
	}
	return false
}

// parseResourcePackType 把 URL 中的类型片段转换成 appearance.PackType。
//
// 参数：
// - value: 路由中的 `{type}`，前端会提交 manifest 使用的 bundle/theme/plugin/text。
//
// 返回值：
// - 第一个返回值是规范化后的资源包类型。
// - 第二个返回值表示类型是否受支持；不支持时调用方应返回 400。
func parseResourcePackType(value string) (appearance.PackType, bool) {
	switch appearance.PackType(strings.TrimSpace(value)) {
	case appearance.BundlePack:
		return appearance.BundlePack, true
	case appearance.ThemePack:
		return appearance.ThemePack, true
	case appearance.PluginPack:
		return appearance.PluginPack, true
	case appearance.LegacyTextPack:
		return appearance.LegacyTextPack, true
	default:
		return "", false
	}
}

// userInstalledPacks 返回当前目录中所有由用户本地安装的资源包。
//
// 参数：
// - catalog: 当前运行时外观目录快照。
//
// 返回值：
// - 只包含 SourceUser 的 bundle、独立主题包、独立插件包以及兼容旧版 text 包。
// - bundle 内部的子主题/子插件不会在本地资源包列表重复出现，因为删除单位是父 bundle。
//
// 排序策略：
// 1. 主题包排在插件/文字包前面，符合“样式优先”的设置页阅读顺序。
// 2. 同类型内按名称排序；名称相同时用 ID 保持稳定结果。
func userInstalledPacks(catalog *appearance.Catalog) []appearance.Pack {
	if catalog == nil {
		return nil
	}
	packs := make([]appearance.Pack, 0, len(catalog.Bundles)+len(catalog.Themes)+len(catalog.Plugins))
	for _, pack := range catalog.Bundles {
		if pack.Source == appearance.SourceUser {
			packs = append(packs, pack)
		}
	}
	for _, pack := range catalog.Themes {
		if pack.Source == appearance.SourceUser && pack.BundleID == "" {
			packs = append(packs, pack)
		}
	}
	for _, pack := range catalog.Plugins {
		if pack.Source == appearance.SourceUser && pack.BundleID == "" {
			packs = append(packs, pack)
		}
	}
	sort.Slice(packs, func(i, j int) bool {
		leftRank := resourcePackTypeRank(packs[i].Type)
		rightRank := resourcePackTypeRank(packs[j].Type)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		leftName := strings.ToLower(strings.TrimSpace(packs[i].Name))
		rightName := strings.ToLower(strings.TrimSpace(packs[j].Name))
		if leftName != rightName {
			return leftName < rightName
		}
		return packs[i].ID < packs[j].ID
	})
	return packs
}

func resourcePackTypeRank(packType appearance.PackType) int {
	switch packType {
	case appearance.BundlePack:
		return 0
	case appearance.ThemePack:
		return 1
	case appearance.PluginPack:
		return 2
	case appearance.LegacyTextPack:
		return 3
	default:
		return 4
	}
}

func packTypeMessageKey(pack appearance.Pack) string {
	switch pack.Type {
	case appearance.BundlePack:
		return "settings.local_packs.type.bundle"
	case appearance.ThemePack:
		return "settings.local_packs.type.theme"
	case appearance.PluginPack:
		return "settings.local_packs.type.plugin"
	case appearance.LegacyTextPack:
		return "settings.local_packs.type.text"
	default:
		return "settings.local_packs.type.unknown"
	}
}

// packInUse 判断资源包是否正在参与当前外观。
//
// 参数：
// - pack: 要检查的资源包。
// - catalog: 当前运行时外观目录快照。
//
// 返回值：
// - bundle 内的当前主题或任一启用插件正在使用时返回 true。
// - 主题包 ID 与 ActiveTheme 一致时返回 true。
// - 插件包或旧版 text 包 ID 出现在 PluginOrder 中时返回 true。
func packInUse(pack appearance.Pack, catalog *appearance.Catalog) bool {
	if catalog == nil {
		return false
	}
	switch pack.Type {
	case appearance.BundlePack:
		if catalog.ActiveTheme.BundleID == pack.ID {
			return true
		}
		for _, plugin := range catalog.ActivePlugins {
			if plugin.BundleID == pack.ID {
				return true
			}
		}
	case appearance.ThemePack:
		return pack.ID == catalog.ActiveTheme.ID
	case appearance.PluginPack, appearance.LegacyTextPack:
		for _, id := range catalog.PluginOrder {
			if id == pack.ID {
				return true
			}
		}
	}
	return false
}

// findUserPack 从当前目录快照中查找一个可删除的用户包。
//
// 参数：
// - catalog: 当前运行时外观目录快照。
// - packType: URL 指定的资源包类型。
// - id: URL 指定的资源包 ID。
//
// 返回值：
// - 找到 SourceUser 且类型/ID 都匹配的包时返回该包和 true。
// - 官方包、未知包或类型不匹配时返回 false。
func findUserPack(catalog *appearance.Catalog, packType appearance.PackType, id string) (appearance.Pack, bool) {
	for _, pack := range userInstalledPacks(catalog) {
		if pack.Type == packType && pack.ID == id {
			return pack, true
		}
	}
	return appearance.Pack{}, false
}

// settingsAfterDeletingPack 计算删除资源包后应该写回的设置。
//
// 参数：
// - settings: 当前站点设置。
// - pack: 即将删除的用户资源包。
// - defaultLocale: 默认主题包的默认语言，用于主题包被删除时一并恢复。
//
// 返回值：
// - 第一个返回值是删除后应保存的设置。
// - 第二个返回值表示设置是否发生变化；只有 true 时才需要 SaveSettings。
//
// 行为：
// - 删除正在使用的主题包时，主题恢复到系统默认主题，并把主题语言恢复为默认主题语言。
// - 删除 bundle 时，如果当前主题来自该 bundle，则恢复默认主题；如果启用插件来自该 bundle，则从顺序中移除。
// - 删除正在使用的插件包或旧版 text 包时，从插件启用顺序中移除该 ID。
// - 删除未启用的包时不改变设置。
func settingsAfterDeletingPack(settings site.Settings, pack appearance.Pack, defaultLocale string) (site.Settings, bool) {
	changed := false
	if strings.TrimSpace(defaultLocale) == "" {
		defaultLocale = "en"
	}

	switch pack.Type {
	case appearance.BundlePack:
		if bundleContainsPackID(pack.BundledThemeIDs, settings.ThemePack.PackID) || fallbackActiveBundleWithoutChildren(pack) {
			settings.ThemePack = appearance.Selection{
				Enabled: false,
				PackID:  appearance.DefaultThemePackID,
			}
			settings.ThemeLocale = defaultLocale
			changed = true
		}
		nextOrder, removed := filterPluginOrder(settings.PluginOrder, pack.BundledPluginIDs)
		if removed {
			settings.PluginOrder = nextOrder
			changed = true
		}
	case appearance.ThemePack:
		if settings.ThemePack.PackID == pack.ID {
			settings.ThemePack = appearance.Selection{
				Enabled: false,
				PackID:  appearance.DefaultThemePackID,
			}
			settings.ThemeLocale = defaultLocale
			changed = true
		}
	case appearance.PluginPack, appearance.LegacyTextPack:
		filtered, removed := filterPluginOrder(settings.PluginOrder, []string{pack.ID})
		if removed {
			settings.PluginOrder = filtered
			changed = true
		}
	}
	return settings, changed
}

// bundleContainsPackID 判断一个 bundle 子包 ID 列表中是否包含目标 ID。
//
// 参数：
// - ids: bundle 扫描阶段记录下来的主题或插件 ID 列表。
// - target: 当前设置中保存的主题/插件 ID。
//
// 返回值：
// - 找到完全匹配的 ID 时返回 true；空字符串或未命中返回 false。
func bundleContainsPackID(ids []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

// fallbackActiveBundleWithoutChildren 兼容极旧的 bundle 快照。
//
// 参数：
// - pack: 即将删除的 bundle 包。
//
// 返回值：
// - 当包被标记为 active，但没有记录任何子主题/子插件 ID 时返回 true。
//
// 设计说明：
// 正常情况下 bundle 都会携带 BundledThemeIDs/BundledPluginIDs，删除逻辑可以精确清理。
// 这个兜底分支只用于避免旧运行时快照缺少子 ID 时无法恢复默认主题。
func fallbackActiveBundleWithoutChildren(pack appearance.Pack) bool {
	return pack.Active && len(pack.BundledThemeIDs) == 0 && len(pack.BundledPluginIDs) == 0
}

// filterPluginOrder 从插件启用顺序中移除指定插件 ID。
//
// 参数：
// - order: 当前 settings.PluginOrder。
// - deletedIDs: 即将删除的独立插件 ID，或某个 bundle 内的所有插件 ID。
//
// 返回值：
// - 第一个返回值是过滤后的插件顺序。
// - 第二个返回值表示是否真的移除了至少一个 ID。
func filterPluginOrder(order []string, deletedIDs []string) ([]string, bool) {
	deleted := map[string]bool{}
	for _, id := range deletedIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			deleted[id] = true
		}
	}
	if len(deleted) == 0 {
		return order, false
	}

	filtered := make([]string, 0, len(order))
	removed := false
	for _, id := range order {
		if deleted[id] {
			removed = true
			continue
		}
		filtered = append(filtered, id)
	}
	return filtered, removed
}

func defaultThemeLocale(catalog *appearance.Catalog) string {
	if catalog == nil {
		return "en"
	}
	for _, theme := range catalog.Themes {
		if theme.ID == appearance.DefaultThemePackID {
			return theme.DefaultLocale
		}
	}
	return "en"
}

func (s *Server) uploadMedia(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file field", http.StatusBadRequest)
		return
	}
	defer file.Close()
	item, err := s.media.SaveUpload(file, header.Filename)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"item":     item,
		"markdown": mediaFigureMarkdown(item),
	})
}

func (s *Server) updateMedia(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var payload media.Update
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	item, err := s.media.Update(r.PathValue("id"), payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]any{
		"item":     item,
		"markdown": mediaFigureMarkdown(item),
	})
}

func (s *Server) deleteMedia(w http.ResponseWriter, r *http.Request) {
	if err := s.media.Delete(r.PathValue("id")); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]bool{"deleted": true})
}

func mediaFigureMarkdown(item media.Item) string {
	caption := strings.TrimSpace(item.Caption)
	if caption == "" {
		caption = item.Alt
	}
	labelBase := site.NormalizeSlug(strings.TrimSuffix(item.OriginalName, filepath.Ext(item.OriginalName)))
	if labelBase == "" {
		labelBase = item.ID
	}
	return fmt.Sprintf(
		"\n\n\\begin{figure}\n![%s](%s)\n\\caption{%s}\n\\label{fig:%s}\n\\end{figure}\n\n",
		escapeMarkdownImageAlt(item.Alt),
		item.Path,
		escapeLatexBraceText(caption),
		labelBase,
	)
}

func escapeMarkdownImageAlt(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `]`, `\]`)
}

func escapeLatexBraceText(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `{`, "")
	return strings.ReplaceAll(value, `}`, "")
}

func (s *Server) searchIndex(w http.ResponseWriter, r *http.Request) {
	type doc struct {
		Title   string   `json:"title"`
		URL     string   `json:"url"`
		Summary string   `json:"summary"`
		Tags    []string `json:"tags,omitempty"`
	}
	var docs []doc
	store := s.currentStore()
	for _, p := range store.Posts {
		docs = append(docs, doc{Title: p.Title, URL: "/posts/" + p.Slug, Summary: p.Summary, Tags: p.Tags})
	}
	for _, p := range store.Pages {
		docs = append(docs, doc{Title: p.Title, URL: "/pages/" + p.Slug, Summary: p.Summary})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(docs)
}

func (s *Server) feed(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	store := s.currentStore()
	s.render(w, "feed.xml", ViewData{Title: "Postizer", Store: store, Posts: store.Posts})
}

func (s *Server) sitemap(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	store := s.currentStore()
	s.render(w, "sitemap.xml", ViewData{Title: "Postizer", Store: store, Posts: store.Posts, Pages: store.Pages, Tags: store.Tags})
}

func (s *Server) render(w http.ResponseWriter, name string, data ViewData) {
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
	if data.Store == nil {
		data.Store = s.currentStore()
	}
	if data.Appearance == nil {
		data.Appearance = s.currentAppearance()
	}
	if err := s.currentTemplates().ExecuteTemplate(w, name, data); err != nil {
		log.Printf("template %s: %v", name, err)
	}
}

func (s *Server) currentStore() *site.Store {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.store
}

func (s *Server) replaceStore(store *site.Store) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store = store
}

func (s *Server) currentAppearance() *appearance.Catalog {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.appearance
}

func (s *Server) currentTemplates() *template.Template {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.templates
}

func (s *Server) replaceRuntime(store *site.Store, catalog *appearance.Catalog, templates *template.Template) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store = store
	s.appearance = catalog
	s.templates = templates
}

// reloadRuntime 在设置或资源包发生变化后重新装载站点运行时状态。
//
// 这里把内容数据、资源包目录、模板集合看作一个整体快照统一替换，
// 避免主题切换后出现“内容已更新但模板还是旧的”这类半刷新状态。
func (s *Server) reloadRuntime() error {
	store, err := site.Load(s.contentRoot)
	if err != nil {
		return err
	}
	catalog, err := appearance.LoadCatalog(s.officialBundlesRoot, s.userContentRoot, store.Settings.ThemePack, store.Settings.ThemeLocale, store.Settings.PluginOrder)
	if err != nil {
		return err
	}
	templates, err := loadTemplates(catalog.ActiveTheme)
	if err != nil {
		return err
	}
	s.replaceRuntime(store, &catalog, templates)
	return nil
}

func messageFromViewData(data ViewData, key string, fallback ...string) string {
	if data.Appearance != nil {
		if value := strings.TrimSpace(data.Appearance.Messages[key]); value != "" {
			return value
		}
	}
	if len(fallback) > 0 {
		return fallback[0]
	}
	return key
}

func parseThemeTemplates(templates *template.Template, dir string) error {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".html" || ext == ".xml" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, file := range files {
		if _, err := templates.ParseFiles(file); err != nil {
			return fmt.Errorf("parse theme template %s: %w", file, err)
		}
	}
	return nil
}

func formatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

func newAuthConfig(contentRoot string) authConfig {
	secret := []byte(os.Getenv("POSTIZER_SESSION_SECRET"))
	if len(secret) == 0 {
		secret = localSessionSecret(filepath.Join(contentRoot, ".session_secret"))
	}
	return authConfig{
		user:   env("POSTIZER_ADMIN_USER", "admin"),
		pass:   env("POSTIZER_ADMIN_PASSWORD", "postizer"),
		secret: secret,
	}
}

func localSessionSecret(path string) []byte {
	if body, err := os.ReadFile(path); err == nil {
		secret := strings.TrimSpace(string(body))
		if len(secret) >= 32 {
			return []byte(secret)
		}
	}

	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		log.Printf("session secret fallback: %v", err)
		return []byte(time.Now().Format(time.RFC3339Nano))
	}
	encoded := base64.RawURLEncoding.EncodeToString(secret)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		log.Printf("create session secret dir: %v", err)
		return []byte(encoded)
	}
	if err := os.WriteFile(path, []byte(encoded+"\n"), 0600); err != nil {
		log.Printf("persist session secret: %v", err)
	}
	return []byte(encoded)
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.isAdmin(r) {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/admin/api/") {
			http.Error(w, "admin login required", http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
	})
}

func (s *Server) isAdmin(r *http.Request) bool {
	token := os.Getenv("POSTIZER_ADMIN_TOKEN")
	if token != "" && (r.Header.Get("Authorization") == "Bearer "+token || r.URL.Query().Get("token") == token) {
		return true
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}
	return s.verifySession(cookie.Value)
}

func (s *Server) sessionCookie(user string, remember bool) *http.Cookie {
	duration := sessionDuration
	if remember {
		duration = rememberSessionDuration
	}
	expires := time.Now().Add(duration)
	payload := user + "|" + strconv.FormatInt(expires.Unix(), 10)
	cookie := &http.Cookie{
		Name:     sessionCookieName,
		Value:    signPayload(payload, s.auth.secret),
		Path:     "/admin",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
	if remember {
		cookie.Expires = expires
		cookie.MaxAge = int(duration.Seconds())
	}
	return cookie
}

func (s *Server) verifySession(value string) bool {
	payload, ok := verifySignedPayload(value, s.auth.secret)
	if !ok {
		return false
	}
	user, expRaw, ok := strings.Cut(payload, "|")
	if !ok || subtle.ConstantTimeCompare([]byte(user), []byte(s.auth.user)) != 1 {
		return false
	}
	exp, err := strconv.ParseInt(expRaw, 10, 64)
	if err != nil {
		return false
	}
	return time.Now().Unix() < exp
}

func signPayload(payload string, secret []byte) string {
	payloadB64 := base64.RawURLEncoding.EncodeToString([]byte(payload))
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payloadB64))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return payloadB64 + "." + sig
}

func verifySignedPayload(value string, secret []byte) (string, bool) {
	payloadB64, sigB64, ok := strings.Cut(value, ".")
	if !ok {
		return "", false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payloadB64))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(sigB64), []byte(expected)) != 1 {
		return "", false
	}
	payload, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return "", false
	}
	return string(payload), true
}

func timing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

func cache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		next.ServeHTTP(w, r)
	})
}
