package http

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"postizer/internal/appearance"
	"postizer/internal/media"
	"postizer/internal/site"
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
