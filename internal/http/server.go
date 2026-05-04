package http

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"postizer/internal/media"
	"postizer/internal/site"
)

type Server struct {
	store       *site.Store
	media       *media.Store
	contentRoot string
	templates   *template.Template
	auth        authConfig
	mu          sync.RWMutex
}

type ViewData struct {
	Title       string
	Store       *site.Store
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

func New(store *site.Store, mediaStore *media.Store, contentRoot string) http.Handler {
	s := &Server{
		store:       store,
		media:       mediaStore,
		contentRoot: contentRoot,
		auth:        newAuthConfig(contentRoot),
		templates:   loadTemplates(),
	}

	mux := http.NewServeMux()
	mux.Handle("GET /static/", cache(http.StripPrefix("/static/", http.FileServer(http.Dir("web/static")))))
	mux.Handle("GET /media/", cache(http.StripPrefix("/media/", http.FileServer(http.Dir(mediaStore.PublicDir())))))
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

	return timing(securityHeaders(mux))
}

func loadTemplates() *template.Template {
	templates := template.Must(template.New("").Funcs(template.FuncMap{
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
	}).ParseGlob(filepath.Join("web", "templates", "*.html")))
	return template.Must(templates.ParseGlob(filepath.Join("web", "templates", "*.xml")))
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	store := s.currentStore()
	s.render(w, "home.html", ViewData{Title: "Postizer", Store: store, Posts: store.Posts, Pages: store.Pages, Tags: store.Tags, Home: true})
}

func (s *Server) archive(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	s.render(w, "archive.html", ViewData{Title: "Archive", Store: store, Posts: store.Posts, Pages: store.Pages, Tags: store.Tags})
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
	s.render(w, "tags.html", ViewData{Title: "Tags", Store: store, Tags: store.Tags})
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
	s.render(w, "search.html", ViewData{Title: "Search", Store: store, Tags: store.Tags})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if s.isAdmin(r) {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	store := s.currentStore()
	s.render(w, "login.html", ViewData{Title: "Admin Login", Store: store, Remember: true})
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
		s.render(w, "login.html", ViewData{Title: "Admin Login", Store: store, Error: "Invalid username or password", Remember: remember})
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
	s.render(w, "admin_dashboard.html", ViewData{Title: "Dashboard", Store: store, Posts: store.AllPosts, Pages: store.Pages, Tags: store.Tags, Media: s.media.Items(), ActiveAdmin: "dashboard"})
}

func (s *Server) adminEditor(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	s.render(w, "admin.html", ViewData{Title: "Editor", Store: store, Media: s.media.Items(), ActiveAdmin: "editor"})
}

func (s *Server) adminPosts(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	s.render(w, "admin_posts.html", ViewData{Title: "Posts", Store: store, Posts: store.AllPosts, ActiveAdmin: "posts"})
}

func (s *Server) adminMedia(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	s.render(w, "media.html", ViewData{Title: "Media", Store: store, Media: s.media.Items(), ActiveAdmin: "media"})
}

func (s *Server) adminSettings(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	s.render(w, "admin_settings.html", ViewData{Title: "Settings", Store: store, ActiveAdmin: "settings"})
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
	nextStore, err := site.Load(s.contentRoot)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.replaceStore(nextStore)
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
	nextStore, err := site.Load(s.contentRoot)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.replaceStore(nextStore)
	writeJSON(w, settings.HomeImage)
}

func (s *Server) clearHomeImage(w http.ResponseWriter, r *http.Request) {
	settings := s.currentStore().Settings
	settings.HomeImage = site.HomeImage{}
	if err := site.SaveSettings(s.contentRoot, settings); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	nextStore, err := site.Load(s.contentRoot)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.replaceStore(nextStore)
	writeJSON(w, map[string]bool{"enabled": false})
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
	if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
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
