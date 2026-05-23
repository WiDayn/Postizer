package http

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"postizer/internal/appearance"
	"postizer/internal/marketplace"
	"postizer/internal/media"
	"postizer/internal/site"
	"postizer/pkg/pluginrpc"
)

func TestLoadTemplatesParsesAdminTemplates(t *testing.T) {
	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Join(previousDir, "..", "..")); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(previousDir); err != nil {
			t.Fatal(err)
		}
	}()

	if _, err := loadTemplates(appearance.Pack{}); err != nil {
		t.Fatal(err)
	}
}

func TestViewNeedsMathOnlyForMathContentAndEditor(t *testing.T) {
	if viewNeedsMath("home.html", ViewData{Posts: []*site.Post{{Source: "plain text"}}}) {
		t.Fatal("plain home content should not need math assets")
	}
	if !viewNeedsMath("post.html", ViewData{Post: &site.Post{Source: `Inline math: \(x^2\).`}}) {
		t.Fatal("post with math should need math assets")
	}
	if !viewNeedsMath("admin.html", ViewData{EditorKind: "post"}) {
		t.Fatal("editor should need math assets for preview rendering")
	}
}

func TestGzipStaticCompressesTextAssets(t *testing.T) {
	body := strings.Repeat("body{color:#111}", 20)
	handler := gzipStatic(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "999")
		_, _ = w.Write([]byte(body))
	}))

	request := httptest.NewRequest(http.MethodGet, "/static/site.css?v=110", nil)
	request.Header.Set("Accept-Encoding", "br, gzip")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if got, want := response.Header().Get("Content-Encoding"), "gzip"; got != want {
		t.Fatalf("content encoding = %q, want %q", got, want)
	}
	if got := response.Header().Get("Content-Length"); got != "" {
		t.Fatalf("content length = %q, want empty for compressed response", got)
	}
	if got := response.Header().Get("Vary"); !strings.Contains(got, "Accept-Encoding") {
		t.Fatalf("vary = %q, want Accept-Encoding", got)
	}

	reader, err := gzip.NewReader(response.Body)
	if err != nil {
		t.Fatalf("open gzip body: %v", err)
	}
	decompressed, err := io.ReadAll(reader)
	if closeErr := reader.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatalf("read gzip body: %v", err)
	}
	if got := string(decompressed); got != body {
		t.Fatalf("decompressed body = %q, want %q", got, body)
	}
}

func TestPaginateItemsSlicesAndBuildsLinks(t *testing.T) {
	request := httptest.NewRequest("GET", "/admin/posts?page=2&token=abc", nil)
	items, pagination := paginateItems(request, []int{1, 2, 3, 4, 5}, 2)

	if got, want := items, []int{3, 4}; !reflect.DeepEqual(got, want) {
		t.Fatalf("paginated items = %#v, want %#v", got, want)
	}
	if got, want := pagination.Page, 2; got != want {
		t.Fatalf("page = %d, want %d", got, want)
	}
	if got, want := pagination.TotalPages, 3; got != want {
		t.Fatalf("total pages = %d, want %d", got, want)
	}
	if got, want := pagination.StartItem, 3; got != want {
		t.Fatalf("start item = %d, want %d", got, want)
	}
	if got, want := pagination.EndItem, 4; got != want {
		t.Fatalf("end item = %d, want %d", got, want)
	}
	if got, want := pagination.PrevURL, "/admin/posts?token=abc"; got != want {
		t.Fatalf("prev url = %q, want %q", got, want)
	}
	if got, want := pagination.NextURL, "/admin/posts?page=3&token=abc"; got != want {
		t.Fatalf("next url = %q, want %q", got, want)
	}
	wantLinks := []PaginationLink{
		{Page: 1, URL: "/admin/posts?token=abc"},
		{Page: 2, URL: "/admin/posts?page=2&token=abc", Current: true},
		{Page: 3, URL: "/admin/posts?page=3&token=abc"},
	}
	if !reflect.DeepEqual(pagination.Pages, wantLinks) {
		t.Fatalf("pagination links = %#v, want %#v", pagination.Pages, wantLinks)
	}
}

func TestPaginateItemsClampsOutOfRangePage(t *testing.T) {
	request := httptest.NewRequest("GET", "/admin/media?page=99", nil)
	items, pagination := paginateItems(request, []string{"a", "b", "c"}, 2)

	if got, want := items, []string{"c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("paginated items = %#v, want %#v", got, want)
	}
	if got, want := pagination.Page, 2; got != want {
		t.Fatalf("page = %d, want %d", got, want)
	}
	if got, want := pagination.StartItem, 3; got != want {
		t.Fatalf("start item = %d, want %d", got, want)
	}
	if got, want := pagination.EndItem, 3; got != want {
		t.Fatalf("end item = %d, want %d", got, want)
	}
	if got, want := pagination.PrevURL, "/admin/media"; got != want {
		t.Fatalf("prev url = %q, want %q", got, want)
	}
	if pagination.NextURL != "" {
		t.Fatalf("next url = %q, want empty", pagination.NextURL)
	}
}

func TestPaginateItemsHandlesEmptyList(t *testing.T) {
	request := httptest.NewRequest("GET", "/admin/pages?page=4", nil)
	items, pagination := paginateItems(request, []int{}, 2)

	if len(items) != 0 {
		t.Fatalf("paginated items = %#v, want empty", items)
	}
	if got, want := pagination.Page, 1; got != want {
		t.Fatalf("page = %d, want %d", got, want)
	}
	if pagination.Show {
		t.Fatal("pagination should be hidden for an empty list")
	}
	if pagination.PrevURL != "" || pagination.NextURL != "" {
		t.Fatalf("pagination urls = prev %q next %q, want empty", pagination.PrevURL, pagination.NextURL)
	}
}

func TestFilterMediaItemsByType(t *testing.T) {
	items := []media.Item{
		{ID: "image", MIMEType: "image/png", OriginalName: "photo.png"},
		{ID: "audio", MIMEType: "audio/mpeg", OriginalName: "song.mp3"},
		{ID: "document", MIMEType: "application/pdf", OriginalName: "paper.pdf"},
		{ID: "archive", MIMEType: "application/octet-stream", OriginalName: "pack.zip"},
		{ID: "other", MIMEType: "application/octet-stream", OriginalName: "data.bin"},
	}

	filtered := filterMediaItemsByType(items, "document")
	if got, want := mediaItemIDs(filtered), []string{"document"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("document media ids = %#v, want %#v", got, want)
	}

	filtered = filterMediaItemsByType(items, "archive")
	if got, want := mediaItemIDs(filtered), []string{"archive"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("archive media ids = %#v, want %#v", got, want)
	}

	filtered = filterMediaItemsByType(items, "unknown")
	if got, want := mediaItemIDs(filtered), mediaItemIDs(items); !reflect.DeepEqual(got, want) {
		t.Fatalf("unknown filter media ids = %#v, want %#v", got, want)
	}
}

func TestMediaTypeFiltersBuildURLsAndCounts(t *testing.T) {
	request := httptest.NewRequest("GET", "/admin/media?page=3&type=image&token=abc", nil)
	items := []media.Item{
		{ID: "image", MIMEType: "image/webp"},
		{ID: "video", MIMEType: "video/mp4"},
		{ID: "archive", OriginalName: "backup.tar.gz"},
	}

	filters := mediaTypeFilters(request, items, "image")
	all := mediaTypeFilterByID(filters, "")
	if all.Count != 3 {
		t.Fatalf("all count = %d, want 3", all.Count)
	}
	if got, want := all.URL, "/admin/media?token=abc"; got != want {
		t.Fatalf("all url = %q, want %q", got, want)
	}

	image := mediaTypeFilterByID(filters, "image")
	if !image.Active {
		t.Fatal("image filter should be active")
	}
	if image.Count != 1 {
		t.Fatalf("image count = %d, want 1", image.Count)
	}
	if got, want := image.URL, "/admin/media?token=abc&type=image"; got != want {
		t.Fatalf("image url = %q, want %q", got, want)
	}
}

func mediaItemIDs(items []media.Item) []string {
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}
	return ids
}

func mediaTypeFilterByID(filters []MediaTypeFilter, id string) MediaTypeFilter {
	for _, filter := range filters {
		if filter.ID == id {
			return filter
		}
	}
	return MediaTypeFilter{}
}

func containsLog(logs []string, pattern string) bool {
	for _, line := range logs {
		if strings.Contains(line, pattern) {
			return true
		}
	}
	return false
}

func TestHomeImageFromMediaItemUsesMetadata(t *testing.T) {
	item := media.Item{
		Path:         "/media/2026/05/cover.webp",
		OriginalName: "cover.webp",
		Alt:          "Cover",
	}

	homeImage := homeImageFromMediaItem(item)
	if !homeImage.Enabled {
		t.Fatal("home image should be enabled")
	}
	if got, want := homeImage.Src, item.Path; got != want {
		t.Fatalf("home image src = %q, want %q", got, want)
	}
	if got, want := homeImage.Alt, item.Alt; got != want {
		t.Fatalf("home image alt = %q, want %q", got, want)
	}
}

func TestHomeImageFromMediaItemFallsBackToOriginalName(t *testing.T) {
	item := media.Item{
		Path:         "/media/2026/05/cover.webp",
		OriginalName: "cover.webp",
	}

	homeImage := homeImageFromMediaItem(item)
	if got, want := homeImage.Alt, item.OriginalName; got != want {
		t.Fatalf("home image alt = %q, want %q", got, want)
	}
}

func TestMediaFigureMarkdownHasNoOuterBlankLines(t *testing.T) {
	markdown := mediaFigureMarkdown(media.Item{
		ID:           "image-id",
		Path:         "/media/2026/05/example.png",
		OriginalName: "example.png",
		Alt:          "Example",
		Caption:      "Caption",
		MIMEType:     "image/png",
	})
	if markdown != strings.TrimSpace(markdown) {
		t.Fatalf("media figure markdown should be trimmed, got %q", markdown)
	}
	if strings.HasPrefix(markdown, "\n") || strings.HasSuffix(markdown, "\n") {
		t.Fatalf("media figure markdown has outer newlines: %q", markdown)
	}
	if !strings.HasPrefix(markdown, `\begin{figure}`) {
		t.Fatalf("media figure markdown should start with figure syntax, got %q", markdown)
	}
}

func TestMediaFigureMarkdownUsesLinkForNonImage(t *testing.T) {
	markdown := mediaFigureMarkdown(media.Item{
		ID:           "file-id",
		Path:         "/media/2026/05/archive.customext",
		OriginalName: "archive.customext",
		Alt:          "Download archive",
		MIMEType:     "application/octet-stream",
	})

	if strings.Contains(markdown, `\begin{figure}`) {
		t.Fatalf("non-image markdown should not use figure syntax, got %q", markdown)
	}
	if got, want := markdown, `[Download archive](/media/2026/05/archive.customext)`; got != want {
		t.Fatalf("non-image markdown = %q, want %q", got, want)
	}
}

func TestHostServiceSavesMediaContentAndUpdatesJob(t *testing.T) {
	contentRoot := t.TempDir()
	mediaStore, err := media.Open(filepath.Join(contentRoot, "media"))
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{
		store:       &site.Store{Settings: site.Settings{}},
		contentRoot: contentRoot,
		media:       mediaStore,
		pluginJobs:  map[string]*importJob{},
	}

	ctx := context.Background()
	mediaResponse, err := s.SaveMedia(ctx, &pluginrpc.SaveMediaRequest{
		OriginalName: "archive.customext",
		Alt:          "Archive",
		Body:         []byte("hello"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(mediaResponse.Item.Path, ".customext") {
		t.Fatalf("media path should keep custom suffix, got %q", mediaResponse.Item.Path)
	}
	if mediaResponse.Item.Alt != "Archive" {
		t.Fatalf("media alt = %q, want Archive", mediaResponse.Item.Alt)
	}

	postResponse, err := s.SavePost(ctx, &pluginrpc.ContentDraft{
		Title: "Imported",
		Slug:  "imported",
		Date:  "2026-05-06T12:00",
		Body:  "Body",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := postResponse.URL, "/posts/imported"; got != want {
		t.Fatalf("post URL = %q, want %q", got, want)
	}

	body, err := os.ReadFile(filepath.Join(contentRoot, "posts", "imported.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Body") {
		t.Fatalf("post body was not written:\n%s", string(body))
	}

	job, err := s.CreateJob(ctx, &pluginrpc.CreateJobRequest{Title: "Test job", Total: 2})
	if err != nil {
		t.Fatal(err)
	}
	job, err = s.UpdateJob(ctx, &pluginrpc.UpdateJobRequest{
		JobID:  job.ID,
		Done:   1,
		Log:    "Halfway",
		Status: "running",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := job.Done, 1; got != want {
		t.Fatalf("job done = %d, want %d", got, want)
	}
	if !containsLog(job.Logs, "Halfway") {
		t.Fatalf("job logs should include update: %#v", job.Logs)
	}
}

func TestUpdateThemeSettingsSavesCustomPrimitiveSettings(t *testing.T) {
	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Join(previousDir, "..", "..")); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(previousDir); err != nil {
			t.Fatal(err)
		}
	}()

	contentRoot := t.TempDir()
	s := &Server{
		store: &site.Store{Settings: site.Settings{ThemeSettings: site.ThemeSettings{
			Sidebars: []site.Menu{{ID: "right-rail", Name: "右侧栏"}},
		}}},
		contentRoot:        contentRoot,
		builtinBundlesRoot: filepath.Join("internal", "bundles"),
		userContentRoot:    contentRoot,
		userBundlesRoot:    filepath.Join(contentRoot, "bundles"),
	}
	request := httptest.NewRequest("POST", "/admin/api/theme-settings", bytes.NewBufferString(`{
  "menu_locations": {"navbar": ""},
  "sidebar": "right-rail",
  "custom": {
    "pure-white": {
      "hero_title": "Hello",
      "items_per_page": 6,
      "opacity": 0.75
    }
  }
}`))
	response := httptest.NewRecorder()

	s.updateThemeSettings(response, request)

	if response.Code != 200 {
		t.Fatalf("updateThemeSettings status = %d, body = %s", response.Code, response.Body.String())
	}
	settings, err := site.LoadSettings(contentRoot)
	if err != nil {
		t.Fatal(err)
	}
	values := settings.ThemeSettings.Custom["pure-white"]
	if got, want := values["hero_title"].StringValue(), "Hello"; got != want {
		t.Fatalf("hero_title = %q, want %q", got, want)
	}
	if got, want := values["items_per_page"].IntegerValue(), int64(6); got != want {
		t.Fatalf("items_per_page = %d, want %d", got, want)
	}
	if got, want := values["opacity"].FloatValue(), 0.75; got != want {
		t.Fatalf("opacity = %v, want %v", got, want)
	}
	if settings.ThemeSettings.Sidebar == nil {
		t.Fatal("sidebar selection was not saved")
	}
	if got, want := *settings.ThemeSettings.Sidebar, "right-rail"; got != want {
		t.Fatalf("sidebar selection = %q, want %q", got, want)
	}
}

func TestUpdateSiteTitleSettingsSavesTitle(t *testing.T) {
	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Join(previousDir, "..", "..")); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(previousDir); err != nil {
			t.Fatal(err)
		}
	}()

	contentRoot := t.TempDir()
	s := &Server{
		store:              &site.Store{Settings: site.Settings{}},
		contentRoot:        contentRoot,
		builtinBundlesRoot: filepath.Join("internal", "bundles"),
		userContentRoot:    contentRoot,
		userBundlesRoot:    filepath.Join(contentRoot, "bundles"),
	}
	request := httptest.NewRequest("POST", "/admin/api/settings/site-title", bytes.NewBufferString(`{
  "main": "  Field Notes  ",
  "subtitle": "  Daily log  "
}`))
	response := httptest.NewRecorder()

	s.updateSiteTitleSettings(response, request)

	if response.Code != 200 {
		t.Fatalf("updateSiteTitleSettings status = %d, body = %s", response.Code, response.Body.String())
	}
	settings, err := site.LoadSettings(contentRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := settings.SiteTitle.Main, "Field Notes"; got != want {
		t.Fatalf("site title main = %q, want %q", got, want)
	}
	if got, want := settings.SiteTitle.Subtitle, "Daily log"; got != want {
		t.Fatalf("site title subtitle = %q, want %q", got, want)
	}
}

func TestUpdatePermalinkSettingsSavesPermalinks(t *testing.T) {
	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Join(previousDir, "..", "..")); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(previousDir); err != nil {
			t.Fatal(err)
		}
	}()

	contentRoot := t.TempDir()
	s := &Server{
		store:              &site.Store{Settings: site.Settings{}},
		contentRoot:        contentRoot,
		builtinBundlesRoot: filepath.Join("internal", "bundles"),
		userContentRoot:    contentRoot,
		userBundlesRoot:    filepath.Join(contentRoot, "bundles"),
	}
	request := httptest.NewRequest("POST", "/admin/api/settings/permalinks", bytes.NewBufferString(`{
  "post": "/notes/%year%/%monthnum%/%postname%",
  "page": "/docs/%pagename%",
  "tag": "/topics/%tag%"
}`))
	response := httptest.NewRecorder()

	s.updatePermalinkSettings(response, request)

	if response.Code != 200 {
		t.Fatalf("updatePermalinkSettings status = %d, body = %s", response.Code, response.Body.String())
	}
	settings, err := site.LoadSettings(contentRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := settings.Permalinks.Post, "/notes/%year%/%monthnum%/%postname%"; got != want {
		t.Fatalf("post permalink = %q, want %q", got, want)
	}
	if got, want := settings.Permalinks.Page, "/docs/%pagename%"; got != want {
		t.Fatalf("page permalink = %q, want %q", got, want)
	}
	if got, want := settings.Permalinks.Tag, "/topics/%tag%"; got != want {
		t.Fatalf("tag permalink = %q, want %q", got, want)
	}
}

func TestUpdateAutoUpdateSettingsSavesToggle(t *testing.T) {
	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Join(previousDir, "..", "..")); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(previousDir); err != nil {
			t.Fatal(err)
		}
	}()

	contentRoot := t.TempDir()
	s := &Server{
		store:              &site.Store{Settings: site.Settings{}},
		contentRoot:        contentRoot,
		builtinBundlesRoot: filepath.Join("internal", "bundles"),
		userContentRoot:    contentRoot,
		userBundlesRoot:    filepath.Join(contentRoot, "bundles"),
	}
	request := httptest.NewRequest("POST", "/admin/api/settings/auto-update", bytes.NewBufferString(`{"enabled": true}`))
	response := httptest.NewRecorder()

	s.updateAutoUpdateSettings(response, request)

	if response.Code != 200 {
		t.Fatalf("updateAutoUpdateSettings status = %d, body = %s", response.Code, response.Body.String())
	}
	settings, err := site.LoadSettings(contentRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !settings.AutoUpdate.Enabled {
		t.Fatal("auto update should be enabled")
	}
}

func TestUpdateCommentSettingsSavesToggle(t *testing.T) {
	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Join(previousDir, "..", "..")); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(previousDir); err != nil {
			t.Fatal(err)
		}
	}()

	contentRoot := t.TempDir()
	s := &Server{
		store:              &site.Store{Settings: site.Settings{}},
		contentRoot:        contentRoot,
		builtinBundlesRoot: filepath.Join("internal", "bundles"),
		userContentRoot:    contentRoot,
		userBundlesRoot:    filepath.Join(contentRoot, "bundles"),
	}
	request := httptest.NewRequest("POST", "/admin/api/settings/comments", bytes.NewBufferString(`{"enabled": true}`))
	response := httptest.NewRecorder()

	s.updateCommentSettings(response, request)

	if response.Code != 200 {
		t.Fatalf("updateCommentSettings status = %d, body = %s", response.Code, response.Body.String())
	}
	settings, err := site.LoadSettings(contentRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !settings.Comments.Enabled {
		t.Fatal("comments should be enabled")
	}
}

func TestUpdateHomePageSettingsSavesPageSize(t *testing.T) {
	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Join(previousDir, "..", "..")); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(previousDir); err != nil {
			t.Fatal(err)
		}
	}()

	contentRoot := t.TempDir()
	s := &Server{
		store:              &site.Store{Settings: site.Settings{}},
		contentRoot:        contentRoot,
		builtinBundlesRoot: filepath.Join("internal", "bundles"),
		userContentRoot:    contentRoot,
		userBundlesRoot:    filepath.Join(contentRoot, "bundles"),
	}
	request := httptest.NewRequest("POST", "/admin/api/settings/home-page", bytes.NewBufferString(`{"page_size": 7}`))
	response := httptest.NewRecorder()

	s.updateHomePageSettings(response, request)

	if response.Code != 200 {
		t.Fatalf("updateHomePageSettings status = %d, body = %s", response.Code, response.Body.String())
	}
	settings, err := site.LoadSettings(contentRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := settings.HomePage.PageSize, 7; got != want {
		t.Fatalf("home page size = %d, want %d", got, want)
	}
}

func TestUpdateAdminAccountSettingsSavesHashedCredentials(t *testing.T) {
	contentRoot := t.TempDir()
	s := &Server{
		contentRoot: contentRoot,
		auth: authConfig{
			user:   "admin",
			pass:   "old-password",
			secret: []byte("test-session-secret-that-is-long-enough"),
		},
	}
	request := httptest.NewRequest("POST", "/admin/api/settings/admin-account", bytes.NewBufferString(`{
  "username": "editor",
  "current_password": "old-password",
  "new_password": "new-password",
  "confirm_password": "new-password"
}`))
	response := httptest.NewRecorder()

	s.updateAdminAccountSettings(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("updateAdminAccountSettings status = %d, body = %s", response.Code, response.Body.String())
	}
	credentials, ok, err := loadStoredAdminCredentials(contentRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("stored admin credentials were not written")
	}
	if got, want := credentials.Username, "editor"; got != want {
		t.Fatalf("username = %q, want %q", got, want)
	}
	if credentials.PasswordHash == "" || strings.Contains(credentials.PasswordHash, "new-password") {
		t.Fatalf("password should be stored as a hash, got %q", credentials.PasswordHash)
	}
	if !verifyAdminPasswordHash(credentials.PasswordHash, "new-password") {
		t.Fatal("stored password hash did not verify new password")
	}
	if verifyAdminPasswordHash(credentials.PasswordHash, "old-password") {
		t.Fatal("stored password hash should not verify old password")
	}
	auth := s.authSnapshot()
	if got, want := auth.user, "editor"; got != want {
		t.Fatalf("runtime auth username = %q, want %q", got, want)
	}
	if !auth.verifyPassword("new-password") {
		t.Fatal("runtime auth should accept new password")
	}
}

func TestUpdateAdminAccountSettingsRejectsWrongCurrentPassword(t *testing.T) {
	contentRoot := t.TempDir()
	s := &Server{
		contentRoot: contentRoot,
		auth: authConfig{
			user:   "admin",
			pass:   "old-password",
			secret: []byte("test-session-secret-that-is-long-enough"),
		},
	}
	request := httptest.NewRequest("POST", "/admin/api/settings/admin-account", bytes.NewBufferString(`{
  "username": "editor",
  "current_password": "wrong-password",
  "new_password": "new-password",
  "confirm_password": "new-password"
}`))
	response := httptest.NewRecorder()

	s.updateAdminAccountSettings(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("updateAdminAccountSettings status = %d, body = %s", response.Code, response.Body.String())
	}
	if _, ok, err := loadStoredAdminCredentials(contentRoot); err != nil || ok {
		t.Fatalf("credentials should not be stored, ok = %v err = %v", ok, err)
	}
}

func TestNewAuthConfigLoadsStoredAdminCredentials(t *testing.T) {
	contentRoot := t.TempDir()
	passwordHash, err := hashAdminPassword("stored-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := saveStoredAdminCredentials(contentRoot, storedAdminCredentials{
		Username:     "stored-admin",
		PasswordHash: passwordHash,
	}); err != nil {
		t.Fatal(err)
	}

	auth := newAuthConfig(contentRoot)

	if got, want := auth.user, "stored-admin"; got != want {
		t.Fatalf("auth user = %q, want %q", got, want)
	}
	if !auth.verifyPassword("stored-password") {
		t.Fatal("auth should verify stored password")
	}
	if auth.verifyPassword("postizer") {
		t.Fatal("auth should not fall back to default password when stored credentials exist")
	}
}

func TestLoginRateLimiterLocksAfterFailures(t *testing.T) {
	s := &Server{}
	key := "203.0.113.7"
	now := time.Date(2026, 5, 22, 4, 30, 0, 0, time.UTC)

	for i := 0; i < loginFailureLimit-1; i++ {
		if retryAfter := s.recordLoginFailure(key, now); retryAfter > 0 {
			t.Fatalf("failure %d should not lock, retry after = %s", i+1, retryAfter)
		}
	}
	retryAfter := s.recordLoginFailure(key, now)
	if retryAfter <= 0 {
		t.Fatal("final allowed failure should lock the login key")
	}
	if got := s.loginRetryAfter(key, now.Add(time.Minute)); got <= 0 {
		t.Fatalf("locked key retry after = %s, want positive", got)
	}
	if got := s.loginRetryAfter(key, now.Add(loginLockoutDuration+time.Second)); got != 0 {
		t.Fatalf("expired lock retry after = %s, want 0", got)
	}
}

func TestLoginPostRateLimitRecordsFailedAudit(t *testing.T) {
	contentRoot := t.TempDir()
	s := &Server{
		contentRoot: contentRoot,
		auth: authConfig{
			user:   "admin",
			pass:   "correct-password",
			secret: []byte("test-session-secret-that-is-long-enough"),
		},
		templates: template.Must(template.New("").Parse(`{{define "login.html"}}{{.ErrorKey}}{{end}}`)),
	}

	for i := 0; i < loginFailureLimit; i++ {
		form := url.Values{"username": {"admin"}, "password": {"wrong-password"}}
		request := httptest.NewRequest("POST", "/admin/login", strings.NewReader(form.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.RemoteAddr = "203.0.113.9:12345"
		response := httptest.NewRecorder()

		s.loginPost(response, request)

		if i < loginFailureLimit-1 && response.Code != http.StatusUnauthorized {
			t.Fatalf("failure %d status = %d, want %d", i+1, response.Code, http.StatusUnauthorized)
		}
		if i == loginFailureLimit-1 && response.Code != http.StatusTooManyRequests {
			t.Fatalf("locked failure status = %d, want %d", response.Code, http.StatusTooManyRequests)
		}
	}

	audit, err := loadStoredLoginAudit(contentRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := audit.FailedSinceLogin, loginFailureLimit; got != want {
		t.Fatalf("failed login audit count = %d, want %d", got, want)
	}
}

func TestLoginAuditNoticeTracksPreviousLoginFailures(t *testing.T) {
	contentRoot := t.TempDir()
	s := &Server{contentRoot: contentRoot}
	firstLogin := time.Date(2026, 5, 22, 5, 0, 0, 0, time.UTC)
	secondLogin := firstLogin.Add(10 * time.Minute)

	s.recordLoginSuccessAudit(firstLogin)
	s.recordLoginFailureAudit(firstLogin.Add(time.Minute))
	s.recordLoginFailureAudit(firstLogin.Add(2 * time.Minute))
	s.recordLoginSuccessAudit(secondLogin)

	notice := s.loginNotice()
	if !notice.Show {
		t.Fatal("login notice should be visible after a successful login")
	}
	if !notice.HasPreviousLogin {
		t.Fatal("login notice should include the previous successful login")
	}
	if !notice.PreviousLogin.Equal(firstLogin) {
		t.Fatalf("previous login = %s, want %s", notice.PreviousLogin, firstLogin)
	}
	if got, want := notice.FailedAttempts, 2; got != want {
		t.Fatalf("failed attempts in notice = %d, want %d", got, want)
	}
	if notice.DismissKey == "" {
		t.Fatal("login notice should include a dismiss key")
	}
	audit, err := loadStoredLoginAudit(contentRoot)
	if err != nil {
		t.Fatal(err)
	}
	if audit.FailedSinceLogin != 0 {
		t.Fatalf("failed attempts should reset after successful login, got %d", audit.FailedSinceLogin)
	}
	if !audit.LastSuccessfulLogin.Equal(secondLogin) {
		t.Fatalf("last successful login = %s, want %s", audit.LastSuccessfulLogin, secondLogin)
	}
}

func TestRenderLoginNoticeOnlyOnDashboard(t *testing.T) {
	contentRoot := t.TempDir()
	s := &Server{
		contentRoot: contentRoot,
		store:       &site.Store{},
		appearance:  &appearance.Catalog{},
		templates: template.Must(template.New("").Parse(
			`{{define "view.html"}}{{if .LoginNotice.Show}}show {{.LoginNotice.DismissKey}}{{else}}hide{{end}}{{end}}`,
		)),
	}
	s.recordLoginSuccessAudit(time.Date(2026, 5, 22, 6, 0, 0, 0, time.UTC))

	response := httptest.NewRecorder()
	s.render(response, "view.html", ViewData{ActiveAdmin: "posts"})
	if got, want := strings.TrimSpace(response.Body.String()), "hide"; got != want {
		t.Fatalf("posts login notice render = %q, want %q", got, want)
	}

	response = httptest.NewRecorder()
	s.render(response, "view.html", ViewData{ActiveAdmin: "dashboard"})
	if got := strings.TrimSpace(response.Body.String()); !strings.HasPrefix(got, "show ") {
		t.Fatalf("dashboard login notice render = %q, want visible notice", got)
	}
}

func TestCurrentAppVersionUsesBuildValueAndEnvOverride(t *testing.T) {
	previous := AppVersion
	AppVersion = "v1.2.3"
	t.Cleanup(func() { AppVersion = previous })

	t.Setenv("POSTIZER_VERSION", "")
	if got, want := currentAppVersion(), "v1.2.3"; got != want {
		t.Fatalf("current app version = %q, want %q", got, want)
	}

	t.Setenv("POSTIZER_VERSION", "v9.9.9")
	if got, want := currentAppVersion(), "v9.9.9"; got != want {
		t.Fatalf("current app version with env override = %q, want %q", got, want)
	}
}

func TestAdminUpdateSettingsRendersCurrentVersion(t *testing.T) {
	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Join(previousDir, "..", "..")); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(previousDir); err != nil {
			t.Fatal(err)
		}
	}()

	previous := AppVersion
	AppVersion = "v1.2.3"
	defer func() { AppVersion = previous }()

	templates, err := loadTemplates(appearance.Pack{})
	if err != nil {
		t.Fatal(err)
	}
	contentRoot := t.TempDir()
	if err := site.AppendUpdateLogEntry(contentRoot, site.UpdateLogEntry{
		Time:    time.Date(2026, 5, 22, 8, 30, 0, 0, time.UTC),
		Event:   site.UpdateEventDetected,
		Version: "v1.2.4",
		Message: "Detected release v1.2.4.",
	}); err != nil {
		t.Fatal(err)
	}
	s := &Server{
		store:       &site.Store{Settings: site.Settings{}},
		templates:   templates,
		contentRoot: contentRoot,
	}
	request := httptest.NewRequest("GET", "/admin/settings/updates", nil)
	response := httptest.NewRecorder()

	s.adminUpdateSettings(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("adminUpdateSettings status = %d, body = %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, "v1.2.3") {
		t.Fatalf("update settings page should include current version:\n%s", body)
	}
	if !strings.Contains(body, "v1.2.4") || !strings.Contains(body, "Detected release v1.2.4.") {
		t.Fatalf("update settings page should include local update log:\n%s", body)
	}
}

func TestHomeUsesConfiguredPageSize(t *testing.T) {
	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Join(previousDir, "..", "..")); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(previousDir); err != nil {
			t.Fatal(err)
		}
	}()

	templates, err := loadTemplates(appearance.Pack{})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{
		store: &site.Store{
			Settings: site.Settings{HomePage: site.HomePageSettings{PageSize: 2}},
			Posts: []*site.Post{
				{Title: "Post One", Slug: "post-one", URL: "/posts/post-one", Summary: "First"},
				{Title: "Post Two", Slug: "post-two", URL: "/posts/post-two", Summary: "Second"},
				{Title: "Post Three", Slug: "post-three", URL: "/posts/post-three", Summary: "Third"},
			},
		},
		templates: templates,
	}
	request := httptest.NewRequest("GET", "/?page=2", nil)
	response := httptest.NewRecorder()

	s.home(response, request)

	if response.Code != 200 {
		t.Fatalf("home status = %d, body = %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	mainStart := strings.Index(body, `<main class="main-column">`)
	mainEnd := strings.Index(body, `</main>`)
	if mainStart < 0 || mainEnd <= mainStart {
		t.Fatalf("home page should render main content:\n%s", body)
	}
	mainBody := body[mainStart:mainEnd]
	if !strings.Contains(mainBody, "Post Three") {
		t.Fatalf("second home page should include third post:\n%s", body)
	}
	if strings.Contains(mainBody, "Post One") || strings.Contains(mainBody, "Post Two") {
		t.Fatalf("second home page should not include first-page posts:\n%s", body)
	}
	if !strings.Contains(body, `aria-current="page">2`) {
		t.Fatalf("home page should render current pagination link:\n%s", body)
	}
}

func TestHomeOmitsRightRailWhenSidebarDisabled(t *testing.T) {
	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Join(previousDir, "..", "..")); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(previousDir); err != nil {
			t.Fatal(err)
		}
	}()

	templates, err := loadTemplates(appearance.Pack{})
	if err != nil {
		t.Fatal(err)
	}
	disabledSidebar := ""
	s := &Server{
		store: &site.Store{
			Settings: site.Settings{
				ThemeSettings: site.ThemeSettings{
					Sidebar: &disabledSidebar,
				},
			},
			Posts: []*site.Post{{Title: "Post One", Slug: "post-one", URL: "/posts/post-one", Summary: "First"}},
		},
		templates: templates,
	}
	request := httptest.NewRequest("GET", "/", nil)
	response := httptest.NewRecorder()

	s.home(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("home status = %d, body = %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if strings.Contains(body, `class="right-rail"`) {
		t.Fatalf("disabled sidebar should not render right rail:\n%s", body)
	}
	if !strings.Contains(body, `site-shell--no-sidebar`) {
		t.Fatalf("disabled sidebar should mark the shell as no-sidebar:\n%s", body)
	}
}

func TestSubmitCommentWritesComment(t *testing.T) {
	contentRoot := t.TempDir()
	if _, err := site.SavePost(contentRoot, site.PostDraft{
		Title: "Hello Comments",
		Date:  "2026-05-20T10:30",
		Body:  "Body.",
	}); err != nil {
		t.Fatal(err)
	}
	settings, err := site.LoadSettings(contentRoot)
	if err != nil {
		t.Fatal(err)
	}
	settings.Comments.Enabled = true
	if err := site.SaveSettings(contentRoot, settings); err != nil {
		t.Fatal(err)
	}
	store, err := site.Load(contentRoot)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{store: store, contentRoot: contentRoot}
	form := url.Values{
		"post_slug": {"hello-comments"},
		"nickname":  {"Reader"},
		"email":     {"reader@example.com"},
		"comment":   {"Nice post."},
	}
	request := httptest.NewRequest("POST", "/comments", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()

	s.submitComment(response, request)

	if response.Code != 303 {
		t.Fatalf("submitComment status = %d, body = %s", response.Code, response.Body.String())
	}
	comments, err := site.CommentsForPost(contentRoot, "hello-comments")
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 1 {
		t.Fatalf("comments = %d, want 1", len(comments))
	}
	if got, want := comments[0].Body, "Nice post."; got != want {
		t.Fatalf("comment body = %q, want %q", got, want)
	}
}

func TestRenderPublicPermalinkUsesCustomPermalink(t *testing.T) {
	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Join(previousDir, "..", "..")); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(previousDir); err != nil {
			t.Fatal(err)
		}
	}()

	contentRoot := t.TempDir()
	if _, err := site.SavePost(contentRoot, site.PostDraft{
		Title: "Custom Route",
		Date:  "2026-05-20T10:30",
		Body:  "Public body.",
	}); err != nil {
		t.Fatal(err)
	}
	settings := site.Settings{
		Permalinks: site.PermalinkSettings{
			Post: "/notes/%year%/%monthnum%/%postname%",
			Page: site.DefaultPagePermalink,
			Tag:  site.DefaultTagPermalink,
		},
	}
	if err := site.SaveSettings(contentRoot, settings); err != nil {
		t.Fatal(err)
	}
	store, err := site.Load(contentRoot)
	if err != nil {
		t.Fatal(err)
	}
	templates, err := loadTemplates(appearance.Pack{})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{store: store, templates: templates}
	request := httptest.NewRequest("GET", "/notes/2026/05/custom-route", nil)
	response := httptest.NewRecorder()

	if !s.renderPublicPermalink(response, request) {
		t.Fatal("custom permalink should render")
	}
	if !strings.Contains(response.Body.String(), "Custom Route") {
		t.Fatalf("response should include post title, got %s", response.Body.String())
	}
}

func TestPostEditButtonOnlyVisibleForAdmin(t *testing.T) {
	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Join(previousDir, "..", "..")); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(previousDir); err != nil {
			t.Fatal(err)
		}
	}()

	contentRoot := t.TempDir()
	templates, err := loadTemplates(appearance.Pack{})
	if err != nil {
		t.Fatal(err)
	}
	post := &site.Post{Title: "Editable Post", Slug: "editable-post", URL: "/posts/editable-post"}
	store := &site.Store{Settings: site.Settings{}, Posts: []*site.Post{post}, PostsBySlug: map[string]*site.Post{post.Slug: post}}
	s := &Server{store: store, templates: templates, contentRoot: contentRoot, auth: newAuthConfig(contentRoot)}

	anonRequest := httptest.NewRequest("GET", "/posts/editable-post", nil)
	anonResponse := httptest.NewRecorder()
	s.renderPost(anonResponse, anonRequest, post, store)
	if strings.Contains(anonResponse.Body.String(), "/admin/posts/editable-post/edit") {
		t.Fatalf("anonymous post page should not include edit link:\n%s", anonResponse.Body.String())
	}

	adminRequest := httptest.NewRequest("GET", "/posts/editable-post", nil)
	adminRequest.AddCookie(s.sessionCookie(s.auth.user, false))
	adminResponse := httptest.NewRecorder()
	s.renderPost(adminResponse, adminRequest, post, store)
	if !strings.Contains(adminResponse.Body.String(), "/admin/posts/editable-post/edit") {
		t.Fatalf("admin post page should include edit link:\n%s", adminResponse.Body.String())
	}
}

func TestSessionCookieUsesSiteWidePath(t *testing.T) {
	s := &Server{auth: newAuthConfig(t.TempDir())}
	cookie := s.sessionCookie(s.auth.user, false)
	if got, want := cookie.Path, "/"; got != want {
		t.Fatalf("session cookie path = %q, want %q", got, want)
	}
}

func TestRequireAdminMirrorsLegacyAdminCookieToSiteRoot(t *testing.T) {
	s := &Server{auth: newAuthConfig(t.TempDir())}
	cookie := s.sessionCookie(s.auth.user, false)
	cookie.Path = "/admin"
	request := httptest.NewRequest("GET", "/admin", nil)
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	handler := s.requireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	handler.ServeHTTP(response, request)

	if got, want := response.Code, http.StatusNoContent; got != want {
		t.Fatalf("admin response status = %d, want %d", got, want)
	}
	cookies := response.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("admin response should mirror a site-wide session cookie")
	}
	if got, want := cookies[0].Path, "/"; got != want {
		t.Fatalf("mirrored cookie path = %q, want %q", got, want)
	}
}

func TestPageTitleFromViewDataUsesConfiguredSiteTitle(t *testing.T) {
	store := &site.Store{Settings: site.Settings{SiteTitle: site.SiteTitle{
		Main:     "Field Notes",
		Subtitle: "Daily log",
	}}}

	if got, want := pageTitleFromViewData(ViewData{Title: "Archive", Store: store}), "Archive | Field Notes | Daily log"; got != want {
		t.Fatalf("archive title = %q, want %q", got, want)
	}
	if got, want := pageTitleFromViewData(ViewData{Title: "Postizer", Store: store, Home: true}), "Field Notes | Daily log"; got != want {
		t.Fatalf("home title = %q, want %q", got, want)
	}
}

func TestUpdateMenusSavesCustomSidebars(t *testing.T) {
	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Join(previousDir, "..", "..")); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(previousDir); err != nil {
			t.Fatal(err)
		}
	}()

	contentRoot := t.TempDir()
	s := &Server{
		store:              &site.Store{Settings: site.Settings{}},
		contentRoot:        contentRoot,
		builtinBundlesRoot: filepath.Join("internal", "bundles"),
		userContentRoot:    contentRoot,
		userBundlesRoot:    filepath.Join(contentRoot, "bundles"),
	}
	request := httptest.NewRequest("POST", "/admin/api/menus", bytes.NewBufferString(`{
  "menus": [
    {
      "id": "menu",
      "name": "新菜单",
      "items": []
    }
  ],
  "sidebars": [
    {
      "id": "sidebar",
      "name": "自定义侧边栏",
      "items": [
        {
          "label": "订阅",
          "main": {
            "type": "custom"
          },
          "items": [
            {
              "label": "RSS",
              "main": {
                "type": "url",
                "url": "/feed.xml"
              }
            }
          ]
        }
      ]
    }
  ]
}`))
	response := httptest.NewRecorder()

	s.updateMenus(response, request)

	if response.Code != 200 {
		t.Fatalf("updateMenus status = %d, body = %s", response.Code, response.Body.String())
	}
	settings, err := site.LoadSettings(contentRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.ThemeSettings.Sidebars) != 1 {
		t.Fatalf("sidebars = %#v, want one sidebar", settings.ThemeSettings.Sidebars)
	}
	sidebar := settings.ThemeSettings.Sidebars[0]
	if got, want := sidebar.ID, "sidebar"; got != want {
		t.Fatalf("sidebar id = %q, want %q", got, want)
	}
	if got, want := sidebar.Name, "自定义侧边栏"; got != want {
		t.Fatalf("sidebar name = %q, want %q", got, want)
	}
	if len(sidebar.Items) != 1 {
		t.Fatalf("sidebar items = %#v, want one item", sidebar.Items)
	}
	if got, want := sidebar.Items[0].Type, site.SidebarSectionTypeCustom; got != want {
		t.Fatalf("sidebar item type = %q, want %q", got, want)
	}
	if got, want := sidebar.Items[0].Label, "订阅"; got != want {
		t.Fatalf("sidebar item label = %q, want %q", got, want)
	}
	if len(sidebar.Items[0].Items) != 1 {
		t.Fatalf("sidebar custom items = %#v, want one item", sidebar.Items[0].Items)
	}
	if got, want := sidebar.Items[0].Items[0].Label, "RSS"; got != want {
		t.Fatalf("sidebar custom item label = %q, want %q", got, want)
	}
	body, err := os.ReadFile(filepath.Join(contentRoot, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"sidebars"`) {
		t.Fatalf("settings.json did not contain sidebars:\n%s", body)
	}
}

// TestUpdateMenusNormalizesBareURLLink 覆盖后台菜单保存接口的自定义链接输入。
//
// 前端菜单编辑器会发送 main.type/main.url 新结构。用户输入裸域名时，接口应保存为
// https 链接并回读到菜单项里，不能在 SaveSettings/LoadSettings 归一化后消失。
func TestUpdateMenusNormalizesBareURLLink(t *testing.T) {
	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Join(previousDir, "..", "..")); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(previousDir); err != nil {
			t.Fatal(err)
		}
	}()

	contentRoot := t.TempDir()
	s := &Server{
		store:              &site.Store{Settings: site.Settings{}},
		contentRoot:        contentRoot,
		builtinBundlesRoot: filepath.Join("internal", "bundles"),
		userContentRoot:    contentRoot,
		userBundlesRoot:    filepath.Join(contentRoot, "bundles"),
	}
	request := httptest.NewRequest("POST", "/admin/api/menus", bytes.NewBufferString(`{
  "menus": [
    {
      "id": "main",
      "name": "Main",
      "items": [
        {
          "label": "Docs",
          "main": {
            "type": "url",
            "url": "example.com/docs?from=menu"
          }
        }
      ]
    }
  ],
  "sidebars": []
}`))
	response := httptest.NewRecorder()

	s.updateMenus(response, request)

	if response.Code != 200 {
		t.Fatalf("updateMenus status = %d, body = %s", response.Code, response.Body.String())
	}
	settings, err := site.LoadSettings(contentRoot)
	if err != nil {
		t.Fatal(err)
	}
	items := settings.ThemeSettings.Menus[0].Items
	if len(items) != 1 {
		t.Fatalf("menu items = %#v, want one custom link", items)
	}
	if got, want := items[0].URL, "https://example.com/docs?from=menu"; got != want {
		t.Fatalf("menu item url = %q, want %q", got, want)
	}
}

func TestUpdateMenusDoesNotPersistDefaultMenu(t *testing.T) {
	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Join(previousDir, "..", "..")); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(previousDir); err != nil {
			t.Fatal(err)
		}
	}()

	contentRoot := t.TempDir()
	s := &Server{
		store:              &site.Store{Settings: site.Settings{}},
		contentRoot:        contentRoot,
		builtinBundlesRoot: filepath.Join("internal", "bundles"),
		userContentRoot:    contentRoot,
		userBundlesRoot:    filepath.Join(contentRoot, "bundles"),
	}
	request := httptest.NewRequest("POST", "/admin/api/menus", bytes.NewBufferString(`{
  "menus": [
    {
      "id": "default-menu",
      "name": "默认菜单",
      "items": [
        {
          "label": "首页",
          "main": {
            "type": "home"
          }
        }
      ]
    },
    {
      "id": "test-menu",
      "name": "测试菜单",
      "items": []
    }
  ],
  "sidebars": []
}`))
	response := httptest.NewRecorder()

	s.updateMenus(response, request)

	if response.Code != 200 {
		t.Fatalf("updateMenus status = %d, body = %s", response.Code, response.Body.String())
	}
	loaded, err := site.LoadSettings(contentRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, menu := range loaded.ThemeSettings.Menus {
		if menu.ID == defaultThemeMenuID {
			t.Fatalf("default menu should not be persisted: %#v", loaded.ThemeSettings.Menus)
		}
	}
	if len(loaded.ThemeSettings.Menus) != 1 {
		t.Fatalf("menus = %#v, want only custom menu", loaded.ThemeSettings.Menus)
	}
	if got, want := loaded.ThemeSettings.Menus[0].ID, "test-menu"; got != want {
		t.Fatalf("menu id = %q, want %q", got, want)
	}
	var returned site.ThemeSettings
	if err := json.Unmarshal(response.Body.Bytes(), &returned); err != nil {
		t.Fatalf("decode updateMenus response: %v", err)
	}
	if len(returned.Menus) != 2 {
		t.Fatalf("response menus = %#v, want default menu plus custom menu", returned.Menus)
	}
	if got, want := returned.Menus[0].ID, defaultThemeMenuID; got != want {
		t.Fatalf("response first menu id = %q, want %q", got, want)
	}
	if got, want := returned.Menus[1].ID, "test-menu"; got != want {
		t.Fatalf("response second menu id = %q, want %q", got, want)
	}
}

func TestUpdateMenusDoesNotPersistDefaultSidebar(t *testing.T) {
	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Join(previousDir, "..", "..")); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(previousDir); err != nil {
			t.Fatal(err)
		}
	}()

	contentRoot := t.TempDir()
	s := &Server{
		store:              &site.Store{Settings: site.Settings{}},
		contentRoot:        contentRoot,
		builtinBundlesRoot: filepath.Join("internal", "bundles"),
		userContentRoot:    contentRoot,
		userBundlesRoot:    filepath.Join(contentRoot, "bundles"),
	}
	request := httptest.NewRequest("POST", "/admin/api/menus", bytes.NewBufferString(`{
  "menus": [],
  "sidebars": [
    {
      "id": "default-sidebar",
      "name": "默认侧边栏",
      "items": [
        {
          "label": "标签",
          "main": {
            "type": "topics"
          }
        }
      ]
    },
    {
      "id": "custom-sidebar",
      "name": "自定义侧边栏",
      "items": [
        {
          "label": "订阅",
          "main": {
            "type": "feeds"
          }
        }
      ]
    }
  ]
}`))
	response := httptest.NewRecorder()

	s.updateMenus(response, request)

	if response.Code != 200 {
		t.Fatalf("updateMenus status = %d, body = %s", response.Code, response.Body.String())
	}
	loaded, err := site.LoadSettings(contentRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.ThemeSettings.Sidebars) != 1 {
		t.Fatalf("sidebars = %#v, want only custom sidebar", loaded.ThemeSettings.Sidebars)
	}
	if got, want := loaded.ThemeSettings.Sidebars[0].ID, "custom-sidebar"; got != want {
		t.Fatalf("sidebar id = %q, want %q", got, want)
	}
	var returned site.ThemeSettings
	if err := json.Unmarshal(response.Body.Bytes(), &returned); err != nil {
		t.Fatalf("decode updateMenus response: %v", err)
	}
	if len(returned.Sidebars) != 2 {
		t.Fatalf("response sidebars = %#v, want default sidebar plus custom sidebar", returned.Sidebars)
	}
	if got, want := returned.Sidebars[0].ID, defaultSidebarID; got != want {
		t.Fatalf("response first sidebar id = %q, want %q", got, want)
	}
	if got, want := returned.Sidebars[1].ID, "custom-sidebar"; got != want {
		t.Fatalf("response second sidebar id = %q, want %q", got, want)
	}
}

func TestResolvedThemeLocaleDoesNotSwitchToPluginLocaleWhenRequestedEmpty(t *testing.T) {
	themes := []appearance.Pack{
		{
			Manifest: appearance.Manifest{
				ID:            appearance.DefaultThemePackID,
				DefaultLocale: "en",
			},
			Locales: []string{"en", "zh-CN"},
		},
	}

	if got, want := resolvedThemeLocale("", appearance.DefaultThemePackID, themes), "en"; got != want {
		t.Fatalf("theme locale = %q, want %q", got, want)
	}
}

func TestResolvedThemeLocaleAcceptsExplicitThemeLocaleFromMergedLocales(t *testing.T) {
	themes := []appearance.Pack{
		{
			Manifest: appearance.Manifest{
				ID:            appearance.DefaultThemePackID,
				DefaultLocale: "en",
			},
			Locales: []string{"en", "zh-CN", "ja"},
		},
	}

	if got, want := resolvedThemeLocale("ja", appearance.DefaultThemePackID, themes), "ja"; got != want {
		t.Fatalf("theme locale = %q, want %q", got, want)
	}
}

func TestResolvedThemeLocaleKeepsExplicitThemeLocaleOverPluginDefault(t *testing.T) {
	themes := []appearance.Pack{
		{
			Manifest: appearance.Manifest{
				ID:            appearance.DefaultThemePackID,
				DefaultLocale: "en",
			},
			Locales: []string{"en", "zh-CN"},
		},
	}

	if got, want := resolvedThemeLocale("zh-CN", appearance.DefaultThemePackID, themes), "zh-CN"; got != want {
		t.Fatalf("theme locale = %q, want %q", got, want)
	}
}

func TestMenuLinksForUnspecifiedDeclaredNavbarUsesDefault(t *testing.T) {
	data := ViewData{
		Store: &site.Store{
			Settings: site.Settings{},
			Pages:    []*site.Page{{Title: "About", Slug: "about"}},
		},
		Appearance: &appearance.Catalog{
			ActiveTheme: appearance.Pack{
				Manifest: appearance.Manifest{
					MenuLocations: []appearance.MenuLocation{{ID: "navbar", Name: "Navbar"}},
				},
			},
		},
	}

	links := menuLinksForLocation(data, "navbar")
	if len(links) == 0 {
		t.Fatal("unspecified declared navbar should use default links")
	}
	if got, want := links[0].URL, "/"; got != want {
		t.Fatalf("first default link = %q, want %q", got, want)
	}
}

func TestMenuLinksForUnspecifiedDeclaredFooterUsesNoMenu(t *testing.T) {
	data := ViewData{
		Store: &site.Store{
			Settings: site.Settings{},
			Pages:    []*site.Page{{Title: "About", Slug: "about"}},
		},
		Appearance: &appearance.Catalog{
			ActiveTheme: appearance.Pack{
				Manifest: appearance.Manifest{
					MenuLocations: []appearance.MenuLocation{{ID: "footer", Name: "Footer"}},
				},
			},
		},
	}

	links := menuLinksForLocation(data, "footer")
	if len(links) != 0 {
		t.Fatalf("unspecified declared footer links = %#v, want none", links)
	}
}

func TestMenuLinksForExplicitDefaultFooterUsesDefault(t *testing.T) {
	data := ViewData{
		Store: &site.Store{
			Settings: site.Settings{
				ThemeSettings: site.ThemeSettings{
					MenuLocations: map[string]string{"footer": defaultThemeMenuID},
				},
			},
			Pages: []*site.Page{{Title: "About", Slug: "about"}},
		},
		Appearance: &appearance.Catalog{
			ActiveTheme: appearance.Pack{
				Manifest: appearance.Manifest{
					MenuLocations: []appearance.MenuLocation{{ID: "footer", Name: "Footer"}},
				},
			},
		},
	}

	links := menuLinksForLocation(data, "footer")
	if len(links) == 0 {
		t.Fatal("explicit default footer should use default links")
	}
	if got, want := links[0].URL, "/"; got != want {
		t.Fatalf("first explicit default footer link = %q, want %q", got, want)
	}
}

func TestMenuAdminDataIncludesProtectedDefaultMenu(t *testing.T) {
	data := ViewData{
		Store: &site.Store{
			Settings: site.Settings{},
			Pages:    []*site.Page{{Title: "About", Slug: "about"}},
		},
	}

	result := menuAdminData(data).(map[string]any)
	if got, want := result["default_menu_id"], defaultThemeMenuID; got != want {
		t.Fatalf("default_menu_id = %#v, want %q", got, want)
	}
	if got, want := result["default_sidebar_id"], defaultSidebarID; got != want {
		t.Fatalf("default_sidebar_id = %#v, want %q", got, want)
	}
	settings := result["settings"].(site.ThemeSettings)
	if len(settings.Menus) == 0 {
		t.Fatalf("menus = %#v, want default menu", settings.Menus)
	}
	menu := settings.Menus[0]
	if got, want := menu.ID, defaultThemeMenuID; got != want {
		t.Fatalf("default menu id = %q, want %q", got, want)
	}
	if len(menu.Items) == 0 {
		t.Fatalf("default menu items = %#v, want generated items", menu.Items)
	}
	if len(settings.Sidebars) == 0 {
		t.Fatalf("sidebars = %#v, want default sidebar", settings.Sidebars)
	}
	sidebar := settings.Sidebars[0]
	if got, want := sidebar.ID, defaultSidebarID; got != want {
		t.Fatalf("default sidebar id = %q, want %q", got, want)
	}
	if len(sidebar.Items) != 3 {
		t.Fatalf("default sidebar items = %#v, want three generated blocks", sidebar.Items)
	}
	for index, item := range sidebar.Items {
		if item.Type == site.SidebarSectionTypeRecentPosts {
			t.Fatalf("default sidebar item[%d] = recent posts, want no recent posts block", index)
		}
	}
}

func TestMenuAdminDataWrapsLegacySidebarsWithoutDefaultBlocks(t *testing.T) {
	data := ViewData{
		Store: &site.Store{
			Settings: site.Settings{
				ThemeSettings: site.ThemeSettings{
					Sidebars: []site.Menu{
						{
							ID:   "legacy",
							Name: "旧侧边栏",
							Items: []site.MenuItem{
								{Type: "url", Label: "RSS", URL: "/feed.xml"},
							},
						},
					},
				},
			},
		},
	}

	result := menuAdminData(data).(map[string]any)
	settings := result["settings"].(site.ThemeSettings)
	if len(settings.Sidebars) != 2 {
		t.Fatalf("sidebars = %#v, want default sidebar plus legacy sidebar", settings.Sidebars)
	}
	legacy := settings.Sidebars[1]
	if len(legacy.Items) != 1 {
		t.Fatalf("legacy sidebar items = %#v, want only custom area", legacy.Items)
	}
	custom := legacy.Items[0]
	if got, want := custom.Type, site.SidebarSectionTypeCustom; got != want {
		t.Fatalf("legacy custom item type = %q, want %q", got, want)
	}
	if len(custom.Items) != 1 {
		t.Fatalf("legacy custom links = %#v, want one preserved link", custom.Items)
	}
	if got, want := custom.Items[0].Label, "RSS"; got != want {
		t.Fatalf("legacy custom link label = %q, want %q", got, want)
	}
}

func TestMenuAdminDataPreservesEmptyCustomSidebar(t *testing.T) {
	data := ViewData{
		Store: &site.Store{
			Settings: site.Settings{
				ThemeSettings: site.ThemeSettings{
					Sidebars: []site.Menu{
						{ID: "empty-sidebar", Name: "Custom Sidebar"},
					},
				},
			},
		},
	}

	result := menuAdminData(data).(map[string]any)
	settings := result["settings"].(site.ThemeSettings)
	if len(settings.Sidebars) != 2 {
		t.Fatalf("sidebars = %#v, want default sidebar plus empty custom sidebar", settings.Sidebars)
	}
	sidebar := settings.Sidebars[1]
	if got, want := sidebar.ID, "empty-sidebar"; got != want {
		t.Fatalf("sidebar id = %q, want %q", got, want)
	}
	if got, want := sidebar.Name, "New Sidebar"; got != want {
		t.Fatalf("sidebar name = %q, want %q", got, want)
	}
	if len(sidebar.Items) != 0 {
		t.Fatalf("empty sidebar items = %#v, want empty items", sidebar.Items)
	}
}

func TestThemeSettingsDataHidesDefaultMenuFromLocationChoices(t *testing.T) {
	data := ViewData{
		Store: &site.Store{
			Settings: site.Settings{
				ThemeSettings: site.ThemeSettings{
					Menus: []site.Menu{
						{ID: defaultThemeMenuID, Name: "默认菜单"},
						{ID: "test-menu", Name: "测试菜单"},
					},
					Sidebars: []site.Menu{
						{ID: defaultSidebarID, Name: "默认侧边栏"},
						{ID: "test-sidebar", Name: "测试侧边栏"},
					},
				},
			},
		},
	}

	result := themeSettingsData(data).(map[string]any)
	settings := result["settings"].(site.ThemeSettings)
	if len(settings.Menus) != 1 {
		t.Fatalf("theme settings menus = %#v, want only custom menu", settings.Menus)
	}
	if got, want := settings.Menus[0].ID, "test-menu"; got != want {
		t.Fatalf("theme settings menu id = %q, want %q", got, want)
	}
	if len(settings.Sidebars) != 1 {
		t.Fatalf("theme settings sidebars = %#v, want only custom sidebar", settings.Sidebars)
	}
	if got, want := settings.Sidebars[0].ID, "test-sidebar"; got != want {
		t.Fatalf("theme settings sidebar id = %q, want %q", got, want)
	}
}

func TestMenuLinksForExplicitEmptyNavbarDoesNotUseDefault(t *testing.T) {
	data := ViewData{
		Store: &site.Store{
			Settings: site.Settings{
				ThemeSettings: site.ThemeSettings{
					MenuLocations: map[string]string{"navbar": ""},
				},
			},
			Pages: []*site.Page{{Title: "About", Slug: "about"}},
		},
		Appearance: &appearance.Catalog{
			ActiveTheme: appearance.Pack{
				Manifest: appearance.Manifest{
					MenuLocations: []appearance.MenuLocation{{ID: "navbar", Name: "Navbar"}},
				},
			},
		},
	}

	if links := menuLinksForLocation(data, "navbar"); len(links) != 0 {
		t.Fatalf("explicit empty navbar links = %#v, want none", links)
	}
}

func TestMenuLinksForUndeclaredNavbarKeepsDefaultFallback(t *testing.T) {
	data := ViewData{
		Store: &site.Store{
			Settings: site.Settings{},
			Pages:    []*site.Page{{Title: "About", Slug: "about"}},
		},
		Appearance: &appearance.Catalog{},
	}

	links := menuLinksForLocation(data, "navbar")
	if len(links) == 0 {
		t.Fatal("undeclared navbar should keep default links")
	}
	if got, want := links[0].URL, "/"; got != want {
		t.Fatalf("first default link = %q, want %q", got, want)
	}
}

func TestSettingsAfterDeletingActiveThemeRestoresDefault(t *testing.T) {
	settings := site.Settings{
		ThemePack: appearance.Selection{
			Enabled: true,
			PackID:  "custom-theme",
		},
		ThemeLocale: "zh-CN",
		PluginOrder: []string{"copy-plugin"},
	}
	pack := appearance.Pack{
		Manifest: appearance.Manifest{
			ID:   "custom-theme",
			Type: appearance.ThemePack,
		},
	}

	next, changed := settingsAfterDeletingPack(settings, pack, "en")
	if !changed {
		t.Fatal("settings should change when the active theme is deleted")
	}
	if next.ThemePack.Enabled {
		t.Fatal("theme pack should be disabled after restoring the default theme")
	}
	if got, want := next.ThemePack.PackID, appearance.DefaultThemePackID; got != want {
		t.Fatalf("theme pack id = %q, want %q", got, want)
	}
	if got, want := next.ThemeLocale, "en"; got != want {
		t.Fatalf("theme locale = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(next.PluginOrder, settings.PluginOrder) {
		t.Fatalf("plugin order changed unexpectedly: %#v", next.PluginOrder)
	}
}

func TestSettingsAfterDeletingActivePluginRemovesItFromOrder(t *testing.T) {
	settings := site.Settings{
		ThemePack: appearance.Selection{
			Enabled: false,
			PackID:  appearance.DefaultThemePackID,
		},
		ThemeLocale: "en",
		PluginOrder: []string{"keep-a", "delete-me", "keep-b"},
	}
	pack := appearance.Pack{
		Manifest: appearance.Manifest{
			ID:   "delete-me",
			Type: appearance.PluginPack,
		},
	}

	next, changed := settingsAfterDeletingPack(settings, pack, "en")
	if !changed {
		t.Fatal("settings should change when an active plugin is deleted")
	}
	if got, want := next.PluginOrder, []string{"keep-a", "keep-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("plugin order = %#v, want %#v", got, want)
	}
}

func TestSettingsAfterDeletingActiveBundleRestoresThemeAndRemovesPlugins(t *testing.T) {
	settings := site.Settings{
		ThemePack: appearance.Selection{
			Enabled: true,
			PackID:  "bundle-theme",
		},
		ThemeLocale: "zh-CN",
		PluginOrder: []string{"keep-a", "bundle-copy", "keep-b"},
	}
	pack := appearance.Pack{
		Manifest: appearance.Manifest{
			ID:   "mixed-bundle",
			Type: appearance.BundlePack,
		},
		BundledThemeIDs:  []string{"bundle-theme"},
		BundledPluginIDs: []string{"bundle-copy"},
	}

	next, changed := settingsAfterDeletingPack(settings, pack, "en")
	if !changed {
		t.Fatal("settings should change when an active mixed bundle is deleted")
	}
	if next.ThemePack.Enabled {
		t.Fatal("theme pack should be disabled after deleting its parent bundle")
	}
	if got, want := next.ThemePack.PackID, appearance.DefaultThemePackID; got != want {
		t.Fatalf("theme pack id = %q, want %q", got, want)
	}
	if got, want := next.PluginOrder, []string{"keep-a", "keep-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("plugin order = %#v, want %#v", got, want)
	}
}

func TestSettingsAfterDeletingPluginOnlyBundleRemovesPluginsOnly(t *testing.T) {
	settings := site.Settings{
		ThemePack: appearance.Selection{
			Enabled: false,
			PackID:  appearance.DefaultThemePackID,
		},
		ThemeLocale: "en",
		PluginOrder: []string{"bundle-copy", "keep"},
	}
	pack := appearance.Pack{
		Manifest: appearance.Manifest{
			ID:   "plugin-bundle",
			Type: appearance.BundlePack,
		},
		BundledPluginIDs: []string{"bundle-copy"},
	}

	next, changed := settingsAfterDeletingPack(settings, pack, "en")
	if !changed {
		t.Fatal("settings should change when an active plugin-only bundle is deleted")
	}
	if !reflect.DeepEqual(next.ThemePack, settings.ThemePack) {
		t.Fatalf("theme pack changed unexpectedly: %#v", next.ThemePack)
	}
	if got, want := next.PluginOrder, []string{"keep"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("plugin order = %#v, want %#v", got, want)
	}
}

func TestSettingsAfterDeletingInactivePackDoesNotChangeSettings(t *testing.T) {
	settings := site.Settings{
		ThemePack: appearance.Selection{
			Enabled: false,
			PackID:  appearance.DefaultThemePackID,
		},
		ThemeLocale: "en",
		PluginOrder: []string{"keep"},
	}
	pack := appearance.Pack{
		Manifest: appearance.Manifest{
			ID:   "inactive",
			Type: appearance.PluginPack,
		},
	}

	next, changed := settingsAfterDeletingPack(settings, pack, "en")
	if changed {
		t.Fatal("settings should not change when the deleted pack is inactive")
	}
	if !reflect.DeepEqual(next, settings) {
		t.Fatalf("settings changed unexpectedly: %#v", next)
	}
}

func TestValidateMarketplaceBundleRequiresMatchingIDAndRepo(t *testing.T) {
	body := marketplaceBundleZip(t, "paper", "https://github.com/example/paper")
	item := marketplace.PackItem{
		ID:   "paper",
		Repo: "https://github.com/example/paper",
	}
	if err := validateMarketplaceBundle(body, item); err != nil {
		t.Fatal(err)
	}

	item.ID = "other"
	if err := validateMarketplaceBundle(body, item); err == nil {
		t.Fatal("validateMarketplaceBundle should reject mismatched IDs")
	}

	item.ID = "paper"
	item.Repo = "https://github.com/example/other"
	if err := validateMarketplaceBundle(body, item); err == nil {
		t.Fatal("validateMarketplaceBundle should reject mismatched repositories")
	}
}

func TestResourceMarketplaceInstallStateDetectsUpdates(t *testing.T) {
	item := marketplace.PackItem{
		ID: "paper",
		Release: marketplace.Release{
			Tag: "v1.2.0",
		},
	}
	catalog := &appearance.Catalog{
		Bundles: []appearance.Pack{
			{
				Manifest: appearance.Manifest{
					ID:      "paper",
					Type:    appearance.BundlePack,
					Version: "1.0.0",
				},
				Source: appearance.SourceUser,
			},
		},
		ActiveTheme: appearance.Pack{BundleID: "paper"},
	}

	installed, version, active, updateAvailable := resourceMarketplaceInstallState(item, catalog)
	if !installed || !active || !updateAvailable {
		t.Fatalf("state = installed %v active %v update %v, want all true", installed, active, updateAvailable)
	}
	if got, want := version, "1.0.0"; got != want {
		t.Fatalf("installed version = %q, want %q", got, want)
	}
}

func marketplaceBundleZip(t *testing.T, id, repo string) []byte {
	t.Helper()
	var body bytes.Buffer
	zw := zip.NewWriter(&body)
	file, err := zw.Create("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.Write([]byte(`{
  "id": "` + id + `",
  "type": "bundle",
  "name": "Paper",
  "version": "1.0.0",
  "source_url": "` + repo + `"
}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes()
}
