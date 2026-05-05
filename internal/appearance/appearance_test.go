package appearance

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCatalogMergesThemeLocaleAndPluginPriority(t *testing.T) {
	root := t.TempDir()

	writePackFile(t, filepath.Join(root, "official", DirThemes, DefaultThemePackID, "manifest.json"), `{
  "id": "newspaper-classic",
  "type": "theme",
  "name": "Newspaper Classic",
  "version": "1.0.0",
  "description": "Default theme",
  "default_locale": "en",
  "translations_dir": "translations",
  "tags": ["default", "official"]
}`)
	writePackFile(t, filepath.Join(root, "official", DirThemes, DefaultThemePackID, "translations", "en.json"), `{
  "nav.front_page": "Front Page",
  "settings.heading": "Settings"
}`)
	writePackFile(t, filepath.Join(root, "official", DirThemes, DefaultThemePackID, "translations", "zh-CN.json"), `{
  "nav.front_page": "首页"
}`)

	writePackFile(t, filepath.Join(root, "user", DirPlugins, "override-a", "manifest.json"), `{
  "id": "override-a",
  "type": "plugin",
  "name": "Override A",
  "version": "1.0.0",
  "description": "Plugin A",
  "translations_dir": "translations"
}`)
	writePackFile(t, filepath.Join(root, "user", DirPlugins, "override-a", "translations", "zh-CN.json"), `{
  "nav.front_page": "插件A首页"
}`)

	writePackFile(t, filepath.Join(root, "user", DirPlugins, "override-b", "manifest.json"), `{
  "id": "override-b",
  "type": "plugin",
  "name": "Override B",
  "version": "1.0.0",
  "description": "Plugin B",
  "translations_dir": "translations"
}`)
	writePackFile(t, filepath.Join(root, "user", DirPlugins, "override-b", "translations", "zh-CN.json"), `{
  "nav.front_page": "插件B首页"
}`)

	catalog, err := LoadCatalog(
		filepath.Join(root, "official"),
		filepath.Join(root, "user"),
		Selection{Enabled: false, PackID: DefaultThemePackID},
		"zh-CN",
		[]string{"override-a", "override-b"},
	)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := catalog.ActiveTheme.ID, DefaultThemePackID; got != want {
		t.Fatalf("active theme = %q, want %q", got, want)
	}
	if got, want := catalog.ThemeLocale, "zh-CN"; got != want {
		t.Fatalf("theme locale = %q, want %q", got, want)
	}
	if got, want := catalog.Messages["nav.front_page"], "插件A首页"; got != want {
		t.Fatalf("front page message = %q, want %q", got, want)
	}
	if got, want := catalog.Messages["settings.heading"], "Settings"; got != want {
		t.Fatalf("fallback message = %q, want %q", got, want)
	}
	if len(catalog.ActivePlugins) != 2 {
		t.Fatalf("active plugins = %d, want 2", len(catalog.ActivePlugins))
	}
}

func TestLoadCatalogSortsThemesByDefaultThenOfficialThenUser(t *testing.T) {
	root := t.TempDir()
	writePackFile(t, filepath.Join(root, "official", DirThemes, DefaultThemePackID, "manifest.json"), `{
  "id": "newspaper-classic",
  "type": "theme",
  "name": "Newspaper Classic",
  "version": "1.0.0",
  "description": "Default theme",
  "default_locale": "en",
  "translations_dir": "translations",
  "tags": ["default", "official"]
}`)
	writePackFile(t, filepath.Join(root, "official", DirThemes, DefaultThemePackID, "translations", "en.json"), `{}`)

	writePackFile(t, filepath.Join(root, "official", DirThemes, "modern-paper", "manifest.json"), `{
  "id": "modern-paper",
  "type": "theme",
  "name": "Modern Paper",
  "version": "1.0.0",
  "description": "Modern theme",
  "default_locale": "en",
  "translations_dir": "translations",
  "tags": ["official"]
}`)
	writePackFile(t, filepath.Join(root, "official", DirThemes, "modern-paper", "translations", "en.json"), `{}`)
	writePackFile(t, filepath.Join(root, "official", DirThemes, "zh-theme", "manifest.json"), `{
  "id": "zh-theme",
  "type": "theme",
  "name": "中文主题",
  "sort_name": "zhongwen zhuti",
  "version": "1.0.0",
  "description": "Chinese theme",
  "default_locale": "en",
  "translations_dir": "translations",
  "tags": ["official"]
}`)
	writePackFile(t, filepath.Join(root, "official", DirThemes, "zh-theme", "translations", "en.json"), `{}`)
	writePackFile(t, filepath.Join(root, "user", DirThemes, "alpha-user", "manifest.json"), `{
  "id": "alpha-user",
  "type": "theme",
  "name": "Alpha User",
  "version": "1.0.0",
  "description": "User theme",
  "default_locale": "en",
  "translations_dir": "translations",
  "tags": ["other"]
}`)
	writePackFile(t, filepath.Join(root, "user", DirThemes, "alpha-user", "translations", "en.json"), `{}`)

	catalog, err := LoadCatalog(
		filepath.Join(root, "official"),
		filepath.Join(root, "user"),
		Selection{Enabled: true, PackID: "modern-paper"},
		"en",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(catalog.Themes) < 4 {
		t.Fatalf("expected at least four themes, got %d", len(catalog.Themes))
	}
	if got, want := catalog.Themes[0].ID, DefaultThemePackID; got != want {
		t.Fatalf("first theme = %q, want %q", got, want)
	}
	if got, want := catalog.Themes[1].ID, "modern-paper"; got != want {
		t.Fatalf("second theme = %q, want %q", got, want)
	}
	if got, want := catalog.Themes[2].ID, "zh-theme"; got != want {
		t.Fatalf("third theme = %q, want %q", got, want)
	}
	if got, want := catalog.Themes[3].ID, "alpha-user"; got != want {
		t.Fatalf("fourth theme = %q, want %q", got, want)
	}
}

func TestLoadCatalogSortsLocalesWithFixedPriority(t *testing.T) {
	root := t.TempDir()
	writePackFile(t, filepath.Join(root, "official", DirThemes, DefaultThemePackID, "manifest.json"), `{
  "id": "newspaper-classic",
  "type": "theme",
  "name": "Newspaper Classic",
  "version": "1.0.0",
  "description": "Default theme",
  "default_locale": "en",
  "translations_dir": "translations",
  "tags": ["default", "official"]
}`)
	writePackFile(t, filepath.Join(root, "official", DirThemes, DefaultThemePackID, "translations", "en.json"), `{}`)
	writePackFile(t, filepath.Join(root, "official", DirThemes, DefaultThemePackID, "translations", "zh-CN.json"), `{}`)
	writePackFile(t, filepath.Join(root, "official", DirThemes, DefaultThemePackID, "translations", "zh-TW.json"), `{}`)
	writePackFile(t, filepath.Join(root, "official", DirThemes, DefaultThemePackID, "translations", "ja.json"), `{}`)
	writePackFile(t, filepath.Join(root, "official", DirThemes, DefaultThemePackID, "translations", "ko.json"), `{}`)

	catalog, err := LoadCatalog(
		filepath.Join(root, "official"),
		filepath.Join(root, "user"),
		Selection{Enabled: false, PackID: DefaultThemePackID},
		"ko",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(catalog.ThemeLocales) != 5 {
		t.Fatalf("theme locales = %d, want 5", len(catalog.ThemeLocales))
	}
	got := []string{
		catalog.ThemeLocales[0].Code,
		catalog.ThemeLocales[1].Code,
		catalog.ThemeLocales[2].Code,
		catalog.ThemeLocales[3].Code,
		catalog.ThemeLocales[4].Code,
	}
	want := []string{"en", "zh-CN", "zh-TW", "ja", "ko"}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("locale order[%d] = %q, want %q", index, got[index], want[index])
		}
	}
}

func TestLoadCatalogIncludesOfficialNewspaperClassicInk(t *testing.T) {
	catalog, err := LoadCatalog(
		filepath.Join("..", "..", "internal", "bundles"),
		t.TempDir(),
		Selection{Enabled: true, PackID: "newspaper-classic-ink"},
		"zh-CN",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := catalog.ActiveTheme.ID, "newspaper-classic-ink"; got != want {
		t.Fatalf("active theme = %q, want %q", got, want)
	}
	if got, want := catalog.ActiveTheme.Name, "Newspaper Classic Ink"; got != want {
		t.Fatalf("active theme name = %q, want %q", got, want)
	}
	if len(catalog.ThemeStyles) != 1 {
		t.Fatalf("theme styles = %d, want 1", len(catalog.ThemeStyles))
	}
	if got, want := catalog.ThemeStyles[0], "/packs/official/bundles/newspaper/themes/ink/ink.css?v=1.0.0"; got != want {
		t.Fatalf("theme style URL = %q, want %q", got, want)
	}
	if len(catalog.ActiveTheme.MenuLocations) != 1 {
		t.Fatalf("menu locations = %d, want 1", len(catalog.ActiveTheme.MenuLocations))
	}
	if got, want := catalog.ActiveTheme.MenuLocations[0].ID, "navbar"; got != want {
		t.Fatalf("first menu location = %q, want %q", got, want)
	}
	if got, want := catalog.Messages["nav.front_page"], "首页"; got != want {
		t.Fatalf("front page message = %q, want %q", got, want)
	}
}

func TestLoadCatalogIncludesOfficialPureWhite(t *testing.T) {
	catalog, err := LoadCatalog(
		filepath.Join("..", "..", "internal", "bundles"),
		t.TempDir(),
		Selection{Enabled: true, PackID: "pure-white"},
		"zh-CN",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := catalog.ActiveTheme.ID, "pure-white"; got != want {
		t.Fatalf("active theme = %q, want %q", got, want)
	}
	if got, want := catalog.ActiveTheme.Name, "Pure White"; got != want {
		t.Fatalf("active theme name = %q, want %q", got, want)
	}
	if len(catalog.ThemeStyles) != 1 {
		t.Fatalf("theme styles = %d, want 1", len(catalog.ThemeStyles))
	}
	if catalog.ActiveTheme.Version == "" {
		t.Fatal("pure-white theme version should not be empty")
	}
	// Pure White 主题版本来自 manifest。测试只关心样式 URL 是否使用当前版本做缓存刷新，
	// 避免每次主题版本升级时都要同步修改 Go 测试里的硬编码版本号。
	wantStyleURL := "/packs/official/bundles/pure-white/themes/pure-white/pure-white.css?v=" + catalog.ActiveTheme.Version
	if got, want := catalog.ThemeStyles[0], wantStyleURL; got != want {
		t.Fatalf("theme style URL = %q, want %q", got, want)
	}
	if len(catalog.ActiveTheme.MenuLocations) != 1 {
		t.Fatalf("menu locations = %d, want 1", len(catalog.ActiveTheme.MenuLocations))
	}
	if got, want := catalog.ActiveTheme.MenuLocations[0].ID, "navbar"; got != want {
		t.Fatalf("first menu location = %q, want %q", got, want)
	}
	if got, want := catalog.Messages["home.lead_kicker"], "文章"; got != want {
		t.Fatalf("home lead kicker = %q, want %q", got, want)
	}
	if got, want := catalog.Messages["nav.front_page"], "首页"; got != want {
		t.Fatalf("default fallback message = %q, want %q", got, want)
	}
}

func TestLoadCatalogFallsBackToDefaultThemeMessagesForPartialTheme(t *testing.T) {
	root := t.TempDir()
	writePackFile(t, filepath.Join(root, "official", DirThemes, DefaultThemePackID, "manifest.json"), `{
  "id": "newspaper-classic",
  "type": "theme",
  "name": "Newspaper Classic",
  "version": "1.0.0",
  "description": "Default theme",
  "default_locale": "en",
  "translations_dir": "translations",
  "tags": ["default", "official"]
}`)
	writePackFile(t, filepath.Join(root, "official", DirThemes, DefaultThemePackID, "translations", "en.json"), `{
  "settings.heading": "Settings",
  "site.edition_line": "Markdown"
}`)
	writePackFile(t, filepath.Join(root, "official", DirThemes, DefaultThemePackID, "translations", "zh-CN.json"), `{
  "settings.heading": "设置"
}`)

	writePackFile(t, filepath.Join(root, "official", DirThemes, "partial-white", "manifest.json"), `{
  "id": "partial-white",
  "type": "theme",
  "name": "Partial White",
  "version": "1.0.0",
  "description": "Partial theme",
  "default_locale": "en",
  "translations_dir": "translations",
  "tags": ["official"]
}`)
	writePackFile(t, filepath.Join(root, "official", DirThemes, "partial-white", "translations", "en.json"), `{
  "site.edition_line": "Dear Reader"
}`)
	writePackFile(t, filepath.Join(root, "official", DirThemes, "partial-white", "translations", "zh-CN.json"), `{
  "site.edition_line": "给读者"
}`)

	catalog, err := LoadCatalog(
		filepath.Join(root, "official"),
		filepath.Join(root, "user"),
		Selection{Enabled: true, PackID: "partial-white"},
		"zh-CN",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := catalog.Messages["settings.heading"], "设置"; got != want {
		t.Fatalf("default theme fallback message = %q, want %q", got, want)
	}
	if got, want := catalog.Messages["site.edition_line"], "给读者"; got != want {
		t.Fatalf("active theme override message = %q, want %q", got, want)
	}
}

func TestLoadCatalogScansBundleThemesPluginsAndPrefersBundledDuplicate(t *testing.T) {
	root := t.TempDir()
	writePackFile(t, filepath.Join(root, "official", DirThemes, DefaultThemePackID, "manifest.json"), `{
  "id": "newspaper-classic",
  "type": "theme",
  "name": "Newspaper Classic",
  "version": "1.0.0",
  "description": "Default theme",
  "default_locale": "en",
  "translations_dir": "translations",
  "tags": ["default", "official"]
}`)
	writePackFile(t, filepath.Join(root, "official", DirThemes, DefaultThemePackID, "translations", "en.json"), `{
  "nav.front_page": "Front Page"
}`)
	writePackFile(t, filepath.Join(root, "official", DirBundles, "paper", "manifest.json"), `{
  "id": "paper",
  "type": "bundle",
  "name": "Paper Bundle",
  "version": "2.0.0",
  "description": "Bundle with two themes and one plugin",
  "packs": [{"path": "themes/classic"}, {"path": "themes/ink"}, {"path": "plugins/copy"}],
  "tags": ["official"]
}`)
	writePackFile(t, filepath.Join(root, "official", DirBundles, "paper", DirThemes, "classic", "manifest.json"), `{
  "id": "newspaper-classic",
  "type": "theme",
  "name": "Newspaper Classic",
  "version": "2.0.0",
  "description": "Bundled default theme",
  "default_locale": "en",
  "translations_dir": "translations",
  "tags": ["default", "official"]
}`)
	writePackFile(t, filepath.Join(root, "official", DirBundles, "paper", DirThemes, "classic", "translations", "en.json"), `{
  "site.edition_line": "Bundled"
}`)
	writePackFile(t, filepath.Join(root, "official", DirBundles, "paper", DirThemes, "ink", "manifest.json"), `{
  "id": "paper-ink",
  "type": "theme",
  "name": "Paper Ink",
  "version": "2.0.0",
  "description": "Bundled ink theme",
  "default_locale": "en",
  "translations_dir": "translations",
  "styles": ["ink.css"],
  "tags": ["official"]
}`)
	writePackFile(t, filepath.Join(root, "official", DirBundles, "paper", DirThemes, "ink", "translations", "en.json"), `{}`)
	writePackFile(t, filepath.Join(root, "official", DirBundles, "paper", DirThemes, "ink", "ink.css"), `html { color-scheme: dark; }`)
	writePackFile(t, filepath.Join(root, "official", DirBundles, "paper", DirPlugins, "copy", "manifest.json"), `{
  "id": "paper-copy",
  "type": "plugin",
  "name": "Paper Copy",
  "version": "2.0.0",
  "description": "Bundled copy override",
  "translations_dir": "translations"
}`)
	writePackFile(t, filepath.Join(root, "official", DirBundles, "paper", DirPlugins, "copy", "translations", "en.json"), `{
  "nav.front_page": "Bundle Front"
}`)

	catalog, err := LoadCatalog(
		filepath.Join(root, "official"),
		filepath.Join(root, "user"),
		Selection{Enabled: false, PackID: DefaultThemePackID},
		"en",
		[]string{"paper-copy"},
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(catalog.Bundles) != 1 {
		t.Fatalf("bundles = %d, want 1", len(catalog.Bundles))
	}
	if got, want := catalog.ActiveTheme.BundleID, "paper"; got != want {
		t.Fatalf("active theme bundle = %q, want %q", got, want)
	}
	if len(catalog.ActivePlugins) != 1 {
		t.Fatalf("active plugins = %d, want 1", len(catalog.ActivePlugins))
	}
	if got, want := catalog.ActivePlugins[0].BundleID, "paper"; got != want {
		t.Fatalf("active plugin bundle = %q, want %q", got, want)
	}
	if got, want := catalog.Bundles[0].BundledPluginIDs[0], "paper-copy"; got != want {
		t.Fatalf("bundle plugin id = %q, want %q", got, want)
	}
	if got, want := catalog.Messages["site.edition_line"], "Bundled"; got != want {
		t.Fatalf("bundled theme message = %q, want %q", got, want)
	}
	if got, want := catalog.Messages["nav.front_page"], "Bundle Front"; got != want {
		t.Fatalf("bundled plugin message = %q, want %q", got, want)
	}
}

func TestInstallPackZIPInstallsBundleWithThemesAndPlugins(t *testing.T) {
	root := t.TempDir()
	userRoot := filepath.Join(root, "user")
	body := buildPackZip(t, map[string]func(io.Writer){
		"manifest.json": func(w io.Writer) {
			_, _ = io.WriteString(w, `{
  "id": "two-pack",
  "type": "bundle",
  "name": "Two Pack",
  "version": "1.0.0",
  "description": "Two themes and one plugin",
  "source_url": "https://github.com/example/two-pack"
}`)
		},
		"themes/alpha/manifest.json": func(w io.Writer) {
			_, _ = io.WriteString(w, `{
  "id": "alpha-theme",
  "type": "theme",
  "name": "Alpha Theme",
  "version": "1.0.0",
  "description": "Alpha",
  "default_locale": "en",
  "translations_dir": "translations"
}`)
		},
		"themes/alpha/translations/en.json": func(w io.Writer) {
			_, _ = io.WriteString(w, `{"site.edition_line": "Alpha"}`)
		},
		"themes/beta/manifest.json": func(w io.Writer) {
			_, _ = io.WriteString(w, `{
  "id": "beta-theme",
  "type": "theme",
  "name": "Beta Theme",
  "version": "1.0.0",
  "description": "Beta",
  "default_locale": "en",
  "translations_dir": "translations",
  "styles": ["beta.css"]
}`)
		},
		"themes/beta/translations/en.json": func(w io.Writer) {
			_, _ = io.WriteString(w, `{}`)
		},
		"themes/beta/beta.css": func(w io.Writer) {
			_, _ = io.WriteString(w, `body { background: white; }`)
		},
		"plugins/copy/manifest.json": func(w io.Writer) {
			_, _ = io.WriteString(w, `{
  "id": "copy-plugin",
  "type": "plugin",
  "name": "Copy Plugin",
  "version": "1.0.0",
  "description": "Copy override",
  "translations_dir": "translations"
}`)
		},
		"plugins/copy/translations/en.json": func(w io.Writer) {
			_, _ = io.WriteString(w, `{"nav.front_page": "Plugin Front"}`)
		},
		"plugins/footer/manifest.json": func(w io.Writer) {
			_, _ = io.WriteString(w, `{
  "id": "footer-plugin",
  "type": "plugin",
  "name": "Footer Plugin",
  "version": "1.0.0",
  "description": "Footer override",
  "translations_dir": "translations"
}`)
		},
		"plugins/footer/translations/en.json": func(w io.Writer) {
			_, _ = io.WriteString(w, `{"site.edition_line": "Footer Plugin"}`)
		},
	})

	installed, err := InstallPackZIP(bytes.NewReader(body), int64(len(body)), userRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := installed.Type, BundlePack; got != want {
		t.Fatalf("installed type = %q, want %q", got, want)
	}
	if got, want := installed.SourceURL, "https://github.com/example/two-pack"; got != want {
		t.Fatalf("installed source url = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(userRoot, DirBundles, "two-pack")); err != nil {
		t.Fatalf("installed bundle should be stored under user bundles dir: %v", err)
	}

	writePackFile(t, filepath.Join(root, "official", DirThemes, DefaultThemePackID, "manifest.json"), `{
  "id": "newspaper-classic",
  "type": "theme",
  "name": "Newspaper Classic",
  "version": "1.0.0",
  "description": "Default theme",
  "default_locale": "en",
  "translations_dir": "translations",
  "tags": ["default", "official"]
}`)
	writePackFile(t, filepath.Join(root, "official", DirThemes, DefaultThemePackID, "translations", "en.json"), `{}`)

	catalog, err := LoadCatalog(
		filepath.Join(root, "official"),
		userRoot,
		Selection{Enabled: true, PackID: "beta-theme"},
		"en",
		[]string{"copy-plugin", "footer-plugin"},
	)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := catalog.ActiveTheme.BundleID, "two-pack"; got != want {
		t.Fatalf("active theme bundle = %q, want %q", got, want)
	}
	if got, want := catalog.Bundles[0].SourceURL, "https://github.com/example/two-pack"; got != want {
		t.Fatalf("bundle source url = %q, want %q", got, want)
	}
	if got, want := catalog.ThemeStyles[0], "/packs/user/bundles/two-pack/themes/beta/beta.css?v=1.0.0"; got != want {
		t.Fatalf("theme style URL = %q, want %q", got, want)
	}
	if len(catalog.ActivePlugins) != 2 {
		t.Fatalf("active plugins = %d, want 2", len(catalog.ActivePlugins))
	}
	if got, want := catalog.ActivePlugins[0].BundleID, "two-pack"; got != want {
		t.Fatalf("active plugin bundle = %q, want %q", got, want)
	}
	if got, want := catalog.ActivePlugins[1].BundleID, "two-pack"; got != want {
		t.Fatalf("second active plugin bundle = %q, want %q", got, want)
	}
	if got, want := catalog.Messages["nav.front_page"], "Plugin Front"; got != want {
		t.Fatalf("bundled plugin message = %q, want %q", got, want)
	}
	if got, want := catalog.Messages["site.edition_line"], "Footer Plugin"; got != want {
		t.Fatalf("second bundled plugin message = %q, want %q", got, want)
	}
}

func TestLoadCatalogTreatsLegacyTextPackAsPlugin(t *testing.T) {
	root := t.TempDir()
	writePackFile(t, filepath.Join(root, "official", DirThemes, DefaultThemePackID, "manifest.json"), `{
  "id": "newspaper-classic",
  "type": "theme",
  "name": "Newspaper Classic",
  "version": "1.0.0",
  "description": "Default theme",
  "default_locale": "en",
  "translations_dir": "translations"
}`)
	writePackFile(t, filepath.Join(root, "official", DirThemes, DefaultThemePackID, "translations", "en.json"), `{}`)
	writePackFile(t, filepath.Join(root, "official", DirThemes, DefaultThemePackID, "translations", "zh-CN.json"), `{}`)
	writePackFile(t, filepath.Join(root, "user", DirTexts, "legacy-copy", "manifest.json"), `{
  "id": "legacy-copy",
  "type": "text",
  "name": "Legacy Copy",
  "version": "1.0.0",
  "description": "Legacy text pack",
  "lang": "zh-CN",
  "messages_file": "messages.json"
}`)
	writePackFile(t, filepath.Join(root, "user", DirTexts, "legacy-copy", "messages.json"), `{
  "nav.front_page": "旧版首页"
}`)

	catalog, err := LoadCatalog(
		filepath.Join(root, "official"),
		filepath.Join(root, "user"),
		Selection{Enabled: false, PackID: DefaultThemePackID},
		"zh-CN",
		[]string{"legacy-copy"},
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(catalog.ActivePlugins) != 1 {
		t.Fatalf("active plugins = %d, want 1", len(catalog.ActivePlugins))
	}
	if got, want := catalog.ActivePlugins[0].ID, "legacy-copy"; got != want {
		t.Fatalf("active plugin = %q, want %q", got, want)
	}
	if got, want := catalog.Messages["nav.front_page"], "旧版首页"; got != want {
		t.Fatalf("legacy plugin message = %q, want %q", got, want)
	}
}

func TestLoadCatalogUsesActivePluginLocalesForTranslationPacks(t *testing.T) {
	root := t.TempDir()
	writePackFile(t, filepath.Join(root, "official", DirThemes, DefaultThemePackID, "manifest.json"), `{
  "id": "newspaper-classic",
  "type": "theme",
  "name": "Newspaper Classic",
  "version": "1.0.0",
  "description": "Default theme",
  "default_locale": "en",
 "translations_dir": "translations"
}`)
	writePackFile(t, filepath.Join(root, "official", DirThemes, DefaultThemePackID, "translations", "en.json"), `{
  "nav.front_page": "Front Page",
  "nav.archive": "Archive"
}`)
	writePackFile(t, filepath.Join(root, "user", DirBundles, "japanese", "manifest.json"), `{
  "id": "japanese",
  "type": "bundle",
  "name": "Japanese Translation Bundle",
  "version": "1.0.0",
  "description": "Bundle with a Japanese plugin translation pack",
  "packs": [{"path": "plugins/japanese"}]
}`)
	writePackFile(t, filepath.Join(root, "user", DirBundles, "japanese", DirPlugins, "japanese", "manifest.json"), `{
  "id": "japanese",
  "type": "plugin",
  "name": "Japanese Translation Pack",
  "version": "1.0.0",
  "description": "Japanese interface translation",
  "default_locale": "ja",
  "translations_dir": "translations"
}`)
	writePackFile(t, filepath.Join(root, "user", DirBundles, "japanese", DirPlugins, "japanese", "translations", "ja.json"), `{
  "nav.front_page": "ホーム",
  "nav.archive": "アーカイブ"
}`)

	catalog, err := LoadCatalog(
		filepath.Join(root, "official"),
		filepath.Join(root, "user"),
		Selection{Enabled: false, PackID: DefaultThemePackID},
		"ja",
		[]string{"japanese"},
	)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := catalog.ThemeLocale, "ja"; got != want {
		t.Fatalf("theme locale = %q, want %q", got, want)
	}
	if got, want := catalog.Messages["nav.front_page"], "ホーム"; got != want {
		t.Fatalf("plugin translation = %q, want %q", got, want)
	}
	foundJapaneseLocale := false
	for _, locale := range catalog.ThemeLocales {
		if locale.Code == "ja" {
			foundJapaneseLocale = true
			break
		}
	}
	if !foundJapaneseLocale {
		t.Fatalf("theme locales should include active plugin locale ja: %#v", catalog.ThemeLocales)
	}
}

func TestLoadCatalogHidesIncompletePluginLocaleFromThemeLocales(t *testing.T) {
	root := t.TempDir()
	writePackFile(t, filepath.Join(root, "official", DirThemes, DefaultThemePackID, "manifest.json"), `{
  "id": "newspaper-classic",
  "type": "theme",
  "name": "Newspaper Classic",
  "version": "1.0.0",
  "description": "Default theme",
  "default_locale": "en",
  "translations_dir": "translations"
}`)
	writePackFile(t, filepath.Join(root, "official", DirThemes, DefaultThemePackID, "translations", "en.json"), `{
  "nav.front_page": "Front Page",
  "nav.archive": "Archive"
}`)
	writePackFile(t, filepath.Join(root, "user", DirBundles, "partial-japanese", "manifest.json"), `{
  "id": "partial-japanese",
  "type": "bundle",
  "name": "Partial Japanese Translation Bundle",
  "version": "1.0.0",
  "description": "Bundle with an incomplete Japanese plugin translation pack",
  "packs": [{"path": "plugins/partial-japanese"}]
}`)
	writePackFile(t, filepath.Join(root, "user", DirBundles, "partial-japanese", DirPlugins, "partial-japanese", "manifest.json"), `{
  "id": "partial-japanese",
  "type": "plugin",
  "name": "Partial Japanese Translation Pack",
  "version": "1.0.0",
  "description": "Incomplete Japanese interface translation",
  "default_locale": "ja",
  "translations_dir": "translations"
}`)
	writePackFile(t, filepath.Join(root, "user", DirBundles, "partial-japanese", DirPlugins, "partial-japanese", "translations", "ja.json"), `{
  "nav.front_page": "ホーム"
}`)

	catalog, err := LoadCatalog(
		filepath.Join(root, "official"),
		filepath.Join(root, "user"),
		Selection{Enabled: false, PackID: DefaultThemePackID},
		"ja",
		[]string{"partial-japanese"},
	)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := catalog.ThemeLocale, "en"; got != want {
		t.Fatalf("theme locale = %q, want fallback %q", got, want)
	}
	for _, locale := range catalog.ThemeLocales {
		if locale.Code == "ja" {
			t.Fatalf("incomplete plugin locale should not be selectable: %#v", catalog.ThemeLocales)
		}
	}
	if got, want := catalog.Messages["nav.front_page"], "Front Page"; got != want {
		t.Fatalf("incomplete plugin should not translate until locale is selectable: %q, want %q", got, want)
	}
}

func TestLoadCatalogAddsPluginLocalesToEachCompatibleTheme(t *testing.T) {
	root := t.TempDir()
	writePackFile(t, filepath.Join(root, "official", DirThemes, DefaultThemePackID, "manifest.json"), `{
  "id": "newspaper-classic",
  "type": "theme",
  "name": "Newspaper Classic",
  "version": "1.0.0",
  "description": "Default theme",
  "default_locale": "en",
  "translations_dir": "translations"
}`)
	writePackFile(t, filepath.Join(root, "official", DirThemes, DefaultThemePackID, "translations", "en.json"), `{
  "nav.front_page": "Front Page",
  "nav.archive": "Archive"
}`)
	writePackFile(t, filepath.Join(root, "official", DirThemes, "pure-white", "manifest.json"), `{
  "id": "pure-white",
  "type": "theme",
  "name": "Pure White",
  "version": "1.0.0",
  "description": "Pure white theme",
  "default_locale": "en",
  "translations_dir": "translations"
}`)
	writePackFile(t, filepath.Join(root, "official", DirThemes, "pure-white", "translations", "en.json"), `{
  "site.brand": "Postizer"
}`)
	writePackFile(t, filepath.Join(root, "user", DirBundles, "japanese", "manifest.json"), `{
  "id": "japanese",
  "type": "bundle",
  "name": "Japanese Translation Bundle",
  "version": "1.0.0",
  "description": "Bundle with a Japanese plugin translation pack",
  "packs": [{"path": "plugins/japanese"}]
}`)
	writePackFile(t, filepath.Join(root, "user", DirBundles, "japanese", DirPlugins, "japanese", "manifest.json"), `{
  "id": "japanese",
  "type": "plugin",
  "name": "Japanese Translation Pack",
  "version": "1.0.0",
  "description": "Japanese interface translation",
  "default_locale": "ja",
  "translations_dir": "translations"
}`)
	writePackFile(t, filepath.Join(root, "user", DirBundles, "japanese", DirPlugins, "japanese", "translations", "ja.json"), `{
  "nav.front_page": "ホーム",
  "nav.archive": "アーカイブ",
  "site.brand": "Postizer"
}`)

	catalog, err := LoadCatalog(
		filepath.Join(root, "official"),
		filepath.Join(root, "user"),
		Selection{Enabled: false, PackID: DefaultThemePackID},
		"en",
		[]string{"japanese"},
	)
	if err != nil {
		t.Fatal(err)
	}

	var pureWhite *Pack
	for i := range catalog.Themes {
		if catalog.Themes[i].ID == "pure-white" {
			pureWhite = &catalog.Themes[i]
			break
		}
	}
	if pureWhite == nil {
		t.Fatal("pure-white theme should be present")
	}
	if !containsString(pureWhite.Locales, "ja") {
		t.Fatalf("compatible inactive theme should include plugin locale ja: %#v", pureWhite.Locales)
	}
}

func TestLoadCatalogRejectsManifestPathEscapes(t *testing.T) {
	root := t.TempDir()
	writePackFile(t, filepath.Join(root, "official", DirThemes, DefaultThemePackID, "manifest.json"), `{
  "id": "newspaper-classic",
  "type": "theme",
  "name": "Newspaper Classic",
  "version": "1.0.0",
  "description": "Default theme",
  "default_locale": "en",
  "translations_dir": "../outside"
}`)

	_, err := LoadCatalog(
		filepath.Join(root, "official"),
		filepath.Join(root, "user"),
		Selection{Enabled: false, PackID: DefaultThemePackID},
		"en",
		nil,
	)
	if err == nil {
		t.Fatal("LoadCatalog should reject manifest paths that escape the pack root")
	}
}

func TestInstallPackZIPRejectsOversizedEntries(t *testing.T) {
	userRoot := t.TempDir()
	body := buildPackZip(t, map[string]func(io.Writer){
		"manifest.json": func(w io.Writer) {
			_, _ = io.WriteString(w, `{
  "id": "huge-pack",
  "type": "bundle",
  "name": "Huge Pack",
  "version": "1.0.0",
  "description": "Too large"
}`)
		},
		"themes/huge/manifest.json": func(w io.Writer) {
			_, _ = io.WriteString(w, `{
  "id": "huge-theme",
  "type": "theme",
  "name": "Huge Theme",
  "version": "1.0.0",
  "description": "Huge",
  "default_locale": "en",
  "translations_dir": "translations"
}`)
		},
		"huge.bin": func(w io.Writer) {
			_, _ = io.CopyN(w, zeroReader{}, maxPackZipSingleFileBytes+1)
		},
	})

	_, err := InstallPackZIP(bytes.NewReader(body), int64(len(body)), userRoot)
	if err == nil {
		t.Fatal("InstallPackZIP should reject entries beyond the single-file extraction limit")
	}
	if _, statErr := os.Stat(filepath.Join(userRoot, DirBundles, "huge-pack")); !os.IsNotExist(statErr) {
		t.Fatalf("oversized pack should not be installed, stat error = %v", statErr)
	}
}

func TestInstallPackZIPRejectsSinglePluginUpload(t *testing.T) {
	userRoot := t.TempDir()
	body := buildPackZip(t, map[string]func(io.Writer){
		"manifest.json": func(w io.Writer) {
			_, _ = io.WriteString(w, `{
  "id": "single-plugin",
  "type": "plugin",
  "name": "Single Plugin",
  "version": "1.0.0",
  "description": "Direct plugin upload"
}`)
		},
	})

	_, err := InstallPackZIP(bytes.NewReader(body), int64(len(body)), userRoot)
	if err == nil {
		t.Fatal("InstallPackZIP should reject direct plugin uploads")
	}
	if _, statErr := os.Stat(filepath.Join(userRoot, DirPlugins, "single-plugin")); !os.IsNotExist(statErr) {
		t.Fatalf("direct plugin should not be installed, stat error = %v", statErr)
	}
}

func TestInstallPackZIPRejectsNonGitHubBundleSourceURL(t *testing.T) {
	userRoot := t.TempDir()
	body := buildPackZip(t, map[string]func(io.Writer){
		"manifest.json": func(w io.Writer) {
			_, _ = io.WriteString(w, `{
  "id": "bad-source",
  "type": "bundle",
  "name": "Bad Source",
  "version": "1.0.0",
  "description": "Bad source URL",
  "source_url": "https://example.com/not-github"
}`)
		},
		"themes/basic/manifest.json": func(w io.Writer) {
			_, _ = io.WriteString(w, `{
  "id": "bad-source-theme",
  "type": "theme",
  "name": "Bad Source Theme",
  "version": "1.0.0",
  "description": "Theme",
  "default_locale": "en",
  "translations_dir": "translations"
}`)
		},
		"themes/basic/translations/en.json": func(w io.Writer) {
			_, _ = io.WriteString(w, `{}`)
		},
	})

	_, err := InstallPackZIP(bytes.NewReader(body), int64(len(body)), userRoot)
	if err == nil {
		t.Fatal("InstallPackZIP should reject non-GitHub source_url values")
	}
	if _, statErr := os.Stat(filepath.Join(userRoot, DirBundles, "bad-source")); !os.IsNotExist(statErr) {
		t.Fatalf("invalid source bundle should not be installed, stat error = %v", statErr)
	}
}

func TestDeleteUserPackRemovesOnlyValidatedUserPackDir(t *testing.T) {
	userRoot := t.TempDir()
	packDir := filepath.Join(userRoot, DirPlugins, "local-copy")
	writePackFile(t, filepath.Join(packDir, "manifest.json"), `{
  "id": "local-copy",
  "type": "plugin",
  "name": "Local Copy",
  "version": "1.0.0",
  "description": "Local plugin"
}`)

	if err := DeleteUserPack(userRoot, PluginPack, "local-copy"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(packDir); !os.IsNotExist(err) {
		t.Fatalf("deleted pack should be gone, stat error = %v", err)
	}
}

func TestDeleteUserPackRejectsInvalidID(t *testing.T) {
	userRoot := t.TempDir()
	if err := DeleteUserPack(userRoot, PluginPack, "../outside"); err == nil {
		t.Fatal("DeleteUserPack should reject path-like ids")
	}
}

func writePackFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func buildPackZip(t *testing.T, files map[string]func(io.Writer)) []byte {
	t.Helper()
	var body bytes.Buffer
	zw := zip.NewWriter(&body)
	for name, write := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		write(w)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes()
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for index := range p {
		p[index] = 0
	}
	return len(p), nil
}
