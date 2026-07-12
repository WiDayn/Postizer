package http

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"postizer/internal/appearance"
	"postizer/internal/marketplace"
	"postizer/internal/media"
	"postizer/internal/pluginhost"
	"postizer/internal/site"
	"postizer/pkg/pluginrpc"
)

type Server struct {
	store              *site.Store
	appearance         *appearance.Catalog
	media              *media.Store
	contentRoot        string
	staticRoot         string
	templatesRoot      string
	appRoot            string
	builtinBundlesRoot string
	userContentRoot    string
	userBundlesRoot    string
	templates          *template.Template
	pluginHost         *pluginhost.Host
	pluginUploads      map[string]pluginrpc.ActionFile
	pluginDownloads    map[string]pluginDownload
	pluginJobs         map[string]*importJob
	marketplaceClient  *http.Client
	updateClient       *http.Client
	updateTrigger      func(string) error
	loginAttempts      map[string]loginAttemptState
	auth               authConfig
	mu                 sync.RWMutex
	authMu             sync.RWMutex
	commentMu          sync.RWMutex
	pluginUploadMu     sync.Mutex
	pluginDownloadMu   sync.Mutex
	pluginJobMu        sync.RWMutex
	loginAttemptMu     sync.Mutex
	loginAuditMu       sync.Mutex
}

type pluginDownload struct {
	path        string
	filename    string
	contentType string
	createdAt   time.Time
}

type importJob struct {
	mu       sync.Mutex
	id       string
	status   string
	done     int
	total    int
	errors   int
	logs     []string
	sections []pluginrpc.ResultSection
}

type ViewData struct {
	Title         string
	TitleKey      string
	Store         *site.Store
	Appearance    *appearance.Catalog
	Posts         []*site.Post
	Pages         []*site.Page
	Tags          []*site.Tag
	Comments      []site.Comment
	AdminComments []AdminComment
	Post          *site.Post
	Page          *site.Page
	Tag           *site.Tag
	Media         []media.Item
	Plugin        *appearance.Pack
	PluginUI      *appearance.PluginUI
	PluginResult  *pluginrpc.InvokeActionResponse
	MediaFilters  []MediaTypeFilter
	MediaFilter   string
	Home          bool
	ActiveAdmin   string
	Error         string
	ErrorKey      string
	Remember      bool
	IsAdmin       bool
	NeedsMath     bool
	AdminUsername string
	LoginNotice   LoginNotice
	AppVersion    string
	UpdateLog     []site.UpdateLogEntry
	EditorKind    string
	EditorSlug    string
	Pagination    Pagination
}

// AdminNavGroup 描述后台侧边栏中的一个可折叠二级导航组。
// View 用于模板里继续访问翻译、主题消息等页面上下文；Items 保存子菜单项。
type AdminNavGroup struct {
	View          ViewData
	Key           string
	ControlsID    string
	TitleKey      string
	TitleFallback string
	Active        bool
	Items         []AdminNavItem
}

// AdminNavItem 描述后台二级导航中的一个子菜单链接。
// ActiveAdmin 对应 ViewData.ActiveAdmin（当前后台页面标识），用于计算当前高亮状态。
type AdminNavItem struct {
	ActiveAdmin   string
	URL           string
	LabelKey      string
	LabelFallback string
	Active        bool
}

type AdminComment struct {
	Comment     site.Comment
	PostTitle   string
	PostURL     string
	PostMissing bool
}

type Pagination struct {
	Page       int
	PageSize   int
	Total      int
	TotalPages int
	StartItem  int
	EndItem    int
	PrevURL    string
	NextURL    string
	Pages      []PaginationLink
	Show       bool
}

type PaginationLink struct {
	Page    int
	URL     string
	Current bool
}

type MediaTypeFilter struct {
	ID       string
	LabelKey string
	Label    string
	Count    int
	URL      string
	Active   bool
}

type LoginNotice struct {
	Show             bool
	HasPreviousLogin bool
	PreviousLogin    time.Time
	FailedAttempts   int
	DismissKey       string
}

type authConfig struct {
	user         string
	pass         string
	passwordHash string
	secret       []byte
}

type storedAdminCredentials struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
}

type loginAttemptState struct {
	Failures    int
	FirstFailed time.Time
	LockedUntil time.Time
}

type storedLoginAudit struct {
	LastSuccessfulLogin time.Time        `json:"last_successful_login,omitempty"`
	FailedSinceLogin    int              `json:"failed_since_login"`
	LastNotice          loginAuditNotice `json:"last_notice,omitempty"`
}

type loginAuditNotice struct {
	PreviousLogin  time.Time `json:"previous_login,omitempty"`
	CurrentLogin   time.Time `json:"current_login,omitempty"`
	FailedAttempts int       `json:"failed_attempts"`
}

const (
	sessionCookieName        = "postizer_session"
	sessionDuration          = 8 * time.Hour
	rememberSessionDuration  = 30 * 24 * time.Hour
	adminCredentialsFile     = ".admin_credentials.json"
	adminLoginAuditFile      = ".admin_login_audit.json"
	adminPasswordHashScheme  = "pbkdf2-sha256"
	adminPasswordHashRounds  = 120000
	adminPasswordHashSaltLen = 16
	adminPasswordHashKeyLen  = 32
	maxUpdateLogEntries      = 50
	adminListPageSize        = 20
	maxPluginActionFileBytes = 32 << 20
	maxPluginMediaBytes      = 64 << 20
	loginFailureLimit        = 5
	loginFailureWindow       = 15 * time.Minute
	loginLockoutDuration     = 15 * time.Minute
	pluginSettingsUIOutlet   = "admin.plugin"
	pluginPublicUIOutlet     = "site.page"
)

var AppVersion = "v0.1.11"

func New(store *site.Store, mediaStore *media.Store, contentRoot string, updateTriggers ...func(string) error) (http.Handler, error) {
	appRoot := env("POSTIZER_APP_ROOT", ".")
	s := &Server{
		store:              store,
		media:              mediaStore,
		contentRoot:        contentRoot,
		staticRoot:         env("POSTIZER_STATIC_ROOT", filepath.Join("web", "static")),
		templatesRoot:      env("POSTIZER_TEMPLATES_ROOT", filepath.Join("web", "templates")),
		appRoot:            appRoot,
		builtinBundlesRoot: env("POSTIZER_BUILTIN_BUNDLES_ROOT", filepath.Join("internal", "bundles")),
		userContentRoot:    contentRoot,
		userBundlesRoot:    filepath.Join(contentRoot, "bundles"),
		pluginUploads:      map[string]pluginrpc.ActionFile{},
		pluginDownloads:    map[string]pluginDownload{},
		pluginJobs:         map[string]*importJob{},
		marketplaceClient:  &http.Client{Timeout: 45 * time.Second},
		updateClient:       &http.Client{Timeout: 20 * time.Second},
		loginAttempts:      map[string]loginAttemptState{},
		auth:               newAuthConfig(contentRoot),
	}
	if len(updateTriggers) > 0 {
		s.updateTrigger = updateTriggers[0]
	}
	s.pluginHost = pluginhost.New(appRoot, s)
	s.pluginHost.SetDataRoot(filepath.Join(contentRoot, "plugin-data"))
	if err := s.reloadRuntime(); err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.Handle("GET /static/", staticAssets(http.StripPrefix("/static/", http.FileServer(http.Dir(s.staticRoot)))))
	mux.Handle("GET /media/", staticAssets(http.StripPrefix("/media/", http.FileServer(http.Dir(mediaStore.PublicDir())))))
	mux.Handle("GET /packs/official/bundles/", staticAssets(http.StripPrefix("/packs/official/bundles/", http.FileServer(http.Dir(s.builtinBundlesRoot)))))
	mux.Handle("GET /packs/user/bundles/", staticAssets(http.StripPrefix("/packs/user/bundles/", http.FileServer(http.Dir(s.userBundlesRoot)))))
	mux.Handle("GET /marketplace/previews/", staticAssets(http.StripPrefix("/marketplace/previews/", http.FileServer(http.Dir(filepath.Join(s.appRoot, "marketplace", "previews"))))))
	mux.HandleFunc("GET /", s.home)
	mux.HandleFunc("GET /archive", s.archive)
	mux.HandleFunc("GET /posts/{slug}", s.post)
	mux.HandleFunc("POST /comments", s.submitComment)
	mux.HandleFunc("GET /pages/{slug}", s.page)
	mux.HandleFunc("GET /tags", s.tags)
	mux.HandleFunc("GET /tags/{slug}", s.tag)
	mux.HandleFunc("GET /search", s.search)
	mux.HandleFunc("GET /search-index.json", s.searchIndex)
	mux.HandleFunc("GET /plugins/{id}", s.publicPluginPage)
	mux.HandleFunc("GET /feed.xml", s.feed)
	mux.HandleFunc("GET /sitemap.xml", s.sitemap)
	mux.HandleFunc("GET /admin/login", s.login)
	mux.HandleFunc("POST /admin/login", s.loginPost)
	mux.HandleFunc("GET /admin/logout", s.logout)
	mux.Handle("GET /admin", s.requireAdmin(http.HandlerFunc(s.adminDashboard)))
	mux.Handle("GET /admin/editor", s.requireAdmin(http.HandlerFunc(s.adminEditor)))
	mux.Handle("GET /admin/posts", s.requireAdmin(http.HandlerFunc(s.adminPosts)))
	mux.Handle("GET /admin/posts/new", s.requireAdmin(http.HandlerFunc(s.adminNewPost)))
	mux.Handle("GET /admin/posts/{slug}/edit", s.requireAdmin(http.HandlerFunc(s.adminEditPost)))
	mux.Handle("POST /admin/posts/{slug}/delete", s.requireAdmin(http.HandlerFunc(s.deletePostAndRedirect)))
	mux.Handle("GET /admin/pages", s.requireAdmin(http.HandlerFunc(s.adminPages)))
	mux.Handle("GET /admin/pages/new", s.requireAdmin(http.HandlerFunc(s.adminNewPage)))
	mux.Handle("GET /admin/pages/{slug}/edit", s.requireAdmin(http.HandlerFunc(s.adminEditPage)))
	mux.Handle("POST /admin/pages/{slug}/delete", s.requireAdmin(http.HandlerFunc(s.deletePageAndRedirect)))
	mux.Handle("GET /admin/comments", s.requireAdmin(http.HandlerFunc(s.adminComments)))
	mux.Handle("POST /admin/comments/{id}/reply", s.requireAdmin(http.HandlerFunc(s.replyToComment)))
	mux.Handle("GET /admin/media", s.requireAdmin(http.HandlerFunc(s.adminMedia)))
	mux.Handle("GET /admin/appearance", s.requireAdmin(http.HandlerFunc(s.adminAppearance)))
	mux.Handle("GET /admin/resource-marketplace", s.requireAdmin(http.HandlerFunc(s.adminResourceMarketplace)))
	mux.Handle("GET /admin/plugins", s.requireAdmin(http.HandlerFunc(s.adminPlugins)))
	mux.Handle("GET /admin/plugins/{id}", s.requireAdmin(http.HandlerFunc(s.adminPluginSettings)))
	mux.Handle("GET /admin/menus", s.requireAdmin(http.HandlerFunc(s.adminMenus)))
	mux.Handle("GET /admin/sidebars", s.requireAdmin(http.HandlerFunc(s.adminSidebars)))
	mux.Handle("GET /admin/theme-settings", s.requireAdmin(http.HandlerFunc(s.adminThemeSettings)))
	mux.Handle("GET /admin/settings", s.requireAdmin(http.HandlerFunc(s.adminSettings)))
	mux.Handle("GET /admin/settings/permalinks", s.requireAdmin(http.HandlerFunc(s.adminPermalinks)))
	mux.Handle("GET /admin/settings/updates", s.requireAdmin(http.HandlerFunc(s.adminUpdateSettings)))
	mux.Handle("GET /admin/api/posts", s.requireAdmin(http.HandlerFunc(s.listPosts)))
	mux.Handle("GET /admin/api/posts/{slug}", s.requireAdmin(http.HandlerFunc(s.getPost)))
	mux.Handle("POST /admin/api/posts", s.requireAdmin(http.HandlerFunc(s.savePost)))
	mux.Handle("DELETE /admin/api/posts/{slug}", s.requireAdmin(http.HandlerFunc(s.deletePost)))
	mux.Handle("GET /admin/api/pages", s.requireAdmin(http.HandlerFunc(s.listPages)))
	mux.Handle("GET /admin/api/pages/{slug}", s.requireAdmin(http.HandlerFunc(s.getPage)))
	mux.Handle("POST /admin/api/pages", s.requireAdmin(http.HandlerFunc(s.savePage)))
	mux.Handle("DELETE /admin/api/pages/{slug}", s.requireAdmin(http.HandlerFunc(s.deletePage)))
	mux.Handle("POST /admin/api/preview", s.requireAdmin(http.HandlerFunc(s.previewMarkdown)))
	mux.Handle("GET /admin/api/media", s.requireAdmin(http.HandlerFunc(s.listMedia)))
	mux.Handle("POST /admin/api/media", s.requireAdmin(http.HandlerFunc(s.uploadMedia)))
	mux.Handle("PATCH /admin/api/media/{id}", s.requireAdmin(http.HandlerFunc(s.updateMedia)))
	mux.Handle("DELETE /admin/api/media/{id}", s.requireAdmin(http.HandlerFunc(s.deleteMedia)))
	mux.Handle("POST /admin/api/media/paste", s.requireAdmin(http.HandlerFunc(s.uploadMedia)))
	mux.Handle("POST /admin/api/home-image", s.requireAdmin(http.HandlerFunc(s.uploadHomeImage)))
	mux.Handle("DELETE /admin/api/home-image", s.requireAdmin(http.HandlerFunc(s.clearHomeImage)))
	mux.Handle("POST /admin/api/settings/site-title", s.requireAdmin(http.HandlerFunc(s.updateSiteTitleSettings)))
	mux.Handle("POST /admin/api/settings/permalinks", s.requireAdmin(http.HandlerFunc(s.updatePermalinkSettings)))
	mux.Handle("POST /admin/api/settings/auto-update", s.requireAdmin(http.HandlerFunc(s.updateAutoUpdateSettings)))
	mux.Handle("GET /admin/api/updates/check", s.requireAdmin(http.HandlerFunc(s.checkForUpdates)))
	mux.Handle("POST /admin/api/settings/updates/check", s.requireAdmin(http.HandlerFunc(s.checkForUpdates)))
	mux.Handle("POST /admin/api/settings/updates/apply", s.requireAdmin(http.HandlerFunc(s.applyUpdate)))
	mux.Handle("POST /admin/api/settings/comments", s.requireAdmin(http.HandlerFunc(s.updateCommentSettings)))
	mux.Handle("POST /admin/api/settings/home-page", s.requireAdmin(http.HandlerFunc(s.updateHomePageSettings)))
	mux.Handle("POST /admin/api/settings/admin-account", s.requireAdmin(http.HandlerFunc(s.updateAdminAccountSettings)))
	mux.Handle("POST /admin/api/settings/time-zone", s.requireAdmin(http.HandlerFunc(s.updateTimeZoneSettings)))
	mux.Handle("POST /admin/api/settings/media-processing", s.requireAdmin(http.HandlerFunc(s.updateMediaProcessingSettings)))
	mux.Handle("POST /admin/api/menus", s.requireAdmin(http.HandlerFunc(s.updateMenus)))
	mux.Handle("POST /admin/api/theme-settings", s.requireAdmin(http.HandlerFunc(s.updateThemeSettings)))
	mux.Handle("GET /admin/api/resource-marketplace", s.requireAdmin(http.HandlerFunc(s.resourceMarketplace)))
	mux.Handle("POST /admin/api/resource-marketplace/install", s.requireAdmin(http.HandlerFunc(s.installResourceMarketplaceItem)))
	mux.Handle("POST /admin/api/resource-packs", s.requireAdmin(http.HandlerFunc(s.uploadResourcePack)))
	mux.Handle("DELETE /admin/api/resource-packs/{type}/{id}", s.requireAdmin(http.HandlerFunc(s.deleteResourcePack)))
	mux.Handle("POST /admin/api/resource-packs/apply", s.requireAdmin(http.HandlerFunc(s.applyResourcePacks)))
	mux.Handle("POST /admin/api/plugins/{id}/actions/{action}", s.requireAdmin(http.HandlerFunc(s.invokePluginAction)))
	mux.Handle("GET /admin/api/plugin-jobs/{id}", s.requireAdmin(http.HandlerFunc(s.pluginJobStatus)))
	mux.Handle("GET /admin/api/plugin-downloads/{id}", s.requireAdmin(http.HandlerFunc(s.pluginDownloadFile)))

	return timing(securityHeaders(mux)), nil
}

func loadTemplates(activeTheme appearance.Pack) (*template.Template, error) {
	return loadTemplatesFromRoot(activeTheme, env("POSTIZER_TEMPLATES_ROOT", filepath.Join("web", "templates")))
}

func (s *Server) loadTemplates(activeTheme appearance.Pack) (*template.Template, error) {
	templatesRoot := s.templatesRoot
	if templatesRoot == "" {
		templatesRoot = env("POSTIZER_TEMPLATES_ROOT", filepath.Join("web", "templates"))
	}
	return loadTemplatesFromRoot(activeTheme, templatesRoot)
}

func loadTemplatesFromRoot(activeTheme appearance.Pack, templatesRoot string) (*template.Template, error) {
	templates, err := template.New("").Funcs(template.FuncMap{
		"date": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return site.FormatDisplayTime(t)
		},
		"viewDate": func(data ViewData, t time.Time) string {
			if t.IsZero() {
				return ""
			}
			if data.Store != nil {
				t = t.In(site.TimeLocation(data.Store.Settings))
			}
			return site.FormatDisplayTime(t)
		},
		"postURL": func(p *site.Post) string { return site.PostURL(site.Settings{}, p) },
		"pageURL": func(p *site.Page) string { return site.PageURL(site.Settings{}, p) },
		"tagURL":  func(t *site.Tag) string { return site.TagURL(site.Settings{}, t) },
		"tagSlugURL": func(data ViewData, slug string) string {
			if data.Store == nil {
				return site.TagSlugURL(site.Settings{}, slug)
			}
			if tag := data.Store.TagsBySlug[strings.TrimSpace(slug)]; tag != nil {
				return site.TagURL(data.Store.Settings, tag)
			}
			return site.TagSlugURL(data.Store.Settings, slug)
		},
		"mediaFigure": func(item media.Item) string {
			return mediaFigureMarkdown(item)
		},
		"mediaIsImage": func(item media.Item) bool {
			return mediaIsImage(item)
		},
		"mediaFileLabel": func(item media.Item) string {
			return mediaFileLabel(item)
		},
		"mediaFileType": func(item media.Item) string {
			return mediaFileType(item)
		},
		"mediaMeta": func(item media.Item) string {
			return mediaMeta(item)
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
		"pluginTools": func(catalog *appearance.Catalog) []appearance.Pack {
			return pluginTools(catalog)
		},
		"hasPluginSettings": func(pack appearance.Pack) bool {
			return hasUIOutlet(pack, pluginSettingsUIOutlet)
		},
		"hasPublicPluginPage": func(pack appearance.Pack) bool {
			return hasUIOutlet(pack, pluginPublicUIOutlet)
		},
		"pluginAction": func(ui *appearance.PluginUI, id string) appearance.PluginUIAction {
			return pluginAction(ui, id)
		},
		"localeLabel": func(code string) string {
			return appearance.LocaleLabel(code)
		},
		"timeZones": func(current string) []string {
			return site.TimeZoneOptions(current)
		},
		"siteMainTitle": func(data ViewData) string {
			return siteMainTitle(data)
		},
		"siteSubtitle": func(data ViewData) string {
			return siteSubtitle(data)
		},
		"siteEditionLine": func(data ViewData) string {
			return siteEditionLine(data)
		},
		"siteFeedDescription": func(data ViewData) string {
			return siteFeedDescription(data)
		},
		"menuLinks": func(data ViewData, location string) []site.MenuLink {
			return menuLinksForLocation(data, location)
		},
		"sidebarSections": func(data ViewData) []site.SidebarLinkSection {
			if data.Store == nil {
				return nil
			}
			return data.Store.SidebarSectionsWithLabels(sidebarLabelsFromViewData(data))
		},
		"menuAdminData": func(data ViewData) any {
			return menuAdminData(data)
		},
		"themeSettingsData": func(data ViewData) any {
			return themeSettingsData(data)
		},
		"adminThemeNavGroup": func(data ViewData) AdminNavGroup {
			return adminThemeNavGroup(data)
		},
		"adminCustomNavGroup": func(data ViewData) AdminNavGroup {
			return adminCustomNavGroup(data)
		},
		"adminSettingsNavGroup": func(data ViewData) AdminNavGroup {
			return adminSettingsNavGroup(data)
		},
		"msg": func(data ViewData, key string, fallback ...string) string {
			return messageFromViewData(data, key, fallback...)
		},
		"pageTitle": func(data ViewData) string {
			return pageTitleFromViewData(data)
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
	}).ParseGlob(filepath.Join(templatesRoot, "*.html"))
	if err != nil {
		return nil, err
	}
	templates, err = templates.ParseGlob(filepath.Join(templatesRoot, "*.xml"))
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
		if s.renderPublicPermalink(w, r) {
			return
		}
		http.NotFound(w, r)
		return
	}
	store := s.currentStore()
	posts, pagination := paginateItems(r, store.Posts, store.Settings.HomePage.PageSize)
	s.render(w, "home.html", ViewData{Title: "Postizer", TitleKey: "title.home", Store: store, Posts: posts, Pages: store.Pages, Tags: store.Tags, Home: true, Pagination: pagination})
}

func (s *Server) archive(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	s.render(w, "archive.html", ViewData{Title: "Archive", TitleKey: "title.archive", Store: store, Posts: store.Posts, Pages: store.Pages, Tags: store.Tags})
}

func (s *Server) publicPluginPage(w http.ResponseWriter, r *http.Request) {
	pack, ok := s.findPlugin(r.PathValue("id"))
	if !ok || !hasUIOutlet(pack, pluginPublicUIOutlet) {
		http.NotFound(w, r)
		return
	}
	ui, err := pluginUI(pack, pluginPublicUIOutlet)
	if err != nil || len(ui.Pages) == 0 || len(ui.Pages[0].Actions) == 0 {
		if err == nil {
			err = errors.New("public plugin page does not declare a render action")
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fields := map[string]string{}
	for key, values := range r.URL.Query() {
		if len(values) > 0 {
			fields[key] = values[len(values)-1]
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	result, err := s.pluginHost.InvokeAction(ctx, pack, &pluginrpc.InvokeActionRequest{
		PluginID: pack.ID,
		ActionID: ui.Pages[0].Actions[0],
		Fields:   fields,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	store := s.currentStore()
	s.render(w, "plugin_public.html", ViewData{
		Title:        ui.Pages[0].Title,
		Store:        store,
		Plugin:       &pack,
		PluginUI:     ui,
		PluginResult: result,
	})
}

func (s *Server) post(w http.ResponseWriter, r *http.Request) {
	if s.renderPublicPermalink(w, r) {
		return
	}
	slug := r.PathValue("slug")
	store := s.currentStore()
	post := store.PostsBySlug[slug]
	if post == nil {
		http.NotFound(w, r)
		return
	}
	s.renderPost(w, r, post, store)
}

func (s *Server) renderPost(w http.ResponseWriter, r *http.Request, post *site.Post, store *site.Store) {
	comments, err := s.commentsForPost(post.Slug)
	if err != nil {
		log.Printf("load comments for %s: %v", post.Slug, err)
	}
	s.render(w, "post.html", ViewData{Title: post.Title, Store: store, Post: post, Tags: store.Tags, Comments: comments, IsAdmin: s.isAdmin(r)})
}

func (s *Server) commentsForPost(slug string) ([]site.Comment, error) {
	s.commentMu.RLock()
	defer s.commentMu.RUnlock()
	return site.CommentsForPost(s.contentRoot, slug)
}

func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	if s.renderPublicPermalink(w, r) {
		return
	}
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
	if s.renderPublicPermalink(w, r) {
		return
	}
	slug := r.PathValue("slug")
	store := s.currentStore()
	tag := store.TagsBySlug[slug]
	if tag == nil {
		http.NotFound(w, r)
		return
	}
	s.render(w, "tag.html", ViewData{Title: tag.Title, Store: store, Tag: tag, Tags: store.Tags})
}

func (s *Server) renderPublicPermalink(w http.ResponseWriter, r *http.Request) bool {
	store := s.currentStore()
	if post := store.PostByPermalink(r.URL.Path); post != nil {
		s.renderPost(w, r, post, store)
		return true
	}
	if page := store.PageByPermalink(r.URL.Path); page != nil {
		s.render(w, "page.html", ViewData{Title: page.Title, Store: store, Page: page, Tags: store.Tags})
		return true
	}
	if tag := store.TagByPermalink(r.URL.Path); tag != nil {
		s.render(w, "tag.html", ViewData{Title: tag.Title, Store: store, Tag: tag, Tags: store.Tags})
		return true
	}
	return false
}

func (s *Server) submitComment(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	if !store.Settings.Comments.Enabled {
		http.Error(w, "comments are disabled", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(r.FormValue("website")) != "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	input := site.CommentInput{
		PostSlug: r.FormValue("post_slug"),
		Nickname: r.FormValue("nickname"),
		Email:    r.FormValue("email"),
		Body:     r.FormValue("comment"),
	}
	post := store.PostsBySlug[strings.TrimSpace(input.PostSlug)]
	if post == nil {
		http.NotFound(w, r)
		return
	}
	s.commentMu.Lock()
	_, err := site.AddComment(s.contentRoot, input, time.Now().In(site.TimeLocation(store.Settings)))
	s.commentMu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, site.PostURL(store.Settings, post)+"#comments", http.StatusSeeOther)
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	s.render(w, "search.html", ViewData{Title: "Search", TitleKey: "title.search", Store: store, Tags: store.Tags})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if s.isAdmin(r) {
		s.mirrorAdminSessionCookie(w, r)
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	store := s.currentStore()
	s.render(w, "login.html", ViewData{Title: "Admin Login", TitleKey: "title.admin.login", Store: store, Remember: true})
}

func (s *Server) loginPost(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	user := r.FormValue("username")
	pass := r.FormValue("password")
	remember := r.FormValue("remember") == "on"
	now := s.loginAuditTime()
	rateKey := loginRateLimitKey(r)
	if retryAfter := s.loginRetryAfter(rateKey, now); retryAfter > 0 {
		s.recordLoginFailureAudit(now)
		writeRetryAfter(w, retryAfter)
		s.renderLoginError(w, remember, http.StatusTooManyRequests, "login.error.rate_limited", "Too many login attempts. Try again later.")
		return
	}
	auth := s.authSnapshot()
	if subtle.ConstantTimeCompare([]byte(user), []byte(auth.user)) != 1 || !auth.verifyPassword(pass) {
		retryAfter := s.recordLoginFailure(rateKey, now)
		s.recordLoginFailureAudit(now)
		if retryAfter > 0 {
			writeRetryAfter(w, retryAfter)
			s.renderLoginError(w, remember, http.StatusTooManyRequests, "login.error.rate_limited", "Too many login attempts. Try again later.")
			return
		}
		s.renderLoginError(w, remember, http.StatusUnauthorized, "login.error.invalid", "Invalid username or password")
		return
	}
	s.clearLoginFailures(rateKey)
	s.recordLoginSuccessAudit(now)
	http.SetCookie(w, s.sessionCookie(auth.user, remember))
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (s *Server) renderLoginError(w http.ResponseWriter, remember bool, status int, errorKey, fallback string) {
	store := s.currentStore()
	w.WriteHeader(status)
	s.render(w, "login.html", ViewData{
		Title:    "Admin Login",
		TitleKey: "title.admin.login",
		Store:    store,
		Error:    fallback,
		ErrorKey: errorKey,
		Remember: remember,
	})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	for _, path := range []string{"/", "/admin"} {
		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    "",
			Path:     path,
			Expires:  time.Unix(0, 0),
			MaxAge:   -1,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
	}
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}

func (s *Server) adminDashboard(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	s.render(w, "admin_dashboard.html", ViewData{Title: "Dashboard", TitleKey: "title.admin.dashboard", Store: store, Posts: store.AllPosts, Pages: store.AllPages, Tags: store.Tags, Media: s.media.Items(), ActiveAdmin: "dashboard"})
}

func (s *Server) adminEditor(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin/posts/new", http.StatusSeeOther)
}

func (s *Server) adminNewPost(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	s.render(w, "admin.html", ViewData{Title: "Editor", TitleKey: "title.admin.editor", Store: store, Media: s.media.Items(), ActiveAdmin: "posts", EditorKind: "post"})
}

func (s *Server) adminEditPost(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	store := s.currentStore()
	if store.AllPostsBySlug[slug] == nil {
		http.NotFound(w, r)
		return
	}
	s.render(w, "admin.html", ViewData{Title: "Editor", TitleKey: "title.admin.editor", Store: store, Media: s.media.Items(), ActiveAdmin: "posts", EditorKind: "post", EditorSlug: slug})
}

func (s *Server) adminPosts(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	posts, pagination := paginateItems(r, store.AllPosts, adminListPageSize)
	s.render(w, "admin_posts.html", ViewData{Title: "Posts", TitleKey: "title.admin.posts", Store: store, Posts: posts, Pagination: pagination, ActiveAdmin: "posts"})
}

func (s *Server) adminPages(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	pages, pagination := paginateItems(r, store.AllPages, adminListPageSize)
	s.render(w, "admin_pages.html", ViewData{Title: "Pages", TitleKey: "title.admin.pages", Store: store, Pages: pages, Pagination: pagination, ActiveAdmin: "pages"})
}

func (s *Server) adminComments(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	s.commentMu.RLock()
	comments, err := site.LoadComments(s.contentRoot)
	s.commentMu.RUnlock()
	if err != nil {
		s.render(w, "admin_comments.html", ViewData{Title: "Comments", TitleKey: "title.admin.comments", Store: store, ActiveAdmin: "comments", Error: err.Error()})
		return
	}
	site.SortCommentsNewestFirst(comments)
	pageComments, pagination := paginateItems(r, comments, adminListPageSize)
	s.render(w, "admin_comments.html", ViewData{
		Title:         "Comments",
		TitleKey:      "title.admin.comments",
		Store:         store,
		AdminComments: adminCommentsForStore(store, pageComments),
		Pagination:    pagination,
		ActiveAdmin:   "comments",
	})
}

func adminCommentsForStore(store *site.Store, comments []site.Comment) []AdminComment {
	items := make([]AdminComment, 0, len(comments))
	for _, comment := range comments {
		item := AdminComment{
			Comment:   comment,
			PostTitle: comment.PostSlug,
			PostURL:   "/posts/" + comment.PostSlug,
		}
		if store != nil {
			if post := store.AllPostsBySlug[comment.PostSlug]; post != nil {
				item.PostTitle = post.Title
				item.PostURL = site.PostURL(store.Settings, post)
			} else {
				item.PostMissing = true
			}
		}
		items = append(items, item)
	}
	return items
}

func (s *Server) adminNewPage(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	s.render(w, "admin.html", ViewData{Title: "Editor", TitleKey: "title.admin.editor", Store: store, Media: s.media.Items(), ActiveAdmin: "pages", EditorKind: "page"})
}

func (s *Server) adminEditPage(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	store := s.currentStore()
	if store.AllPagesBySlug[slug] == nil {
		http.NotFound(w, r)
		return
	}
	s.render(w, "admin.html", ViewData{Title: "Editor", TitleKey: "title.admin.editor", Store: store, Media: s.media.Items(), ActiveAdmin: "pages", EditorKind: "page", EditorSlug: slug})
}

func (s *Server) adminMedia(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	allItems := s.media.Items()
	activeFilter := normalizeMediaTypeFilter(r.URL.Query().Get("type"))
	filteredItems := filterMediaItemsByType(allItems, activeFilter)
	items, pagination := paginateItems(r, filteredItems, adminListPageSize)
	s.render(w, "media.html", ViewData{
		Title:        "Media",
		TitleKey:     "title.admin.media",
		Store:        store,
		Media:        items,
		MediaFilters: mediaTypeFilters(r, allItems, activeFilter),
		MediaFilter:  activeFilter,
		Pagination:   pagination,
		ActiveAdmin:  "media",
	})
}

func (s *Server) adminAppearance(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	s.render(w, "admin_appearance.html", ViewData{Title: "Theme Packs", TitleKey: "title.admin.appearance", Store: store, ActiveAdmin: "appearance"})
}

func (s *Server) adminResourceMarketplace(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	s.render(w, "admin_resource_marketplace.html", ViewData{Title: "Resource Marketplace", TitleKey: "title.admin.resource_marketplace", Store: store, ActiveAdmin: "resource-marketplace"})
}

func (s *Server) adminPlugins(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	s.render(w, "admin_plugins.html", ViewData{Title: "Plugin Packs", TitleKey: "title.admin.plugins", Store: store, ActiveAdmin: "plugins"})
}

func paginateItems[T any](r *http.Request, items []T, pageSize int) ([]T, Pagination) {
	if pageSize <= 0 {
		pageSize = adminListPageSize
	}
	total := len(items)
	totalPages := 0
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	page := parsePositiveInt(r.URL.Query().Get("page"), 1)
	if totalPages == 0 {
		page = 1
	} else if page > totalPages {
		page = totalPages
	}

	start := 0
	end := 0
	if total > 0 {
		start = (page - 1) * pageSize
		end = start + pageSize
		if end > total {
			end = total
		}
	}

	pagination := Pagination{
		Page:       page,
		PageSize:   pageSize,
		Total:      total,
		TotalPages: totalPages,
		Show:       totalPages > 1,
	}
	if total > 0 {
		pagination.StartItem = start + 1
		pagination.EndItem = end
	}
	if page > 1 {
		pagination.PrevURL = pageURL(r, page-1)
	}
	if totalPages > 0 && page < totalPages {
		pagination.NextURL = pageURL(r, page+1)
	}
	if pagination.Show {
		for _, pageNumber := range visiblePageNumbers(page, totalPages) {
			pagination.Pages = append(pagination.Pages, PaginationLink{
				Page:    pageNumber,
				URL:     pageURL(r, pageNumber),
				Current: pageNumber == page,
			})
		}
	}

	return items[start:end], pagination
}

func parsePositiveInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 1 {
		return fallback
	}
	return parsed
}

func visiblePageNumbers(current, total int) []int {
	if total <= 7 {
		pages := make([]int, total)
		for i := 0; i < total; i++ {
			pages[i] = i + 1
		}
		return pages
	}
	start := current - 2
	if start < 1 {
		start = 1
	}
	end := start + 4
	if end > total {
		end = total
		start = end - 4
	}
	pages := make([]int, 0, end-start+1)
	for page := start; page <= end; page++ {
		pages = append(pages, page)
	}
	return pages
}

func pageURL(r *http.Request, page int) string {
	values := r.URL.Query()
	if page <= 1 {
		values.Del("page")
	} else {
		values.Set("page", strconv.Itoa(page))
	}
	if encoded := values.Encode(); encoded != "" {
		return r.URL.Path + "?" + encoded
	}
	return r.URL.Path
}

func filterMediaItemsByType(items []media.Item, filter string) []media.Item {
	filter = normalizeMediaTypeFilter(filter)
	if filter == "" {
		return items
	}
	filtered := make([]media.Item, 0, len(items))
	for _, item := range items {
		if mediaTypeID(item) == filter {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func mediaTypeFilters(r *http.Request, items []media.Item, active string) []MediaTypeFilter {
	active = normalizeMediaTypeFilter(active)
	counts := map[string]int{"": len(items)}
	for _, item := range items {
		counts[mediaTypeID(item)]++
	}
	filters := make([]MediaTypeFilter, 0, len(mediaTypeFilterSpecs))
	for _, spec := range mediaTypeFilterSpecs {
		filters = append(filters, MediaTypeFilter{
			ID:       spec.ID,
			LabelKey: spec.LabelKey,
			Label:    spec.Label,
			Count:    counts[spec.ID],
			URL:      mediaTypeFilterURL(r, spec.ID),
			Active:   spec.ID == active,
		})
	}
	return filters
}

func mediaTypeFilterURL(r *http.Request, filter string) string {
	values := r.URL.Query()
	values.Del("page")
	if filter == "" {
		values.Del("type")
	} else {
		values.Set("type", filter)
	}
	if encoded := values.Encode(); encoded != "" {
		return r.URL.Path + "?" + encoded
	}
	return r.URL.Path
}

func normalizeMediaTypeFilter(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, spec := range mediaTypeFilterSpecs {
		if spec.ID != "" && value == spec.ID {
			return value
		}
	}
	return ""
}

var mediaTypeFilterSpecs = []MediaTypeFilter{
	{ID: "", LabelKey: "media.filter.all", Label: "All"},
	{ID: "image", LabelKey: "media.filter.images", Label: "Images"},
	{ID: "audio", LabelKey: "media.filter.audio", Label: "Audio"},
	{ID: "video", LabelKey: "media.filter.video", Label: "Video"},
	{ID: "document", LabelKey: "media.filter.documents", Label: "Documents"},
	{ID: "archive", LabelKey: "media.filter.archives", Label: "Archives"},
	{ID: "other", LabelKey: "media.filter.other", Label: "Other"},
}

func (s *Server) adminPluginSettings(w http.ResponseWriter, r *http.Request) {
	pack, ok := s.findPlugin(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	ui, err := pluginUI(pack, pluginSettingsUIOutlet)
	if err != nil {
		store := s.currentStore()
		s.render(w, "admin_plugin_settings.html", ViewData{
			Title:       pack.Name,
			Store:       store,
			Plugin:      &pack,
			ActiveAdmin: "plugins",
			Error:       err.Error(),
		})
		return
	}
	store := s.currentStore()
	s.render(w, "admin_plugin_settings.html", ViewData{
		Title:       pack.Name,
		Store:       store,
		Plugin:      &pack,
		PluginUI:    ui,
		ActiveAdmin: "plugins",
	})
}

// adminMenus 渲染自定义菜单编辑页。
// 参数:
//   - w: HTTP 响应写入器。
//   - r: 当前请求；这里只需要通过登录态中间件确认权限，不读取额外参数。
//
// 返回:
//   - 无直接返回值；函数会把 admin_menus.html 写入响应。
//
// 设计说明:
//
//	/admin/menus 现在专职编辑自定义菜单，侧边栏编辑已拆到 /admin/sidebars。
func (s *Server) adminMenus(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	s.render(w, "admin_menus.html", ViewData{Title: "Custom Menus", TitleKey: "title.admin.custom_menus", Store: store, ActiveAdmin: "menus"})
}

// adminSidebars 渲染自定义侧边栏编辑页。
// 参数:
//   - w: HTTP 响应写入器。
//   - r: 当前请求；这里只需要通过登录态中间件确认权限，不读取额外参数。
//
// 返回:
//   - 无直接返回值；函数会把 admin_sidebars.html 写入响应。
//
// 设计说明:
//
//	该页面复用自定义菜单编辑器的布局和 menus.js（菜单编辑脚本），但保存时只提交 sidebars 字段。
func (s *Server) adminSidebars(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	s.render(w, "admin_sidebars.html", ViewData{Title: "Custom Sidebar", TitleKey: "title.admin.custom_sidebars", Store: store, ActiveAdmin: "sidebars"})
}

func (s *Server) adminThemeSettings(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	s.render(w, "admin_theme_settings.html", ViewData{Title: "Theme Settings", TitleKey: "title.admin.theme_settings", Store: store, ActiveAdmin: "theme-settings"})
}

func (s *Server) adminSettings(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	s.render(w, "admin_settings.html", ViewData{Title: "Settings", TitleKey: "title.admin.settings", Store: store, ActiveAdmin: "settings", AdminUsername: s.authSnapshot().user})
}

func (s *Server) adminPermalinks(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	s.render(w, "admin_permalinks.html", ViewData{Title: "Permalinks", TitleKey: "title.admin.permalinks", Store: store, ActiveAdmin: "permalinks"})
}

func (s *Server) adminUpdateSettings(w http.ResponseWriter, r *http.Request) {
	store := s.currentStore()
	updateLog, err := site.LoadUpdateLog(s.contentRoot, maxUpdateLogEntries)
	if err != nil {
		log.Printf("load update log: %v", err)
	}
	s.render(w, "admin_updates.html", ViewData{
		Title:       "Auto Update",
		TitleKey:    "title.admin.updates",
		Store:       store,
		ActiveAdmin: "updates",
		AppVersion:  currentAppVersion(),
		UpdateLog:   updateLog,
	})
}

func (s *Server) replyToComment(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	store := s.currentStore()
	s.commentMu.Lock()
	_, err := site.ReplyToComment(s.contentRoot, r.PathValue("id"), r.FormValue("reply"), time.Now().In(site.TimeLocation(store.Settings)))
	s.commentMu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/admin/comments#comment-"+r.PathValue("id"), http.StatusSeeOther)
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
			Date:    formatInputDateTime(p.Date),
			Updated: formatInputDateTime(p.Updated),
			Tags:    p.Tags,
			Summary: p.Summary,
			Draft:   p.Draft,
			URL:     site.PostURL(store.Settings, p),
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
		Date:    formatInputDateTime(p.Date),
		Updated: formatInputDateTime(p.Updated),
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
	saved, err := site.SavePost(s.contentRoot, draft)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if originalSlug := strings.TrimSpace(draft.OriginalSlug); originalSlug != "" && originalSlug != saved.Slug {
		s.commentMu.Lock()
		err = site.MoveComments(s.contentRoot, originalSlug, saved.Slug)
		s.commentMu.Unlock()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if err := s.reloadRuntime(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	store := s.currentStore()
	url := site.PostDraftURL(store.Settings, saved)
	if post := store.AllPostsBySlug[saved.Slug]; post != nil {
		url = site.PostURL(store.Settings, post)
	}
	writeJSON(w, map[string]any{
		"slug":    saved.Slug,
		"url":     url,
		"draft":   saved.Draft,
		"date":    saved.Date,
		"updated": saved.Updated,
	})
}

func (s *Server) deletePost(w http.ResponseWriter, r *http.Request) {
	if err := s.deleteContent(r.PathValue("slug"), site.DeletePost); err != nil {
		writeDeleteError(w, err)
		return
	}
	writeJSON(w, map[string]bool{"deleted": true})
}

func (s *Server) deletePostAndRedirect(w http.ResponseWriter, r *http.Request) {
	if err := s.deleteContent(r.PathValue("slug"), site.DeletePost); err != nil {
		writeDeleteError(w, err)
		return
	}
	http.Redirect(w, r, "/admin/posts", http.StatusSeeOther)
}

func (s *Server) listPages(w http.ResponseWriter, r *http.Request) {
	type pageSummary struct {
		Title   string `json:"title"`
		Slug    string `json:"slug"`
		Date    string `json:"date"`
		Updated string `json:"updated"`
		Summary string `json:"summary"`
		Draft   bool   `json:"draft"`
		URL     string `json:"url"`
	}
	store := s.currentStore()
	pages := make([]pageSummary, 0, len(store.AllPages))
	for _, p := range store.AllPages {
		pages = append(pages, pageSummary{
			Title:   p.Title,
			Slug:    p.Slug,
			Date:    formatInputDateTime(p.Date),
			Updated: formatInputDateTime(p.Updated),
			Summary: p.Summary,
			Draft:   p.Draft,
			URL:     site.PageURL(store.Settings, p),
		})
	}
	writeJSON(w, pages)
}

func (s *Server) getPage(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	store := s.currentStore()
	p := store.AllPagesBySlug[slug]
	if p == nil {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, site.PageDraft{
		Title:   p.Title,
		Slug:    p.Slug,
		Date:    formatInputDateTime(p.Date),
		Updated: formatInputDateTime(p.Updated),
		Summary: p.Summary,
		Draft:   p.Draft,
		TOC:     p.TOC,
		Body:    p.Source,
	})
}

func (s *Server) savePage(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	var draft site.PageDraft
	if err := json.NewDecoder(r.Body).Decode(&draft); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	saved, err := site.SavePage(s.contentRoot, draft)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.reloadRuntime(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	store := s.currentStore()
	url := site.PageDraftURL(store.Settings, saved)
	if page := store.AllPagesBySlug[saved.Slug]; page != nil {
		url = site.PageURL(store.Settings, page)
	}
	writeJSON(w, map[string]any{
		"slug":    saved.Slug,
		"url":     url,
		"draft":   saved.Draft,
		"date":    saved.Date,
		"updated": saved.Updated,
	})
}

func (s *Server) deletePage(w http.ResponseWriter, r *http.Request) {
	if err := s.deleteContent(r.PathValue("slug"), site.DeletePage); err != nil {
		writeDeleteError(w, err)
		return
	}
	writeJSON(w, map[string]bool{"deleted": true})
}

func (s *Server) deletePageAndRedirect(w http.ResponseWriter, r *http.Request) {
	if err := s.deleteContent(r.PathValue("slug"), site.DeletePage); err != nil {
		writeDeleteError(w, err)
		return
	}
	http.Redirect(w, r, "/admin/pages", http.StatusSeeOther)
}

func (s *Server) deleteContent(slug string, deleteFile func(string, string) error) error {
	if !site.ValidSlug(slug) {
		return fmt.Errorf("invalid slug %q", slug)
	}
	if err := deleteFile(s.contentRoot, slug); err != nil {
		return err
	}
	return s.reloadRuntime()
}

func writeDeleteError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, os.ErrNotExist):
		http.Error(w, "content not found", http.StatusNotFound)
	case strings.HasPrefix(err.Error(), "invalid "):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
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
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		s.setHomeImageFromMedia(w, r)
		return
	}

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

	item, err := s.media.SaveUploadWithOptions(file, header.Filename, s.mediaProcessingOptions())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	settings := s.currentStore().Settings
	settings.HomeImage = homeImageFromMediaItem(item)
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

func (s *Server) setHomeImageFromMedia(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var payload struct {
		MediaID string `json:"media_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	mediaID := strings.TrimSpace(payload.MediaID)
	if mediaID == "" {
		http.Error(w, "missing media_id", http.StatusBadRequest)
		return
	}
	item, ok := s.media.Item(mediaID)
	if !ok {
		http.Error(w, "media item not found", http.StatusNotFound)
		return
	}
	if !strings.HasPrefix(item.MIMEType, "image/") {
		http.Error(w, "selected media item is not an image", http.StatusBadRequest)
		return
	}

	settings := s.currentStore().Settings
	settings.HomeImage = homeImageFromMediaItem(item)
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

func homeImageFromMediaItem(item media.Item) site.HomeImage {
	alt := strings.TrimSpace(item.Alt)
	if alt == "" {
		alt = strings.TrimSpace(item.OriginalName)
	}
	if alt == "" {
		alt = "Home page image"
	}
	return site.HomeImage{
		Enabled: true,
		Src:     item.Path,
		Alt:     alt,
	}
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

func (s *Server) updateSiteTitleSettings(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var payload site.SiteTitle
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	settings := s.currentStore().Settings
	settings.SiteTitle = payload
	if err := site.SaveSettings(s.contentRoot, settings); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.reloadRuntime(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, s.currentStore().Settings.SiteTitle)
}

func (s *Server) updatePermalinkSettings(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var payload site.PermalinkSettings
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := site.ValidatePermalinks(payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	settings := s.currentStore().Settings
	settings.Permalinks = payload
	if err := site.SaveSettings(s.contentRoot, settings); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.reloadRuntime(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, s.currentStore().Settings.Permalinks)
}

func (s *Server) updateAutoUpdateSettings(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var payload site.AutoUpdateSettings
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	settings := s.currentStore().Settings
	settings.AutoUpdate = payload
	if err := site.SaveSettings(s.contentRoot, settings); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.reloadRuntime(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, s.currentStore().Settings.AutoUpdate)
}

func (s *Server) updateCommentSettings(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var payload site.CommentSettings
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	settings := s.currentStore().Settings
	settings.Comments = payload
	if err := site.SaveSettings(s.contentRoot, settings); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.reloadRuntime(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, s.currentStore().Settings.Comments)
}

func (s *Server) updateHomePageSettings(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var payload site.HomePageSettings
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	settings := s.currentStore().Settings
	settings.HomePage = payload
	if err := site.SaveSettings(s.contentRoot, settings); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.reloadRuntime(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, s.currentStore().Settings.HomePage)
}

func (s *Server) updateAdminAccountSettings(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var payload struct {
		Username        string `json:"username"`
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
		ConfirmPassword string `json:"confirm_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	username := strings.TrimSpace(payload.Username)
	if username == "" {
		http.Error(w, "username is required", http.StatusBadRequest)
		return
	}
	if strings.ContainsAny(username, "\r\n\t|") {
		http.Error(w, "username contains unsupported characters", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(payload.CurrentPassword) == "" {
		http.Error(w, "current password is required", http.StatusBadRequest)
		return
	}

	auth := s.authSnapshot()
	if !auth.verifyPassword(payload.CurrentPassword) {
		http.Error(w, "current password is incorrect", http.StatusUnauthorized)
		return
	}

	password := payload.CurrentPassword
	if payload.NewPassword != "" || payload.ConfirmPassword != "" {
		if len([]rune(payload.NewPassword)) < 8 {
			http.Error(w, "new password must be at least 8 characters", http.StatusBadRequest)
			return
		}
		if subtle.ConstantTimeCompare([]byte(payload.NewPassword), []byte(payload.ConfirmPassword)) != 1 {
			http.Error(w, "new passwords do not match", http.StatusBadRequest)
			return
		}
		password = payload.NewPassword
	}

	hash, err := hashAdminPassword(password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	credentials := storedAdminCredentials{
		Username:     username,
		PasswordHash: hash,
	}
	if err := saveStoredAdminCredentials(s.contentRoot, credentials); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	nextAuth := auth
	nextAuth.user = username
	nextAuth.pass = ""
	nextAuth.passwordHash = hash
	s.setAuth(nextAuth)
	http.SetCookie(w, s.sessionCookie(username, false))
	writeJSON(w, map[string]string{"username": username})
}

func (s *Server) updateTimeZoneSettings(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var payload struct {
		TimeZone string `json:"time_zone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	timeZone := strings.TrimSpace(payload.TimeZone)
	if !site.ValidTimeZone(timeZone) {
		http.Error(w, "invalid time zone", http.StatusBadRequest)
		return
	}
	settings := s.currentStore().Settings
	settings.TimeZone = timeZone
	if err := site.SaveSettings(s.contentRoot, settings); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.reloadRuntime(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"time_zone": s.currentStore().Settings.TimeZone})
}

func (s *Server) updateMediaProcessingSettings(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var payload site.MediaProcessing
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	settings := s.currentStore().Settings
	settings.MediaProcessing = payload
	if err := site.SaveSettings(s.contentRoot, settings); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.reloadRuntime(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, s.currentStore().Settings.MediaProcessing)
}

// updateMenusPayload 表示后台菜单编辑器提交的增量保存请求。
//
// 设计说明：
// Menus 和 Sidebars 使用切片指针，是为了区分“字段没有提交”和“字段提交为空列表”：
// - nil 表示旧客户端或分区保存没有提交该字段，应保留当前配置。
// - 非 nil 但长度为 0 表示用户明确删除了该类自定义结构，应保存为空列表。
type updateMenusPayload struct {
	Menus    *[]site.Menu `json:"menus"`
	Sidebars *[]site.Menu `json:"sidebars"`
}

func (s *Server) updateMenus(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var payload updateMenusPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	settings := s.currentStore().Settings
	if payload.Menus != nil {
		settings.ThemeSettings.Menus = filterPersistedMenus(*payload.Menus)
	}
	if payload.Sidebars != nil {
		settings.ThemeSettings.Sidebars = filterPersistedSidebars(*payload.Sidebars)
	}
	if err := site.SaveSettings(s.contentRoot, settings); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.reloadRuntime(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, themeSettingsWithDefaultMenu(ViewData{
		Store:      s.currentStore(),
		Appearance: s.currentAppearance(),
	}))
}

func (s *Server) updateThemeSettings(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var payload struct {
		MenuLocations map[string]string         `json:"menu_locations"`
		Sidebar       *string                   `json:"sidebar"`
		Custom        *site.ThemeCustomSettings `json:"custom"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	settings := s.currentStore().Settings
	settings.ThemeSettings.MenuLocations = payload.MenuLocations
	settings.ThemeSettings.Sidebar = payload.Sidebar
	if payload.Custom != nil {
		settings.ThemeSettings.Custom = *payload.Custom
	}
	if err := site.SaveSettings(s.contentRoot, settings); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.reloadRuntime(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, s.currentStore().Settings.ThemeSettings)
}

func (s *Server) mediaProcessingOptions() media.ProcessingOptions {
	settings := s.currentStore().Settings.MediaProcessing
	return media.ProcessingOptions{
		ConvertToWebP: settings.AutoWebP,
		WebPQuality:   settings.WebPQuality,
		KeepOriginal:  settings.KeepOriginal,
	}
}

type resourceMarketplaceResponse struct {
	Source string                    `json:"source"`
	Items  []resourceMarketplaceItem `json:"items"`
}

type resourceMarketplaceItem struct {
	ID               string                   `json:"id"`
	Name             string                   `json:"name"`
	Summary          string                   `json:"summary"`
	Description      string                   `json:"description"`
	Repo             string                   `json:"repo"`
	Preview          string                   `json:"preview"`
	Tags             []string                 `json:"tags"`
	Themes           []marketplace.PackMember `json:"themes"`
	Plugins          []marketplace.PackMember `json:"plugins"`
	Release          marketplace.Release      `json:"release"`
	MinPostizer      string                   `json:"min_postizer"`
	Installed        bool                     `json:"installed"`
	InstalledVersion string                   `json:"installed_version,omitempty"`
	Active           bool                     `json:"active"`
	UpdateAvailable  bool                     `json:"update_available"`
}

func (s *Server) resourceMarketplace(w http.ResponseWriter, r *http.Request) {
	index, source, err := s.loadResourceMarketplaceIndex(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, s.resourceMarketplaceResponse(index, source))
}

func (s *Server) installResourceMarketplaceItem(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var payload struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id := strings.TrimSpace(payload.ID)
	if id == "" {
		http.Error(w, "resource marketplace id is required", http.StatusBadRequest)
		return
	}

	index, _, err := s.loadResourceMarketplaceIndex(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	item, ok := marketplace.FindPack(index, id)
	if !ok {
		http.Error(w, "resource marketplace item not found", http.StatusNotFound)
		return
	}
	if err := ensureResourceMarketplaceCompatible(item); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	body, assetURL, err := marketplace.DownloadReleaseAsset(r.Context(), s.marketplaceClient, item)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	checksumBody, checksumURL, err := marketplace.DownloadReleaseChecksums(r.Context(), s.marketplaceClient, item)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if err := marketplace.VerifyReleaseAsset(item, body, checksumBody); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateMarketplaceBundle(body, item); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	installed, err := appearance.InstallPackZIPWithCompatibility(bytes.NewReader(body), int64(len(body)), s.userContentRoot, currentHostCompatibility())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.reloadRuntime(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"installed":    installed,
		"asset_url":    assetURL,
		"checksum_url": checksumURL,
	})
}

func (s *Server) loadResourceMarketplaceIndex(ctx context.Context) (marketplace.Index, string, error) {
	var failures []string
	for _, source := range s.resourceMarketplaceSources() {
		index, err := marketplace.LoadIndex(ctx, source, s.marketplaceClient)
		if err == nil {
			return index, source, nil
		}
		failures = append(failures, source+": "+err.Error())
	}
	if len(failures) == 0 {
		return marketplace.Index{}, "", errors.New("resource marketplace source is not configured")
	}
	return marketplace.Index{}, "", errors.New("load resource marketplace: " + strings.Join(failures, "; "))
}

func (s *Server) resourceMarketplaceSources() []string {
	if configured := strings.TrimSpace(os.Getenv("POSTIZER_RESOURCE_MARKETPLACE_URL")); configured != "" {
		return []string{configured}
	}
	if configured := strings.TrimSpace(os.Getenv("POSTIZER_THEME_MARKETPLACE_URL")); configured != "" {
		return []string{configured}
	}

	candidates := []string{marketplace.DefaultPackIndexURL}
	for _, candidate := range []string{
		filepath.Join(s.appRoot, "marketplace", "packs", "index.json"),
		filepath.Join("marketplace", "packs", "index.json"),
	} {
		candidate = filepath.Clean(candidate)
		if candidate == "." {
			continue
		}
		if _, err := os.Stat(candidate); err == nil {
			candidates = append(candidates, candidate)
		}
	}
	return dedupeStrings(candidates)
}

func (s *Server) resourceMarketplaceResponse(index marketplace.Index, source string) resourceMarketplaceResponse {
	catalog := s.currentAppearance()
	items := make([]resourceMarketplaceItem, 0, len(index.Items))
	for _, item := range index.Items {
		installed, installedVersion, active, updateAvailable := resourceMarketplaceInstallState(item, catalog)
		items = append(items, resourceMarketplaceItem{
			ID:               item.ID,
			Name:             item.Name,
			Summary:          item.Summary,
			Description:      item.Description,
			Repo:             item.Repo,
			Preview:          marketplace.ResolvePreviewURL(source, item.Preview),
			Tags:             item.Tags,
			Themes:           item.Themes,
			Plugins:          item.Plugins,
			Release:          item.Release,
			MinPostizer:      item.MinPostizer,
			Installed:        installed,
			InstalledVersion: installedVersion,
			Active:           active,
			UpdateAvailable:  updateAvailable,
		})
	}
	return resourceMarketplaceResponse{Source: source, Items: items}
}

func resourceMarketplaceInstallState(item marketplace.PackItem, catalog *appearance.Catalog) (bool, string, bool, bool) {
	if catalog == nil {
		return false, "", false, false
	}
	for _, bundle := range catalog.Bundles {
		if bundle.ID != item.ID || bundle.Source != appearance.SourceUser {
			continue
		}
		active := catalog.ActiveTheme.BundleID == bundle.ID
		updateAvailable := false
		if compare, ok := marketplace.CompareVersions(item.Release.Tag, bundle.Version); ok && compare > 0 {
			updateAvailable = true
		}
		return true, bundle.Version, active, updateAvailable
	}
	return false, "", false, false
}

func ensureResourceMarketplaceCompatible(item marketplace.PackItem) error {
	minPostizer := strings.TrimSpace(item.MinPostizer)
	if minPostizer == "" {
		return nil
	}
	if compare, ok := marketplace.CompareVersions(currentAppVersion(), minPostizer); ok && compare < 0 {
		return fmt.Errorf("resource pack %q requires Postizer %s or newer; current version is %s", item.ID, minPostizer, currentAppVersion())
	}
	return nil
}

func validateMarketplaceBundle(body []byte, item marketplace.PackItem) error {
	manifest, err := appearance.ReadPackZIPManifest(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return err
	}
	if manifest.Type != appearance.BundlePack {
		return fmt.Errorf("resource marketplace asset must be a %q bundle, got %q", appearance.BundlePack, manifest.Type)
	}
	if manifest.ID != item.ID {
		return fmt.Errorf("resource marketplace asset id %q does not match index id %q", manifest.ID, item.ID)
	}
	if !marketplace.SameGitHubRepo(manifest.SourceURL, item.Repo) {
		return errors.New("resource marketplace asset source_url must match the indexed GitHub repository")
	}
	return nil
}

func dedupeStrings(values []string) []string {
	seen := map[string]bool{}
	deduped := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		deduped = append(deduped, value)
	}
	return deduped
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
	installed, err := appearance.InstallPackZIPWithCompatibility(bytes.NewReader(body), int64(len(body)), s.userContentRoot, currentHostCompatibility())
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

func currentHostCompatibility() appearance.HostCompatibility {
	return appearance.HostCompatibility{
		PostizerVersion: currentAppVersion(),
		PluginServices: []appearance.PluginService{
			{
				Name:    pluginrpc.ServiceName,
				Methods: []string{"Handshake", "InvokeAction", "Shutdown"},
			},
		},
		HostServices: []appearance.PluginService{
			{
				Name: pluginrpc.HostServiceName,
				Methods: []string{
					"CreateJob",
					"UpdateJob",
					"SaveMedia",
					"SavePost",
					"SavePage",
					"ReloadRuntime",
					"CreateContentExport",
				},
			},
		},
	}
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

func (s *Server) invokePluginAction(w http.ResponseWriter, r *http.Request) {
	pack, ok := s.findPlugin(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	if pack.Runtime.Kind != appearance.RuntimeGRPC {
		http.Error(w, "plugin does not expose a grpc runtime", http.StatusBadRequest)
		return
	}

	req, uploadToken, err := s.pluginActionRequest(w, r, pack.ID, r.PathValue("action"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	response, err := s.pluginHost.InvokeAction(ctx, pack, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if uploadToken != "" {
		replaceUploadToken(response, uploadToken)
	}
	writeJSON(w, response)
}

func (s *Server) pluginJobStatus(w http.ResponseWriter, r *http.Request) {
	job, ok := s.importJob(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, job.snapshot())
}

func (s *Server) pluginActionRequest(w http.ResponseWriter, r *http.Request, pluginID, actionID string) (*pluginrpc.InvokeActionRequest, string, error) {
	fields := map[string]string{}
	var files []pluginrpc.ActionFile
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			return nil, "", err
		}
		for key, values := range r.MultipartForm.Value {
			if len(values) > 0 {
				fields[key] = values[len(values)-1]
			}
		}
		for fieldName, headers := range r.MultipartForm.File {
			for _, header := range headers {
				file, err := header.Open()
				if err != nil {
					return nil, "", err
				}
				body, readErr := readLimited(file, maxPluginActionFileBytes)
				closeErr := file.Close()
				if readErr != nil {
					return nil, "", readErr
				}
				if closeErr != nil {
					return nil, "", closeErr
				}
				files = append(files, pluginrpc.ActionFile{
					Name:        fieldName,
					Filename:    header.Filename,
					ContentType: header.Header.Get("Content-Type"),
					Body:        body,
				})
			}
		}
	} else {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		var payload struct {
			Fields map[string]string `json:"fields"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil && !errors.Is(err, io.EOF) {
			return nil, "", err
		}
		for key, value := range payload.Fields {
			fields[key] = value
		}
	}

	if token := strings.TrimSpace(fields["upload_token"]); token != "" {
		file, ok := s.pluginUpload(token)
		if !ok {
			return nil, "", fmt.Errorf("uploaded file token has expired")
		}
		files = append(files, file)
	}

	uploadToken := ""
	if len(files) > 0 && strings.TrimSpace(fields["upload_token"]) == "" {
		token, err := randomUploadToken()
		if err != nil {
			return nil, "", err
		}
		s.storePluginUpload(token, files[0])
		uploadToken = token
	}
	return &pluginrpc.InvokeActionRequest{
		PluginID: pluginID,
		ActionID: actionID,
		Fields:   fields,
		Files:    files,
	}, uploadToken, nil
}

func (s *Server) CreateJob(_ context.Context, req *pluginrpc.CreateJobRequest) (*pluginrpc.ImportJob, error) {
	id, err := randomUploadToken()
	if err != nil {
		return nil, err
	}
	total := req.Total
	if total == 0 {
		total = 1
	}
	job := &importJob{
		id:     id,
		status: "running",
		total:  total,
	}
	job.log(fallbackString(req.Title, "Plugin job created."))

	s.pluginJobMu.Lock()
	s.pluginJobs[id] = job
	s.pluginJobMu.Unlock()

	return job.snapshot(), nil
}

func (s *Server) UpdateJob(_ context.Context, req *pluginrpc.UpdateJobRequest) (*pluginrpc.ImportJob, error) {
	job, ok := s.importJob(req.JobID)
	if !ok {
		return nil, fmt.Errorf("plugin job %q not found", req.JobID)
	}
	job.update(req)
	return job.snapshot(), nil
}

func (s *Server) SaveMedia(_ context.Context, req *pluginrpc.SaveMediaRequest) (*pluginrpc.SaveMediaResponse, error) {
	if len(req.Body) == 0 {
		return nil, fmt.Errorf("media body is empty")
	}
	if int64(len(req.Body)) > maxPluginMediaBytes {
		return nil, fmt.Errorf("media file exceeds %d bytes", maxPluginMediaBytes)
	}
	item, err := s.media.SaveUploadWithOptions(bytes.NewReader(req.Body), fallbackString(req.OriginalName, "plugin-media.bin"), s.mediaProcessingOptions())
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Alt) != "" || strings.TrimSpace(req.Caption) != "" {
		item, err = s.media.Update(item.ID, media.Update{
			OriginalName: item.OriginalName,
			Alt:          req.Alt,
			Caption:      req.Caption,
		})
		if err != nil {
			return nil, err
		}
	}
	return &pluginrpc.SaveMediaResponse{Item: mediaItemRPC(item), Markdown: mediaFigureMarkdown(item)}, nil
}

func (s *Server) SavePost(_ context.Context, draft *pluginrpc.ContentDraft) (*pluginrpc.SaveContentResponse, error) {
	saved, err := site.SaveImportedPost(s.contentRoot, site.PostDraft{
		Title:   draft.Title,
		Slug:    draft.Slug,
		Date:    draft.Date,
		Updated: draft.Updated,
		Tags:    draft.Tags,
		Summary: draft.Summary,
		Draft:   draft.Draft,
		TOC:     draft.TOC,
		Body:    draft.Body,
	}, false)
	if err != nil {
		return nil, err
	}
	settings := s.currentStore().Settings
	return &pluginrpc.SaveContentResponse{Title: saved.Title, Slug: saved.Slug, URL: site.PostDraftURL(settings, saved)}, nil
}

func (s *Server) SavePage(_ context.Context, draft *pluginrpc.ContentDraft) (*pluginrpc.SaveContentResponse, error) {
	saved, err := site.SaveImportedPage(s.contentRoot, site.PageDraft{
		Title:   draft.Title,
		Slug:    draft.Slug,
		Date:    draft.Date,
		Updated: draft.Updated,
		Summary: draft.Summary,
		Draft:   draft.Draft,
		TOC:     draft.TOC,
		Body:    draft.Body,
	}, false)
	if err != nil {
		return nil, err
	}
	settings := s.currentStore().Settings
	return &pluginrpc.SaveContentResponse{Title: saved.Title, Slug: saved.Slug, URL: site.PageDraftURL(settings, saved)}, nil
}

func (s *Server) ReloadRuntime(context.Context, *pluginrpc.ReloadRuntimeRequest) (*pluginrpc.ReloadRuntimeResponse, error) {
	if err := s.reloadRuntime(); err != nil {
		return nil, err
	}
	return &pluginrpc.ReloadRuntimeResponse{OK: true}, nil
}

func (s *Server) importJob(id string) (*importJob, bool) {
	s.pluginJobMu.RLock()
	defer s.pluginJobMu.RUnlock()
	job, ok := s.pluginJobs[id]
	return job, ok
}

// resolvedPackSelection 鎶婂墠绔彁浜ょ殑璧勬簮鍖?ID 杞崲鎴愬畨鍏ㄧ殑瀛樺偍鍊笺€?// 濡傛灉鎻愪氦浜嗙┖鍊兼垨鏈煡鍊硷紝浼氳嚜鍔ㄥ洖閫€鍒伴粯璁ゅ寘锛岄伩鍏?settings 涓啓鍏ユ偓绌洪€夋嫨銆?
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

// parseResourcePackType 鎶?URL 涓殑绫诲瀷鐗囨杞崲鎴?appearance.PackType銆?//
// 鍙傛暟锛?// - value: 璺敱涓殑 `{type}`锛屽墠绔細鎻愪氦 manifest 浣跨敤鐨?bundle/theme/plugin/text銆?//
// 杩斿洖鍊硷細
// - 绗竴涓繑鍥炲€兼槸瑙勮寖鍖栧悗鐨勮祫婧愬寘绫诲瀷銆?// - 绗簩涓繑鍥炲€艰〃绀虹被鍨嬫槸鍚﹀彈鏀寔锛涗笉鏀寔鏃惰皟鐢ㄦ柟搴旇繑鍥?400銆?
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

// userInstalledPacks 杩斿洖褰撳墠鐩綍涓墍鏈夌敱鐢ㄦ埛鏈湴瀹夎鐨勮祫婧愬寘銆?//
// 鍙傛暟锛?// - catalog: 褰撳墠杩愯鏃跺瑙傜洰褰曞揩鐓с€?//
// 杩斿洖鍊硷細
// - 鍙寘鍚?SourceUser 鐨?bundle銆佺嫭绔嬩富棰樺寘銆佺嫭绔嬫彃浠跺寘浠ュ強鍏煎鏃х増 text 鍖呫€?// - bundle 鍐呴儴鐨勫瓙涓婚/瀛愭彃浠朵笉浼氬湪鏈湴璧勬簮鍖呭垪琛ㄩ噸澶嶅嚭鐜帮紝鍥犱负鍒犻櫎鍗曚綅鏄埗 bundle銆?//
// 鎺掑簭绛栫暐锛?// 1. 涓婚鍖呮帓鍦ㄦ彃浠?鏂囧瓧鍖呭墠闈紝绗﹀悎鈥滄牱寮忎紭鍏堚€濈殑璁剧疆椤甸槄璇婚『搴忋€?// 2. 鍚岀被鍨嬪唴鎸夊悕绉版帓搴忥紱鍚嶇О鐩稿悓鏃剁敤 ID 淇濇寔绋冲畾缁撴灉銆?
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

// packInUse 鍒ゆ柇璧勬簮鍖呮槸鍚︽鍦ㄥ弬涓庡綋鍓嶅瑙傘€?//
// 鍙傛暟锛?// - pack: 瑕佹鏌ョ殑璧勬簮鍖呫€?// - catalog: 褰撳墠杩愯鏃跺瑙傜洰褰曞揩鐓с€?//
// 杩斿洖鍊硷細
// - bundle 鍐呯殑褰撳墠涓婚鎴栦换涓€鍚敤鎻掍欢姝ｅ湪浣跨敤鏃惰繑鍥?true銆?// - 涓婚鍖?ID 涓?ActiveTheme 涓€鑷存椂杩斿洖 true銆?// - 鎻掍欢鍖呮垨鏃х増 text 鍖?ID 鍑虹幇鍦?PluginOrder 涓椂杩斿洖 true銆?
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

// findUserPack 浠庡綋鍓嶇洰褰曞揩鐓т腑鏌ユ壘涓€涓彲鍒犻櫎鐨勭敤鎴峰寘銆?//
// 鍙傛暟锛?// - catalog: 褰撳墠杩愯鏃跺瑙傜洰褰曞揩鐓с€?// - packType: URL 鎸囧畾鐨勮祫婧愬寘绫诲瀷銆?// - id: URL 鎸囧畾鐨勮祫婧愬寘 ID銆?//
// 杩斿洖鍊硷細
// - 鎵惧埌 SourceUser 涓旂被鍨?ID 閮藉尮閰嶇殑鍖呮椂杩斿洖璇ュ寘鍜?true銆?// - 瀹樻柟鍖呫€佹湭鐭ュ寘鎴栫被鍨嬩笉鍖归厤鏃惰繑鍥?false銆?
func findUserPack(catalog *appearance.Catalog, packType appearance.PackType, id string) (appearance.Pack, bool) {
	for _, pack := range userInstalledPacks(catalog) {
		if pack.Type == packType && pack.ID == id {
			return pack, true
		}
	}
	return appearance.Pack{}, false
}

// settingsAfterDeletingPack 璁＄畻鍒犻櫎璧勬簮鍖呭悗搴旇鍐欏洖鐨勮缃€?//
// 鍙傛暟锛?// - settings: 褰撳墠绔欑偣璁剧疆銆?// - pack: 鍗冲皢鍒犻櫎鐨勭敤鎴疯祫婧愬寘銆?// - defaultLocale: 榛樿涓婚鍖呯殑榛樿璇█锛岀敤浜庝富棰樺寘琚垹闄ゆ椂涓€骞舵仮澶嶃€?//
// 杩斿洖鍊硷細
// - 绗竴涓繑鍥炲€兼槸鍒犻櫎鍚庡簲淇濆瓨鐨勮缃€?// - 绗簩涓繑鍥炲€艰〃绀鸿缃槸鍚﹀彂鐢熷彉鍖栵紱鍙湁 true 鏃舵墠闇€瑕?SaveSettings銆?//
// 琛屼负锛?// - 鍒犻櫎姝ｅ湪浣跨敤鐨勪富棰樺寘鏃讹紝涓婚鎭㈠鍒扮郴缁熼粯璁や富棰橈紝骞舵妸涓婚璇█鎭㈠涓洪粯璁や富棰樿瑷€銆?// - 鍒犻櫎 bundle 鏃讹紝濡傛灉褰撳墠涓婚鏉ヨ嚜璇?bundle锛屽垯鎭㈠榛樿涓婚锛涘鏋滃惎鐢ㄦ彃浠舵潵鑷 bundle锛屽垯浠庨『搴忎腑绉婚櫎銆?// - 鍒犻櫎姝ｅ湪浣跨敤鐨勬彃浠跺寘鎴栨棫鐗?text 鍖呮椂锛屼粠鎻掍欢鍚敤椤哄簭涓Щ闄よ ID銆?// - 鍒犻櫎鏈惎鐢ㄧ殑鍖呮椂涓嶆敼鍙樿缃€?
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

// bundleContainsPackID 鍒ゆ柇涓€涓?bundle 瀛愬寘 ID 鍒楄〃涓槸鍚﹀寘鍚洰鏍?ID銆?//
// 鍙傛暟锛?// - ids: bundle 鎵弿闃舵璁板綍涓嬫潵鐨勪富棰樻垨鎻掍欢 ID 鍒楄〃銆?// - target: 褰撳墠璁剧疆涓繚瀛樼殑涓婚/鎻掍欢 ID銆?//
// 杩斿洖鍊硷細
// - 鎵惧埌瀹屽叏鍖归厤鐨?ID 鏃惰繑鍥?true锛涚┖瀛楃涓叉垨鏈懡涓繑鍥?false銆?
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

// fallbackActiveBundleWithoutChildren 鍏煎鏋佹棫鐨?bundle 蹇収銆?//
// 鍙傛暟锛?// - pack: 鍗冲皢鍒犻櫎鐨?bundle 鍖呫€?//
// 杩斿洖鍊硷細
// - 褰撳寘琚爣璁颁负 active锛屼絾娌℃湁璁板綍浠讳綍瀛愪富棰?瀛愭彃浠?ID 鏃惰繑鍥?true銆?//
// 璁捐璇存槑锛?// 姝ｅ父鎯呭喌涓?bundle 閮戒細鎼哄甫 BundledThemeIDs/BundledPluginIDs锛屽垹闄ら€昏緫鍙互绮剧‘娓呯悊銆?// 杩欎釜鍏滃簳鍒嗘敮鍙敤浜庨伩鍏嶆棫杩愯鏃跺揩鐓х己灏戝瓙 ID 鏃舵棤娉曟仮澶嶉粯璁や富棰樸€?
func fallbackActiveBundleWithoutChildren(pack appearance.Pack) bool {
	return pack.Active && len(pack.BundledThemeIDs) == 0 && len(pack.BundledPluginIDs) == 0
}

// filterPluginOrder 浠庢彃浠跺惎鐢ㄩ『搴忎腑绉婚櫎鎸囧畾鎻掍欢 ID銆?//
// 鍙傛暟锛?// - order: 褰撳墠 settings.PluginOrder銆?// - deletedIDs: 鍗冲皢鍒犻櫎鐨勭嫭绔嬫彃浠?ID锛屾垨鏌愪釜 bundle 鍐呯殑鎵€鏈夋彃浠?ID銆?//
// 杩斿洖鍊硷細
// - 绗竴涓繑鍥炲€兼槸杩囨护鍚庣殑鎻掍欢椤哄簭銆?// - 绗簩涓繑鍥炲€艰〃绀烘槸鍚︾湡鐨勭Щ闄や簡鑷冲皯涓€涓?ID銆?
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

func pluginTools(catalog *appearance.Catalog) []appearance.Pack {
	if catalog == nil {
		return nil
	}
	var plugins []appearance.Pack
	for _, plugin := range catalog.Plugins {
		if hasUIOutlet(plugin, pluginSettingsUIOutlet) {
			plugins = append(plugins, plugin)
		}
	}
	return plugins
}

func hasUIOutlet(pack appearance.Pack, outlet string) bool {
	outlet = strings.TrimSpace(outlet)
	for _, entry := range pack.UIEntries {
		if strings.TrimSpace(entry.Outlet) == outlet && strings.TrimSpace(entry.Path) != "" {
			return true
		}
	}
	return false
}

func pluginAction(ui *appearance.PluginUI, id string) appearance.PluginUIAction {
	if ui == nil {
		return appearance.PluginUIAction{}
	}
	for _, action := range ui.Actions {
		if action.ID == id {
			return action
		}
	}
	return appearance.PluginUIAction{}
}

func pluginUI(pack appearance.Pack, outlet string) (*appearance.PluginUI, error) {
	var merged appearance.PluginUI
	found := false
	for _, entry := range pack.UIEntries {
		if strings.TrimSpace(entry.Outlet) != outlet {
			continue
		}
		found = true
		ui, err := readPluginUI(pack, entry)
		if err != nil {
			return nil, err
		}
		merged.Pages = append(merged.Pages, ui.Pages...)
		merged.Actions = append(merged.Actions, ui.Actions...)
	}
	if !found {
		return nil, fmt.Errorf("plugin %q does not declare a %s UI entry", pack.ID, outlet)
	}
	return &merged, nil
}

func readPluginUI(pack appearance.Pack, entry appearance.UIEntry) (appearance.PluginUI, error) {
	cleaned := filepath.Clean(strings.ReplaceAll(strings.TrimSpace(entry.Path), "\\", "/"))
	if cleaned == "." || cleaned == "" || filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) || cleaned == ".." {
		return appearance.PluginUI{}, fmt.Errorf("plugin ui entry %q escapes pack root", entry.ID)
	}
	fullPath := filepath.Join(pack.RootDir, cleaned)
	body, err := os.ReadFile(fullPath)
	if err != nil {
		return appearance.PluginUI{}, fmt.Errorf("read plugin ui %q: %w", entry.ID, err)
	}
	var ui appearance.PluginUI
	if err := json.Unmarshal(body, &ui); err != nil {
		return appearance.PluginUI{}, fmt.Errorf("parse plugin ui %q: %w", entry.ID, err)
	}
	return ui, nil
}

func (s *Server) findPlugin(id string) (appearance.Pack, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return appearance.Pack{}, false
	}
	catalog := s.currentAppearance()
	if catalog == nil {
		return appearance.Pack{}, false
	}
	for _, plugin := range catalog.Plugins {
		if plugin.ID == id {
			return plugin, true
		}
	}
	return appearance.Pack{}, false
}

func randomUploadToken() (string, error) {
	var raw [24]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func (s *Server) storePluginUpload(token string, file pluginrpc.ActionFile) {
	s.pluginUploadMu.Lock()
	defer s.pluginUploadMu.Unlock()
	s.pluginUploads[token] = file
}

func (s *Server) pluginUpload(token string) (pluginrpc.ActionFile, bool) {
	s.pluginUploadMu.Lock()
	defer s.pluginUploadMu.Unlock()
	file, ok := s.pluginUploads[token]
	return file, ok
}

func (j *importJob) log(message string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.logs = append(j.logs, time.Now().Format("15:04:05")+" "+message)
}

func (j *importJob) fail(message string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.errors++
	j.logs = append(j.logs, time.Now().Format("15:04:05")+" "+message)
}

func (j *importJob) update(req *pluginrpc.UpdateJobRequest) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if req.Total > 0 {
		j.total = req.Total
	}
	if j.total <= 0 {
		j.total = 1
	}
	if req.Done > 0 {
		j.done = req.Done
		if j.done > j.total {
			j.done = j.total
		}
	}
	now := time.Now().Format("15:04:05")
	if message := strings.TrimSpace(req.Log); message != "" {
		j.logs = append(j.logs, now+" "+message)
	}
	if message := strings.TrimSpace(req.Error); message != "" {
		j.errors++
		j.logs = append(j.logs, now+" "+message)
	}
	if req.Sections != nil {
		j.sections = append([]pluginrpc.ResultSection(nil), req.Sections...)
	}
	if status := strings.TrimSpace(req.Status); status != "" {
		j.status = status
	}
}

func (j *importJob) step() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.done < j.total {
		j.done++
	}
}

func (j *importJob) setSections(sections []pluginrpc.ResultSection) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.sections = sections
}

func (j *importJob) finish() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.done = j.total
	if j.errors > 0 {
		j.status = "completed_with_errors"
		j.logs = append(j.logs, time.Now().Format("15:04:05")+" Import completed with "+strconv.Itoa(j.errors)+" error(s).")
		return
	}
	j.status = "completed"
	j.logs = append(j.logs, time.Now().Format("15:04:05")+" Import completed.")
}

func (j *importJob) snapshot() *pluginrpc.ImportJob {
	j.mu.Lock()
	defer j.mu.Unlock()
	percent := 0
	if j.total > 0 {
		percent = (j.done * 100) / j.total
	}
	logs := append([]string(nil), j.logs...)
	sections := append([]pluginrpc.ResultSection(nil), j.sections...)
	return &pluginrpc.ImportJob{
		ID:       j.id,
		Status:   j.status,
		Done:     j.done,
		Total:    j.total,
		Percent:  percent,
		Logs:     logs,
		Sections: sections,
	}
}

func replaceUploadToken(response *pluginrpc.InvokeActionResponse, token string) {
	if response == nil || token == "" {
		return
	}
	for actionIndex := range response.NextActions {
		if response.NextActions[actionIndex].Fields == nil {
			response.NextActions[actionIndex].Fields = map[string]string{}
		}
		for key, value := range response.NextActions[actionIndex].Fields {
			if value == "__HOST_UPLOAD_TOKEN__" {
				response.NextActions[actionIndex].Fields[key] = token
			}
		}
	}
}

func readLimited(reader io.Reader, maxBytes int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("file exceeds %d bytes", maxBytes)
	}
	return body, nil
}

func fallbackString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "item"
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
	item, err := s.media.SaveUploadWithOptions(file, header.Filename, s.mediaProcessingOptions())
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
	if !mediaIsImage(item) {
		return fmt.Sprintf("[%s](%s)", escapeMarkdownLinkText(mediaFileLabel(item)), escapeMarkdownLinkDestination(item.Path))
	}
	caption := strings.TrimSpace(item.Caption)
	if caption == "" {
		caption = item.Alt
	}
	labelBase := site.NormalizeSlug(strings.TrimSuffix(item.OriginalName, filepath.Ext(item.OriginalName)))
	if labelBase == "" {
		labelBase = item.ID
	}
	return fmt.Sprintf(
		"\\begin{figure}\n![%s](%s)\n\\caption{%s}\n\\label{fig:%s}\n\\end{figure}",
		escapeMarkdownImageAlt(item.Alt),
		item.Path,
		escapeLatexBraceText(caption),
		labelBase,
	)
}

func mediaItemRPC(item media.Item) pluginrpc.MediaItem {
	return pluginrpc.MediaItem{
		ID:           item.ID,
		Path:         item.Path,
		OriginalName: item.OriginalName,
		Alt:          item.Alt,
		Caption:      item.Caption,
		MIMEType:     item.MIMEType,
		Width:        item.Width,
		Height:       item.Height,
	}
}

func mediaIsImage(item media.Item) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(item.MIMEType)), "image/")
}

func mediaTypeID(item media.Item) string {
	mimeType := strings.ToLower(strings.TrimSpace(item.MIMEType))
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return "image"
	case strings.HasPrefix(mimeType, "audio/"):
		return "audio"
	case strings.HasPrefix(mimeType, "video/"):
		return "video"
	case isArchiveMedia(item, mimeType):
		return "archive"
	case isDocumentMedia(item, mimeType):
		return "document"
	default:
		return "other"
	}
}

func isDocumentMedia(item media.Item, mimeType string) bool {
	if strings.HasPrefix(mimeType, "text/") {
		return true
	}
	switch mimeType {
	case "application/pdf",
		"application/json",
		"application/msword",
		"application/rtf",
		"application/vnd.ms-excel",
		"application/vnd.ms-powerpoint",
		"application/vnd.openxmlformats-officedocument.presentationml.presentation",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return true
	}
	switch mediaFileExtension(item) {
	case ".csv", ".doc", ".docx", ".epub", ".json", ".md", ".ods", ".odt", ".pdf", ".ppt", ".pptx", ".rtf", ".txt", ".xls", ".xlsx", ".xml":
		return true
	default:
		return false
	}
}

func isArchiveMedia(item media.Item, mimeType string) bool {
	switch mimeType {
	case "application/gzip",
		"application/java-archive",
		"application/vnd.rar",
		"application/x-7z-compressed",
		"application/x-bzip",
		"application/x-bzip2",
		"application/x-gzip",
		"application/x-rar-compressed",
		"application/x-tar",
		"application/zip":
		return true
	}
	switch mediaFileExtension(item) {
	case ".7z", ".bz2", ".gz", ".jar", ".rar", ".tar", ".tgz", ".xz", ".zip":
		return true
	default:
		return false
	}
}

func mediaFileExtension(item media.Item) string {
	for _, value := range []string{item.OriginalName, item.Path} {
		if ext := strings.ToLower(filepath.Ext(value)); ext != "" {
			return ext
		}
	}
	return ""
}

func mediaFileLabel(item media.Item) string {
	for _, value := range []string{item.Caption, item.Alt, item.OriginalName, filepath.Base(item.Path)} {
		if label := strings.TrimSpace(value); label != "" && label != "." && label != string(filepath.Separator) {
			return label
		}
	}
	return "File"
}

func mediaFileType(item media.Item) string {
	for _, value := range []string{item.OriginalName, item.Path} {
		if ext := strings.TrimPrefix(strings.ToUpper(filepath.Ext(value)), "."); ext != "" {
			return ext
		}
	}
	return "FILE"
}

func mediaMeta(item media.Item) string {
	parts := []string{}
	if mediaIsImage(item) && item.Width > 0 && item.Height > 0 {
		parts = append(parts, fmt.Sprintf("%dx%d", item.Width, item.Height))
	}
	if mimeType := strings.TrimSpace(item.MIMEType); mimeType != "" {
		parts = append(parts, mimeType)
	}
	if len(parts) == 0 {
		return mediaFileType(item)
	}
	return strings.Join(parts, " / ")
}

func escapeMarkdownImageAlt(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `]`, `\]`)
}

func escapeMarkdownLinkText(value string) string {
	return escapeMarkdownImageAlt(value)
}

func escapeMarkdownLinkDestination(value string) string {
	value = strings.ReplaceAll(value, `\`, `%5C`)
	value = strings.ReplaceAll(value, `(`, `%28`)
	return strings.ReplaceAll(value, `)`, `%29`)
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
		docs = append(docs, doc{Title: p.Title, URL: site.PostURL(store.Settings, p), Summary: p.Summary, Tags: p.Tags})
	}
	for _, p := range store.Pages {
		docs = append(docs, doc{Title: p.Title, URL: site.PageURL(store.Settings, p), Summary: p.Summary})
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
	if !data.NeedsMath {
		data.NeedsMath = viewNeedsMath(name, data)
	}
	if data.ActiveAdmin == "dashboard" {
		data.LoginNotice = s.loginNotice()
	}
	if err := s.currentTemplates().ExecuteTemplate(w, name, data); err != nil {
		log.Printf("template %s: %v", name, err)
	}
}

func viewNeedsMath(templateName string, data ViewData) bool {
	if data.EditorKind != "" {
		return true
	}
	if data.Post != nil && contentNeedsMath(data.Post.Source) {
		return true
	}
	if data.Page != nil && contentNeedsMath(data.Page.Source) {
		return true
	}
	if templateName == "home.html" {
		for _, post := range data.Posts {
			if post != nil && contentNeedsMath(post.Source) {
				return true
			}
		}
	}
	return false
}

func contentNeedsMath(source string) bool {
	if source == "" {
		return false
	}
	if strings.Contains(source, `\(`) ||
		strings.Contains(source, `\[`) ||
		strings.Contains(source, `\begin{equation}`) ||
		strings.Contains(source, `\begin{align}`) ||
		strings.Contains(source, "$$") {
		return true
	}
	return containsUnescapedDollar(source)
}

func containsUnescapedDollar(source string) bool {
	escaped := false
	for _, char := range source {
		if escaped {
			escaped = false
			continue
		}
		if char == '\\' {
			escaped = true
			continue
		}
		if char == '$' {
			return true
		}
	}
	return false
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

// reloadRuntime 鍦ㄨ缃垨璧勬簮鍖呭彂鐢熷彉鍖栧悗閲嶆柊瑁呰浇绔欑偣杩愯鏃剁姸鎬併€?//
// 杩欓噷鎶婂唴瀹规暟鎹€佽祫婧愬寘鐩綍銆佹ā鏉块泦鍚堢湅浣滀竴涓暣浣撳揩鐓х粺涓€鏇挎崲锛?// 閬垮厤涓婚鍒囨崲鍚庡嚭鐜扳€滃唴瀹瑰凡鏇存柊浣嗘ā鏉胯繕鏄棫鐨勨€濊繖绫诲崐鍒锋柊鐘舵€併€?
func (s *Server) reloadRuntime() error {
	store, err := site.Load(s.contentRoot)
	if err != nil {
		return err
	}
	catalog, err := appearance.LoadCatalog(s.builtinBundlesRoot, s.userContentRoot, store.Settings.ThemePack, store.Settings.ThemeLocale, store.Settings.PluginOrder)
	if err != nil {
		return err
	}
	templates, err := s.loadTemplates(catalog.ActiveTheme)
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

func siteMainTitle(data ViewData) string {
	if data.Store == nil {
		return site.DefaultSiteTitle
	}
	title := strings.TrimSpace(data.Store.Settings.SiteTitle.Main)
	if title == "" {
		return site.DefaultSiteTitle
	}
	return title
}

func siteSubtitle(data ViewData) string {
	if data.Store == nil {
		return ""
	}
	return strings.TrimSpace(data.Store.Settings.SiteTitle.Subtitle)
}

func siteEditionLine(data ViewData) string {
	if subtitle := siteSubtitle(data); subtitle != "" {
		return subtitle
	}
	return messageFromViewData(data, "site.edition_line", "")
}

func pageTitleFromViewData(data ViewData) string {
	pageTitle := strings.TrimSpace(data.Title)
	if strings.TrimSpace(data.TitleKey) != "" {
		pageTitle = strings.TrimSpace(messageFromViewData(data, data.TitleKey, data.Title))
	}
	mainTitle := siteMainTitle(data)
	if data.Home || pageTitle == "" || strings.EqualFold(pageTitle, mainTitle) || strings.EqualFold(pageTitle, site.DefaultSiteTitle) {
		pageTitle = mainTitle
	} else {
		pageTitle += " | " + mainTitle
	}
	if subtitle := siteSubtitle(data); subtitle != "" {
		pageTitle += " | " + subtitle
	}
	return pageTitle
}

// adminThemeNavGroup 构造后台侧边栏的主题二级导航数据。
// 参数:
//   - data: 当前页面的 ViewData（视图数据），其中 ActiveAdmin 用于判断当前高亮项。
//
// 返回:
//   - AdminNavGroup（后台导航组），包含“主题”父级和主题包、主题设置两个子项。
//
// 使用示例:
//
//	模板中通过 {{template "adminNavGroup" (adminThemeNavGroup .)}} 复用统一的折叠组件。
func adminThemeNavGroup(data ViewData) AdminNavGroup {
	return newAdminNavGroup(data, "theme", "adminNavThemeSub", "nav.theme", "Theme", []AdminNavItem{
		{
			ActiveAdmin:   "appearance",
			URL:           "/admin/appearance",
			LabelKey:      "nav.appearance",
			LabelFallback: "Theme Packs",
		},
		{
			ActiveAdmin:   "theme-settings",
			URL:           "/admin/theme-settings",
			LabelKey:      "nav.theme_settings",
			LabelFallback: "Theme Settings",
		},
	})
}

// adminCustomNavGroup 构造后台侧边栏的自定义二级导航数据。
// 参数:
//   - data: 当前页面的 ViewData（视图数据），其中 ActiveAdmin 用于判断当前高亮项。
//
// 返回:
//   - AdminNavGroup（后台导航组），包含“自定义”父级和自定义菜单、自定义侧边栏两个子项。
//
// 使用示例:
//
//	模板中通过 {{template "adminNavGroup" (adminCustomNavGroup .)}} 复用统一的折叠组件。
func adminCustomNavGroup(data ViewData) AdminNavGroup {
	return newAdminNavGroup(data, "customize", "adminNavCustomizeSub", "nav.menus", "Customize", []AdminNavItem{
		{
			ActiveAdmin:   "menus",
			URL:           "/admin/menus",
			LabelKey:      "nav.custom_menus",
			LabelFallback: "Custom Menus",
		},
		{
			ActiveAdmin:   "sidebars",
			URL:           "/admin/sidebars",
			LabelKey:      "nav.custom_sidebars",
			LabelFallback: "Custom Sidebar",
		},
	})
}

// adminSettingsNavGroup 构造后台侧边栏的设置二级导航数据。
// 参数:
//   - data: 当前页面的 ViewData（视图数据），其中 ActiveAdmin 用于判断当前高亮项。
//
// 返回:
//   - AdminNavGroup（后台导航组），包含“设置”父级和通用设置、固定链接、自动更新三个子项。
//
// 使用示例:
//
//	模板中通过 {{template "adminNavGroup" (adminSettingsNavGroup .)}} 复用统一的折叠组件。
func adminSettingsNavGroup(data ViewData) AdminNavGroup {
	return newAdminNavGroup(data, "settings", "adminNavSettingsSub", "nav.settings", "Settings", []AdminNavItem{
		{
			ActiveAdmin:   "settings",
			URL:           "/admin/settings",
			LabelKey:      "nav.general_settings",
			LabelFallback: "General Settings",
		},
		{
			ActiveAdmin:   "permalinks",
			URL:           "/admin/settings/permalinks",
			LabelKey:      "nav.permalinks",
			LabelFallback: "Permalinks",
		},
		{
			ActiveAdmin:   "updates",
			URL:           "/admin/settings/updates",
			LabelKey:      "nav.updates",
			LabelFallback: "Auto Update",
		},
	})
}

// newAdminNavGroup 根据当前后台页面状态生成可折叠导航组。
// 参数:
//   - data: 当前页面的 ViewData（视图数据）。
//   - key: 分组稳定标识，用于 CSS/JS 状态和 localStorage 记忆展开状态。
//   - controlsID: 子菜单容器 ID，用于 aria-controls 关联父级按钮和子菜单。
//   - titleKey: 父级标题的翻译键。
//   - titleFallback: 父级标题缺少翻译时使用的英文 fallback。
//   - items: 子菜单项列表，ActiveAdmin 字段会与 data.ActiveAdmin 匹配。
//
// 返回:
//   - AdminNavGroup（后台导航组），其中 Active 表示当前页面是否落在该分组下。
//
// 设计说明:
//
//	组件需要知道哪个子项处于当前页，以便服务端首屏就展开对应分组，避免 JS 加载前
//	侧边栏先闪一下收起状态。这里集中计算 Active，模板只负责展示。
func newAdminNavGroup(data ViewData, key, controlsID, titleKey, titleFallback string, items []AdminNavItem) AdminNavGroup {
	active := false
	for index := range items {
		items[index].Active = data.ActiveAdmin == items[index].ActiveAdmin
		if items[index].Active {
			active = true
		}
	}
	return AdminNavGroup{
		View:          data,
		Key:           key,
		ControlsID:    controlsID,
		TitleKey:      titleKey,
		TitleFallback: titleFallback,
		Active:        active,
		Items:         items,
	}
}

func siteFeedDescription(data ViewData) string {
	title := siteMainTitle(data)
	if subtitle := siteSubtitle(data); subtitle != "" {
		return title + " | " + subtitle
	}
	return title + " feed"
}

func menuLinksForLocation(data ViewData, location string) []site.MenuLink {
	if data.Store == nil {
		return nil
	}
	if menuID, ok := data.Store.Settings.ThemeSettings.MenuLocations[strings.TrimSpace(location)]; ok {
		if menuID == defaultThemeMenuID {
			return menuLinksFromMenu(data.Store, defaultMenu(data))
		}
	}
	links := data.Store.MenuLinks(location)
	if len(links) > 0 || data.Store.MenuLocationAssigned(location) {
		return links
	}
	if location != "navbar" {
		return nil
	}
	return menuLinksFromMenu(data.Store, defaultMenu(data))
}

func menuAdminData(data ViewData) any {
	if data.Store == nil {
		return map[string]any{}
	}
	return map[string]any{
		"default_menu_id":    defaultThemeMenuID,
		"default_sidebar_id": defaultSidebarID,
		"settings":           themeSettingsWithDefaultMenu(data),
		"options":            menuContentOptions(data.Store),
	}
}

// sidebarLabelsFromViewData 读取当前主题语言下的侧边栏区块默认文案。
//
// @param data 当前模板视图数据。
// @returns 返回自动区块和订阅链接使用的文案。
func sidebarLabelsFromViewData(data ViewData) site.SidebarLabels {
	return site.SidebarLabels{
		TopicsTitle:      messageFromViewData(data, "site.right_rail.topics", "Topics"),
		PagesTitle:       messageFromViewData(data, "site.right_rail.pages", "Pages"),
		FeedsTitle:       messageFromViewData(data, "site.right_rail.feeds", "Feeds"),
		RecentPostsTitle: messageFromViewData(data, "site.right_rail.recent_posts", "Recent Posts"),
		CustomTitle:      messageFromViewData(data, "sidebar.type.custom", "Custom Area"),
		RSSLabel:         messageFromViewData(data, "site.feed.rss", "RSS"),
		SitemapLabel:     messageFromViewData(data, "site.feed.sitemap", "Sitemap"),
	}
}

// sidebarAdminMenus 返回后台侧边栏编辑器使用的侧边栏列表。
//
// 设计说明：
//   - 第一层保持“侧边栏列表”，不把标签/页面等功能塞进顶部下拉框。
//   - default-sidebar 和 default-menu 一样只是后台展示用的运行时哨兵项，
//     前端会禁止保存、删除或改名，保存 payload 时也会过滤掉。
//   - 空侧边栏按空结构展示，和新建空菜单一致，不自动塞默认区块。
//   - 旧版直接链接侧边栏只包装成一个“自定义区域”区块，不在后台结构里
//     额外插入标签、页面、订阅等预设项。
//
// @param data 当前模板视图数据，用于读取默认标题。
// @param sidebars settings.json 中保存的侧边栏列表。
// @returns 返回可由后台直接编辑的侧边栏列表。
func sidebarAdminMenus(data ViewData, sidebars []site.Menu) []site.Menu {
	labels := sidebarLabelsFromViewData(data)
	adminSidebars := make([]site.Menu, 0, len(sidebars)+1)
	adminSidebars = append(adminSidebars, defaultSidebar(data, labels))
	for _, sidebar := range sidebars {
		if sidebar.ID == defaultSidebarID {
			continue
		}
		if len(sidebar.Items) == 0 {
			if legacyGeneratedSidebarName(sidebar.Name, labels) {
				sidebar.Name = messageFromViewData(data, "sidebar.section.default_name", "New Sidebar")
			}
			adminSidebars = append(adminSidebars, sidebar)
			continue
		}
		if sidebarHasSidebarBlocks(sidebar.Items) {
			adminSidebars = append(adminSidebars, sidebar)
			continue
		}
		title := strings.TrimSpace(sidebar.Name)
		if title == "" {
			title = labels.CustomTitle
		}
		sidebar.Items = []site.MenuItem{{
			Type:  site.SidebarSectionTypeCustom,
			Label: title,
			Items: sidebar.Items,
		}}
		adminSidebars = append(adminSidebars, sidebar)
	}
	return adminSidebars
}

// legacyGeneratedSidebarName 判断侧边栏名是否是旧版新建逻辑生成的默认名称。
//
// @param name settings.json 中保存的侧边栏名称。
// @param labels 当前主题语言下的侧边栏文案。
// @returns 如果名称像旧版“自定义侧边栏 / Custom Sidebar”默认名则返回 true。
func legacyGeneratedSidebarName(name string, labels site.SidebarLabels) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return true
	}
	return name == strings.TrimSpace(labels.CustomTitle) || name == "自定义侧边栏" || name == "Custom Sidebar"
}

// sidebarHasSidebarBlocks 判断侧边栏是否已经使用新版区块结构。
//
// @param items 侧边栏项目列表。
// @returns 如果包含任一新版区块类型则返回 true。
func sidebarHasSidebarBlocks(items []site.MenuItem) bool {
	for _, item := range items {
		switch strings.TrimSpace(item.Type) {
		case site.SidebarSectionTypeNone, site.SidebarSectionTypeCustom, site.SidebarSectionTypeTopics, site.SidebarSectionTypePages, site.SidebarSectionTypeFeeds, site.SidebarSectionTypeRecentPosts:
			return true
		}
	}
	return false
}

func themeSettingsData(data ViewData) any {
	if data.Store == nil {
		return map[string]any{}
	}
	locations := []appearance.MenuLocation{}
	if data.Appearance != nil {
		locations = data.Appearance.ActiveTheme.MenuLocations
	}
	settings := themeSettingsWithoutDefaultMenu(data.Store.Settings.ThemeSettings)
	settings.Sidebars = themeSettingsSidebarChoices(data, settings.Sidebars)
	return map[string]any{
		"default_menu_id": defaultThemeMenuID,
		"settings":        settings,
		"locations":       locations,
		"options":         menuContentOptions(data.Store),
	}
}

// themeSettingsSidebarChoices 返回主题设置页侧边栏下拉框使用的自定义侧边栏列表。
//
// @param data 当前模板视图数据，用于复用后台侧边栏名称兼容逻辑。
// @param sidebars settings.json 中保存的侧边栏列表。
// @returns 返回不包含 default-sidebar 的可选自定义侧边栏列表。
func themeSettingsSidebarChoices(data ViewData, sidebars []site.Menu) []site.Menu {
	adminSidebars := sidebarAdminMenus(data, sidebars)
	choices := make([]site.Menu, 0, len(adminSidebars))
	for _, sidebar := range adminSidebars {
		if sidebar.ID == defaultSidebarID {
			continue
		}
		choices = append(choices, sidebar)
	}
	return choices
}

const (
	defaultThemeMenuID = "default-menu"
	defaultSidebarID   = "default-sidebar"
)

// themeSettingsWithDefaultMenu 返回后台菜单编辑器使用的主题菜单设置。
//
// 设计说明：
//   - “默认菜单”原本只是未指定 navbar 时的 fallback，不存在于 settings.json。
//   - 菜单编辑页需要把它当作真实菜单结构展示，因此这里在返回给前端的数据中补上
//     default-menu。
//   - default-menu 是运行时只读菜单，不会从 settings.json 读取或写回。
//
// @param data 当前模板视图数据，用于生成本地化默认标签和页面菜单项。
// @returns 返回包含 default-menu 的 ThemeSettings 副本，不会直接修改 Store。
func themeSettingsWithDefaultMenu(data ViewData) site.ThemeSettings {
	settings := data.Store.Settings.ThemeSettings
	menus := make([]site.Menu, 0, len(settings.Menus)+1)
	menus = append(menus, defaultMenu(data))
	for _, menu := range settings.Menus {
		if menu.ID == defaultThemeMenuID {
			continue
		}
		menus = append(menus, menu)
	}
	settings.Menus = menus
	settings.Sidebars = sidebarAdminMenus(data, settings.Sidebars)
	return settings
}

// themeSettingsWithoutDefaultMenu 返回主题位置设置页使用的主题菜单和侧边栏设置。
//
// 设计说明：
// 主题位置下拉框已经有 “Default menu” 哨兵选项；如果把 default-menu 也作为普通
// 菜单传给前端，会出现两个“默认菜单”。侧边栏也同理过滤 default-sidebar，让
// 哨兵选项继续代表默认侧边栏结构。
//
// @param settings 原始主题设置。
// @returns 返回过滤 default-menu 后的 ThemeSettings 副本。
func themeSettingsWithoutDefaultMenu(settings site.ThemeSettings) site.ThemeSettings {
	menus := make([]site.Menu, 0, len(settings.Menus))
	for _, menu := range settings.Menus {
		if menu.ID == defaultThemeMenuID {
			continue
		}
		menus = append(menus, menu)
	}
	settings.Menus = menus
	sidebars := make([]site.Menu, 0, len(settings.Sidebars))
	for _, sidebar := range settings.Sidebars {
		if sidebar.ID == defaultSidebarID {
			continue
		}
		sidebars = append(sidebars, sidebar)
	}
	settings.Sidebars = sidebars
	return settings
}

// filterPersistedMenus 过滤不允许写入 settings.json 的运行时菜单。
//
// @param menus 前端提交的菜单列表。
// @returns 返回去除 default-menu 后的可持久化菜单列表。
func filterPersistedMenus(menus []site.Menu) []site.Menu {
	filtered := make([]site.Menu, 0, len(menus))
	for _, menu := range menus {
		if menu.ID == defaultThemeMenuID {
			continue
		}
		filtered = append(filtered, menu)
	}
	return filtered
}

// filterPersistedSidebars 过滤不允许写入 settings.json 的运行时侧边栏。
//
// @param sidebars 前端提交的侧边栏列表。
// @returns 返回去除 default-sidebar 后的可持久化侧边栏列表。
func filterPersistedSidebars(sidebars []site.Menu) []site.Menu {
	filtered := make([]site.Menu, 0, len(sidebars))
	for _, sidebar := range sidebars {
		if sidebar.ID == defaultSidebarID {
			continue
		}
		filtered = append(filtered, sidebar)
	}
	return filtered
}

// defaultSidebar 生成只读展示用的默认侧边栏结构。
//
// @param data 当前模板视图数据，用于读取本地化默认名称。
// @param labels 当前主题语言下的侧边栏文案。
// @returns 返回固定 ID 的默认侧边栏。
func defaultSidebar(data ViewData, labels site.SidebarLabels) site.Menu {
	return site.Menu{
		ID:    defaultSidebarID,
		Name:  messageFromViewData(data, "sidebar.section.default", "Default Sidebar"),
		Items: defaultSidebarItemsForLabels(labels),
	}
}

// defaultSidebarItemsForLabels 生成后台默认侧边栏展示用的自动区块列表。
//
// @param labels 当前主题语言下的侧边栏文案。
// @returns 返回标签、页面、订阅三个自动区块。
func defaultSidebarItemsForLabels(labels site.SidebarLabels) []site.MenuItem {
	return []site.MenuItem{
		{Type: site.SidebarSectionTypeTopics, Label: labels.TopicsTitle},
		{Type: site.SidebarSectionTypePages, Label: labels.PagesTitle},
		{Type: site.SidebarSectionTypeFeeds, Label: labels.FeedsTitle},
	}
}

// defaultMenu 生成只读展示用的默认菜单结构。
//
// @param data 当前模板视图数据，用于读取页面列表和本地化导航标签。
// @returns 返回固定 ID 的默认菜单。
func defaultMenu(data ViewData) site.Menu {
	menu := site.Menu{
		ID:   defaultThemeMenuID,
		Name: messageFromViewData(data, "theme_settings.locations.default", "Default menu"),
		Items: []site.MenuItem{
			{Type: "home", Label: messageFromViewData(data, "nav.front_page", "Front Page")},
			{Type: "archive", Label: messageFromViewData(data, "nav.archive", "Archive")},
			{Type: "tags", Label: messageFromViewData(data, "nav.topics", "Topics")},
			{Type: "search", Label: messageFromViewData(data, "nav.search", "Search")},
		},
	}
	if data.Store != nil {
		for _, page := range data.Store.Pages {
			menu.Items = append(menu.Items, site.MenuItem{Type: "page", Label: page.Title, Target: page.Slug})
		}
	}
	menu.Items = append(menu.Items, site.MenuItem{Type: "admin", Label: messageFromViewData(data, "nav.admin", "Admin")})
	return menu
}

// menuLinksFromMenu 把可编辑菜单项解析成前台链接。
//
// @param store 当前站点 Store，用于解析页面、文章、标签等目标。
// @param menu 要解析的菜单。
// @returns 返回过滤无效项后的菜单链接。
func menuLinksFromMenu(store *site.Store, menu site.Menu) []site.MenuLink {
	if store == nil {
		return nil
	}
	links := make([]site.MenuLink, 0, len(menu.Items))
	for _, item := range menu.Items {
		if link, ok := store.ResolveMenuItem(item); ok {
			links = append(links, link)
		}
	}
	return links
}

func menuContentOptions(store *site.Store) map[string]any {
	type option struct {
		Title string `json:"title"`
		Slug  string `json:"slug"`
	}
	pages := make([]option, 0, len(store.Pages))
	for _, page := range store.Pages {
		pages = append(pages, option{Title: page.Title, Slug: page.Slug})
	}
	posts := make([]option, 0, len(store.Posts))
	for _, post := range store.Posts {
		posts = append(posts, option{Title: post.Title, Slug: post.Slug})
	}
	tags := make([]option, 0, len(store.Tags))
	for _, tag := range store.Tags {
		tags = append(tags, option{Title: tag.Title, Slug: tag.Slug})
	}
	return map[string]any{"pages": pages, "posts": posts, "tags": tags}
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

func formatInputDateTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return site.FormatInputDateTime(t)
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
	auth := authConfig{
		user:   env("POSTIZER_ADMIN_USER", "admin"),
		pass:   env("POSTIZER_ADMIN_PASSWORD", "postizer"),
		secret: secret,
	}
	if credentials, ok, err := loadStoredAdminCredentials(contentRoot); err != nil {
		log.Printf("load admin credentials: %v", err)
	} else if ok {
		auth.user = credentials.Username
		auth.pass = ""
		auth.passwordHash = credentials.PasswordHash
	}
	return auth
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

func currentAppVersion() string {
	version := strings.TrimSpace(os.Getenv("POSTIZER_VERSION"))
	if version != "" {
		return version
	}
	version = strings.TrimSpace(AppVersion)
	if version != "" && version != "dev" {
		return version
	}
	if gitVersion := discoverAppVersionFromGit(); gitVersion != "" {
		return gitVersion
	}
	if version != "" {
		return version
	}
	return "dev"
}

func sourceDirectoryCandidates() []string {
	candidates := make([]string, 0, 3)
	if sourceDir := strings.TrimSpace(os.Getenv("POSTIZER_SOURCE_DIR")); sourceDir != "" {
		candidates = append(candidates, sourceDir)
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, cwd)
	}
	candidates = append(candidates, "/usr/local/src/postizer")

	dirs := make([]string, 0, len(candidates))
	seen := map[string]bool{}
	for _, candidate := range candidates {
		dir := filepath.Clean(candidate)
		if dir == "." || seen[dir] {
			continue
		}
		seen[dir] = true
		dirs = append(dirs, dir)
	}
	return dirs
}

func discoverAppVersionFromGit() string {
	for _, dir := range sourceDirectoryCandidates() {
		if version := gitDirectoryVersion(dir); version != "" {
			return version
		}
	}
	return ""
}

func gitDirectoryVersion(dir string) string {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return ""
	}
	exactTag := gitOutput(dir, "describe", "--tags", "--exact-match", "--match", "v[0-9]*.[0-9]*.[0-9]*", "HEAD")
	dirty := gitOutput(dir, "status", "--porcelain") != ""
	if exactTag != "" && !dirty {
		return exactTag
	}
	shortCommit := gitOutput(dir, "rev-parse", "--short", "HEAD")
	if shortCommit == "" {
		return ""
	}
	if dirty {
		return "dev-" + shortCommit + "-dirty"
	}
	return "dev-" + shortCommit
}

func gitOutput(dir string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func adminCredentialsPath(contentRoot string) string {
	return filepath.Join(contentRoot, adminCredentialsFile)
}

func loadStoredAdminCredentials(contentRoot string) (storedAdminCredentials, bool, error) {
	body, err := os.ReadFile(adminCredentialsPath(contentRoot))
	if errors.Is(err, os.ErrNotExist) {
		return storedAdminCredentials{}, false, nil
	}
	if err != nil {
		return storedAdminCredentials{}, false, err
	}
	var credentials storedAdminCredentials
	if err := json.Unmarshal(body, &credentials); err != nil {
		return storedAdminCredentials{}, false, err
	}
	credentials.Username = strings.TrimSpace(credentials.Username)
	credentials.PasswordHash = strings.TrimSpace(credentials.PasswordHash)
	if credentials.Username == "" || credentials.PasswordHash == "" {
		return storedAdminCredentials{}, false, fmt.Errorf("stored admin credentials are incomplete")
	}
	if !validAdminPasswordHash(credentials.PasswordHash) {
		return storedAdminCredentials{}, false, fmt.Errorf("stored admin password hash is invalid")
	}
	return credentials, true, nil
}

func saveStoredAdminCredentials(contentRoot string, credentials storedAdminCredentials) error {
	if err := os.MkdirAll(contentRoot, 0755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(credentials, "", "  ")
	if err != nil {
		return err
	}
	path := adminCredentialsPath(contentRoot)
	if err := os.WriteFile(path, append(body, '\n'), 0600); err != nil {
		return err
	}
	return os.Chmod(path, 0600)
}

func adminLoginAuditPath(contentRoot string) string {
	return filepath.Join(contentRoot, adminLoginAuditFile)
}

func loadStoredLoginAudit(contentRoot string) (storedLoginAudit, error) {
	body, err := os.ReadFile(adminLoginAuditPath(contentRoot))
	if errors.Is(err, os.ErrNotExist) {
		return storedLoginAudit{}, nil
	}
	if err != nil {
		return storedLoginAudit{}, err
	}
	var audit storedLoginAudit
	if err := json.Unmarshal(body, &audit); err != nil {
		return storedLoginAudit{}, err
	}
	if audit.FailedSinceLogin < 0 {
		audit.FailedSinceLogin = 0
	}
	if audit.LastNotice.FailedAttempts < 0 {
		audit.LastNotice.FailedAttempts = 0
	}
	return audit, nil
}

func saveStoredLoginAudit(contentRoot string, audit storedLoginAudit) error {
	if err := os.MkdirAll(contentRoot, 0755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(audit, "", "  ")
	if err != nil {
		return err
	}
	path := adminLoginAuditPath(contentRoot)
	if err := os.WriteFile(path, append(body, '\n'), 0600); err != nil {
		return err
	}
	return os.Chmod(path, 0600)
}

func (s *Server) recordLoginFailureAudit(now time.Time) {
	s.loginAuditMu.Lock()
	defer s.loginAuditMu.Unlock()
	audit, err := loadStoredLoginAudit(s.contentRoot)
	if err != nil {
		log.Printf("load login audit: %v", err)
		return
	}
	audit.FailedSinceLogin++
	if err := saveStoredLoginAudit(s.contentRoot, audit); err != nil {
		log.Printf("save login audit: %v", err)
	}
}

func (s *Server) recordLoginSuccessAudit(now time.Time) {
	s.loginAuditMu.Lock()
	defer s.loginAuditMu.Unlock()
	audit, err := loadStoredLoginAudit(s.contentRoot)
	if err != nil {
		log.Printf("load login audit: %v", err)
		audit = storedLoginAudit{}
	}
	audit.LastNotice = loginAuditNotice{
		PreviousLogin:  audit.LastSuccessfulLogin,
		CurrentLogin:   now,
		FailedAttempts: audit.FailedSinceLogin,
	}
	audit.LastSuccessfulLogin = now
	audit.FailedSinceLogin = 0
	if err := saveStoredLoginAudit(s.contentRoot, audit); err != nil {
		log.Printf("save login audit: %v", err)
	}
}

func (s *Server) loginNotice() LoginNotice {
	s.loginAuditMu.Lock()
	defer s.loginAuditMu.Unlock()
	audit, err := loadStoredLoginAudit(s.contentRoot)
	if err != nil {
		log.Printf("load login audit: %v", err)
		return LoginNotice{}
	}
	notice := audit.LastNotice
	if notice.CurrentLogin.IsZero() && notice.PreviousLogin.IsZero() && notice.FailedAttempts == 0 {
		return LoginNotice{}
	}
	return LoginNotice{
		Show:             true,
		HasPreviousLogin: !notice.PreviousLogin.IsZero(),
		PreviousLogin:    notice.PreviousLogin,
		FailedAttempts:   notice.FailedAttempts,
		DismissKey:       notice.CurrentLogin.UTC().Format(time.RFC3339Nano),
	}
}

func (s *Server) loginAuditTime() time.Time {
	if store := s.currentStore(); store != nil {
		return time.Now().In(site.TimeLocation(store.Settings)).Truncate(time.Second)
	}
	return time.Now().Truncate(time.Second)
}

func loginRateLimitKey(r *http.Request) string {
	addr := strings.TrimSpace(r.RemoteAddr)
	if host, _, err := net.SplitHostPort(addr); err == nil {
		addr = host
	}
	if addr == "" {
		return "unknown"
	}
	return addr
}

func (s *Server) loginRetryAfter(key string, now time.Time) time.Duration {
	s.loginAttemptMu.Lock()
	defer s.loginAttemptMu.Unlock()
	state, ok := s.loginAttempts[key]
	if !ok {
		return 0
	}
	if !state.LockedUntil.IsZero() && now.Before(state.LockedUntil) {
		return state.LockedUntil.Sub(now)
	}
	if state.FirstFailed.IsZero() || now.Sub(state.FirstFailed) > loginFailureWindow {
		delete(s.loginAttempts, key)
	}
	return 0
}

func (s *Server) recordLoginFailure(key string, now time.Time) time.Duration {
	s.loginAttemptMu.Lock()
	defer s.loginAttemptMu.Unlock()
	if s.loginAttempts == nil {
		s.loginAttempts = map[string]loginAttemptState{}
	}
	state := s.loginAttempts[key]
	if state.FirstFailed.IsZero() || now.Sub(state.FirstFailed) > loginFailureWindow || (!state.LockedUntil.IsZero() && !now.Before(state.LockedUntil)) {
		state = loginAttemptState{FirstFailed: now}
	}
	state.Failures++
	if state.Failures >= loginFailureLimit {
		state.LockedUntil = now.Add(loginLockoutDuration)
	}
	s.loginAttempts[key] = state
	if !state.LockedUntil.IsZero() && now.Before(state.LockedUntil) {
		return state.LockedUntil.Sub(now)
	}
	return 0
}

func (s *Server) clearLoginFailures(key string) {
	s.loginAttemptMu.Lock()
	defer s.loginAttemptMu.Unlock()
	delete(s.loginAttempts, key)
}

func writeRetryAfter(w http.ResponseWriter, retryAfter time.Duration) {
	if retryAfter <= 0 {
		return
	}
	seconds := int((retryAfter + time.Second - 1) / time.Second)
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
}

func (s *Server) authSnapshot() authConfig {
	s.authMu.RLock()
	defer s.authMu.RUnlock()
	auth := s.auth
	if auth.secret != nil {
		auth.secret = append([]byte(nil), auth.secret...)
	}
	return auth
}

func (s *Server) setAuth(auth authConfig) {
	s.authMu.Lock()
	defer s.authMu.Unlock()
	s.auth = auth
}

func (auth authConfig) verifyPassword(password string) bool {
	if auth.passwordHash != "" {
		return verifyAdminPasswordHash(auth.passwordHash, password)
	}
	return subtle.ConstantTimeCompare([]byte(password), []byte(auth.pass)) == 1
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.isAdmin(r) {
			s.mirrorAdminSessionCookie(w, r)
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
	for _, cookie := range r.Cookies() {
		if cookie.Name == sessionCookieName && s.verifySession(cookie.Value) {
			return true
		}
	}
	return false
}

func (s *Server) mirrorAdminSessionCookie(w http.ResponseWriter, r *http.Request) {
	for _, cookie := range r.Cookies() {
		if cookie.Name != sessionCookieName || !s.verifySession(cookie.Value) {
			continue
		}
		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    cookie.Value,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
		return
	}
}

func (s *Server) sessionCookie(user string, remember bool) *http.Cookie {
	auth := s.authSnapshot()
	duration := sessionDuration
	if remember {
		duration = rememberSessionDuration
	}
	expires := time.Now().Add(duration)
	payload := user + "|" + strconv.FormatInt(expires.Unix(), 10)
	cookie := &http.Cookie{
		Name:     sessionCookieName,
		Value:    signPayload(payload, auth.secret),
		Path:     "/",
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
	auth := s.authSnapshot()
	payload, ok := verifySignedPayload(value, auth.secret)
	if !ok {
		return false
	}
	user, expRaw, ok := strings.Cut(payload, "|")
	if !ok || subtle.ConstantTimeCompare([]byte(user), []byte(auth.user)) != 1 {
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

func hashAdminPassword(password string) (string, error) {
	salt := make([]byte, adminPasswordHashSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := pbkdf2SHA256Key([]byte(password), salt, adminPasswordHashRounds, adminPasswordHashKeyLen)
	return strings.Join([]string{
		adminPasswordHashScheme,
		strconv.Itoa(adminPasswordHashRounds),
		base64.RawURLEncoding.EncodeToString(salt),
		base64.RawURLEncoding.EncodeToString(key),
	}, "$"), nil
}

func validAdminPasswordHash(encoded string) bool {
	_, _, _, ok := parseAdminPasswordHash(encoded)
	return ok
}

func verifyAdminPasswordHash(encoded, password string) bool {
	rounds, salt, expected, ok := parseAdminPasswordHash(encoded)
	if !ok {
		return false
	}
	key := pbkdf2SHA256Key([]byte(password), salt, rounds, len(expected))
	return subtle.ConstantTimeCompare(key, expected) == 1
}

func parseAdminPasswordHash(encoded string) (int, []byte, []byte, bool) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != adminPasswordHashScheme {
		return 0, nil, nil, false
	}
	rounds, err := strconv.Atoi(parts[1])
	if err != nil || rounds < 10000 {
		return 0, nil, nil, false
	}
	salt, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(salt) < 8 {
		return 0, nil, nil, false
	}
	key, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil || len(key) < 16 {
		return 0, nil, nil, false
	}
	return rounds, salt, key, true
}

func pbkdf2SHA256Key(password, salt []byte, rounds, keyLen int) []byte {
	if rounds <= 0 || keyLen <= 0 {
		return nil
	}
	hashLen := sha256.Size
	blockCount := (keyLen + hashLen - 1) / hashLen
	derived := make([]byte, 0, blockCount*hashLen)
	var counter [4]byte
	for block := 1; block <= blockCount; block++ {
		binary.BigEndian.PutUint32(counter[:], uint32(block))
		mac := hmac.New(sha256.New, password)
		mac.Write(salt)
		mac.Write(counter[:])
		u := mac.Sum(nil)
		t := append([]byte(nil), u...)
		for i := 1; i < rounds; i++ {
			mac = hmac.New(sha256.New, password)
			mac.Write(u)
			u = mac.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		derived = append(derived, t...)
	}
	return derived[:keyLen]
}

func timing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

func staticAssets(next http.Handler) http.Handler {
	return cache(gzipStatic(next))
}

func cache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		next.ServeHTTP(w, r)
	})
}

func gzipStatic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !shouldGzipStatic(r) {
			next.ServeHTTP(w, r)
			return
		}

		addVary(w.Header(), "Accept-Encoding")
		writer := &gzipResponseWriter{ResponseWriter: w}
		defer writer.Close()
		next.ServeHTTP(writer, r)
	})
}

type gzipResponseWriter struct {
	http.ResponseWriter
	writer      *gzip.Writer
	status      int
	wroteHeader bool
}

func (w *gzipResponseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.status = status
	if shouldGzipStatus(status) {
		w.Header().Del("Content-Length")
		w.Header().Set("Content-Encoding", "gzip")
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *gzipResponseWriter) Write(body []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if !shouldGzipStatus(w.status) {
		return w.ResponseWriter.Write(body)
	}
	if w.writer == nil {
		w.writer = gzip.NewWriter(w.ResponseWriter)
	}
	return w.writer.Write(body)
}

func (w *gzipResponseWriter) Flush() {
	if w.writer != nil {
		_ = w.writer.Flush()
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *gzipResponseWriter) Close() error {
	if w.writer == nil {
		return nil
	}
	return w.writer.Close()
}

func shouldGzipStatus(status int) bool {
	return status >= http.StatusOK && status != http.StatusNoContent && status != http.StatusNotModified
}

func shouldGzipStatic(r *http.Request) bool {
	if r.Method == http.MethodHead || r.Header.Get("Range") != "" || !acceptsGzip(r.Header.Get("Accept-Encoding")) {
		return false
	}
	return compressibleStaticPath(r.URL.Path)
}

func acceptsGzip(value string) bool {
	for _, part := range strings.Split(value, ",") {
		token := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		if token == "gzip" {
			return true
		}
	}
	return false
}

func compressibleStaticPath(requestPath string) bool {
	switch strings.ToLower(filepath.Ext(requestPath)) {
	case ".css", ".js", ".mjs", ".json", ".map", ".svg", ".txt", ".html", ".htm", ".xml", ".webmanifest", ".wasm":
		return true
	default:
		return false
	}
}

func addVary(header http.Header, value string) {
	for _, existing := range header.Values("Vary") {
		for _, part := range strings.Split(existing, ",") {
			if strings.EqualFold(strings.TrimSpace(part), value) {
				return
			}
		}
	}
	header.Add("Vary", value)
}
