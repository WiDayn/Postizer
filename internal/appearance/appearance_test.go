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
		filepath.Join("..", "..", "packs", "official"),
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
	if got, want := catalog.ThemeStyles[0], "/packs/official/themes/newspaper-classic-ink/ink.css"; got != want {
		t.Fatalf("theme style URL = %q, want %q", got, want)
	}
	if got, want := catalog.Messages["nav.front_page"], "首页"; got != want {
		t.Fatalf("front page message = %q, want %q", got, want)
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
  "type": "plugin",
  "name": "Huge Pack",
  "version": "1.0.0",
  "description": "Too large"
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
	if _, statErr := os.Stat(filepath.Join(userRoot, DirPlugins, "huge-pack")); !os.IsNotExist(statErr) {
		t.Fatalf("oversized pack should not be installed, stat error = %v", statErr)
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

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for index := range p {
		p[index] = 0
	}
	return len(p), nil
}
