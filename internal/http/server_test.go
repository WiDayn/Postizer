package http

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"postizer/internal/appearance"
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
