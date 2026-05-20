package http

import (
	"bytes"
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"postizer/internal/appearance"
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
		store:              &site.Store{Settings: site.Settings{}},
		contentRoot:        contentRoot,
		builtinBundlesRoot: filepath.Join("internal", "bundles"),
		userContentRoot:    contentRoot,
		userBundlesRoot:    filepath.Join(contentRoot, "bundles"),
	}
	request := httptest.NewRequest("POST", "/admin/api/theme-settings", bytes.NewBufferString(`{
  "menu_locations": {"navbar": ""},
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
