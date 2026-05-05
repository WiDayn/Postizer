package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"postizer/internal/appearance"
)

func TestSavePostAndLoadDraft(t *testing.T) {
	root := t.TempDir()
	_, err := SavePost(root, PostDraft{
		Title:   "Editor Smoke",
		Slug:    "editor-smoke",
		Date:    "2026-05-04T10:30",
		Tags:    []string{"go", "editor"},
		Summary: "Saved from the editor.",
		Draft:   true,
		TOC:     true,
		Body:    "## Body\n\nDraft content.",
	})
	if err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(filepath.Join(root, "posts", "editor-smoke.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `draft: true`) {
		t.Fatalf("saved post did not preserve draft state:\n%s", body)
	}
	if !strings.Contains(string(body), `date: "2026-05-04T10:30"`) {
		t.Fatalf("saved post did not preserve minute-level date:\n%s", body)
	}
	if !strings.Contains(string(body), `updated: "`) {
		t.Fatalf("saved post did not auto-populate updated time:\n%s", body)
	}

	store, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if store.AllPostsBySlug["editor-smoke"] == nil {
		t.Fatal("draft post was not available to admin index")
	}
	if store.PostsBySlug["editor-smoke"] != nil {
		t.Fatal("draft post leaked into public index")
	}
}

func TestSavePostPreservesManualUpdatedMinute(t *testing.T) {
	root := t.TempDir()
	saved, err := SavePost(root, PostDraft{
		Title:         "Manual Updated",
		Slug:          "manual-updated",
		Date:          "2026-05-04",
		Updated:       "2026-05-05 12:34",
		UpdatedManual: true,
		Body:          "Body.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := saved.Date, "2026-05-04T00:00"; got != want {
		t.Fatalf("date = %q, want %q", got, want)
	}
	if got, want := saved.Updated, "2026-05-05T12:34"; got != want {
		t.Fatalf("updated = %q, want %q", got, want)
	}
	body, err := os.ReadFile(filepath.Join(root, "posts", "manual-updated.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `updated: "2026-05-05T12:34"`) {
		t.Fatalf("manual updated time was not serialized:\n%s", body)
	}
}

func TestLoadParsesMinuteDatesInConfiguredTimeZone(t *testing.T) {
	root := t.TempDir()
	settings := defaultSettings()
	settings.TimeZone = "Asia/Tokyo"
	if err := SaveSettings(root, settings); err != nil {
		t.Fatal(err)
	}
	postDir := filepath.Join(root, "posts")
	if err := os.MkdirAll(postDir, 0755); err != nil {
		t.Fatal(err)
	}
	body := `---
title: "Time Zone"
slug: "time-zone"
date: "2026-05-04T08:30"
updated: "2026-05-04T09:45"
tags: []
summary: ""
draft: false
toc: true
---

Body.
`
	if err := os.WriteFile(filepath.Join(postDir, "time-zone.md"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	store, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	post := store.PostsBySlug["time-zone"]
	if post == nil {
		t.Fatal("post not loaded")
	}
	if got, want := post.Date.Location().String(), "Asia/Tokyo"; got != want {
		t.Fatalf("location = %q, want %q", got, want)
	}
	if got, want := FormatInputDateTime(post.Date), "2026-05-04T08:30"; got != want {
		t.Fatalf("date = %q, want %q", got, want)
	}
	if got, want := FormatDisplayTime(post.Updated), "2026-05-04 09:45"; got != want {
		t.Fatalf("updated display = %q, want %q", got, want)
	}
}

func TestRenderMarkdownPreservesKatexDelimiters(t *testing.T) {
	html, err := RenderMarkdown(`Inline math: \(x^2\).`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(html), `\(x^2\)`) {
		t.Fatalf("math delimiters were not preserved: %s", html)
	}
}

func TestRenderMarkdownNumbersLabelledDisplayEquations(t *testing.T) {
	html, err := RenderMarkdown(`Reference before equation: \eqref{eq:energy}.

$$
E = mc^2
\label{eq:energy}
$$

Reference after equation: \ref{eq:energy}.`)
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(html)
	for _, expected := range []string{
		`<span id="eq-eq-energy" class="equation-anchor"></span>`,
		`\tag{1}`,
		`<a href="#eq-eq-energy">(1)</a>`,
		`<a href="#eq-eq-energy">1</a>`,
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered HTML did not contain %q:\n%s", expected, rendered)
		}
	}
	if strings.Contains(rendered, `\label{eq:energy}`) {
		t.Fatalf("equation label leaked into rendered HTML:\n%s", rendered)
	}
}

func TestRenderMarkdownNumbersAllDisplayEquations(t *testing.T) {
	html, err := RenderMarkdown(`Single-line display:

$$
E = mc^2
$$

Multi-line display:

\[
\begin{aligned}
a &= b + c \\
d &= e + f
\end{aligned}
\]`)
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(html)
	for _, expected := range []string{`\tag{1}`, `\tag{2}`} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("display equation was not numbered with %q:\n%s", expected, rendered)
		}
	}
	if strings.Contains(rendered, `\label{`) {
		t.Fatalf("unexpected label leaked into rendered HTML:\n%s", rendered)
	}
}

func TestRenderMarkdownPreservesExplicitEquationTags(t *testing.T) {
	html, err := RenderMarkdown(`$$
a^2 + b^2 = c^2
\tag{P}
\label{eq:pythagoras}
$$

See \eqref{eq:pythagoras}.`)
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(html)
	if strings.Count(rendered, `\tag{P}`) != 1 {
		t.Fatalf("explicit tag was not preserved exactly once:\n%s", rendered)
	}
	if strings.Contains(rendered, `\tag{1}`) {
		t.Fatalf("auto tag was added despite explicit tag:\n%s", rendered)
	}
	if !strings.Contains(rendered, `<a href="#eq-eq-pythagoras">(P)</a>`) {
		t.Fatalf("explicit tag was not used for eqref:\n%s", rendered)
	}
}

func TestRenderMarkdownDoesNotReplaceEquationReferencesInCode(t *testing.T) {
	html, err := RenderMarkdown("```tex\n\\eqref{eq:energy}\n```\n\n$$\nE = mc^2\n\\label{eq:energy}\n$$")
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(html)
	if !strings.Contains(rendered, `\eqref{eq:energy}`) {
		t.Fatalf("equation reference inside code was replaced:\n%s", rendered)
	}
}

func TestRenderMarkdownDoesNotNumberDisplayEquationsInCode(t *testing.T) {
	html, err := RenderMarkdown("```tex\n$$\nE = mc^2\n$$\n```\n\nInline math stays inline: \\(x^2\\).")
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(html)
	if strings.Contains(rendered, `\tag{1}`) {
		t.Fatalf("display equation inside code was numbered:\n%s", rendered)
	}
}

func TestRenderMarkdownRendersFigureBlocksWithReferences(t *testing.T) {
	html, err := RenderMarkdown(`See \figref{fig:sample} and \ref{fig:sample}.

\begin{figure}
![Sample image](/media/2026/05/sample.webp)
\caption{A sample figure.}
\label{fig:sample}
\end{figure}`)
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(html)
	for _, expected := range []string{
		`<figure id="fig-fig-sample" class="article-figure">`,
		`<img src="/media/2026/05/sample.webp" alt="Sample image" loading="lazy">`,
		`<span class="figure-number">Figure 1.</span> A sample figure.`,
		`<a href="#fig-fig-sample">Figure 1</a>`,
		`<a href="#fig-fig-sample">1</a>`,
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered HTML did not contain %q:\n%s", expected, rendered)
		}
	}
	if strings.Contains(rendered, `\caption`) || strings.Contains(rendered, `\label`) {
		t.Fatalf("figure control syntax leaked into rendered HTML:\n%s", rendered)
	}
}

func TestRenderMarkdownTreatsStandaloneImagesAsFigures(t *testing.T) {
	html, err := RenderMarkdown(`![Standalone image](/media/2026/05/standalone.webp)`)
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(html)
	for _, expected := range []string{
		`<figure class="article-figure">`,
		`<img src="/media/2026/05/standalone.webp" alt="Standalone image" loading="lazy">`,
		`<span class="figure-number">Figure 1.</span> Standalone image`,
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("standalone image was not rendered as a figure with %q:\n%s", expected, rendered)
		}
	}
}

func TestRenderMarkdownDoesNotRenderFiguresInCode(t *testing.T) {
	html, err := RenderMarkdown("```markdown\n![Code image](/media/code.webp)\n\\figref{fig:code}\n```")
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(html)
	if strings.Contains(rendered, `article-figure`) || !strings.Contains(rendered, `\figref{fig:code}`) {
		t.Fatalf("figure syntax inside code was modified:\n%s", rendered)
	}
}

func TestLoadSettingsMigratesLegacyThemeField(t *testing.T) {
	root := t.TempDir()
	body := `{
  "theme": "newspaper"
}`
	if err := os.WriteFile(filepath.Join(root, "settings.json"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}

	settings, err := LoadSettings(root)
	if err != nil {
		t.Fatal(err)
	}
	if settings.ThemePack.Enabled {
		t.Fatal("legacy newspaper theme should fall back to default pack without forcing enabled=true")
	}
	if got, want := settings.ThemePack.PackID, appearance.DefaultThemePackID; got != want {
		t.Fatalf("theme pack id = %q, want %q", got, want)
	}
	if got, want := settings.ThemeLocale, "en"; got != want {
		t.Fatalf("theme locale = %q, want %q", got, want)
	}
}

func TestLoadSettingsMigratesLegacyTextPackIntoThemeLocaleAndPluginOrder(t *testing.T) {
	root := t.TempDir()
	body := `{
  "text_pack": {
    "enabled": true,
    "pack_id": "custom-copy"
  },
  "theme_locale": "zh_cn",
  "plugin_order": ["custom-copy", "extra-copy", "custom-copy"]
}`
	if err := os.WriteFile(filepath.Join(root, "settings.json"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}

	settings, err := LoadSettings(root)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := settings.ThemeLocale, "zh-CN"; got != want {
		t.Fatalf("theme locale = %q, want %q", got, want)
	}
	if got, want := strings.Join(settings.PluginOrder, ","), "custom-copy,extra-copy"; got != want {
		t.Fatalf("plugin order = %q, want %q", got, want)
	}
}

func TestLoadSettingsIgnoresDisabledLegacyTextPack(t *testing.T) {
	root := t.TempDir()
	body := `{
  "text_pack": {
    "enabled": false,
    "pack_id": "custom-copy"
  },
  "theme_locale": "zh_cn",
  "plugin_order": ["extra-copy"]
}`
	if err := os.WriteFile(filepath.Join(root, "settings.json"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}

	settings, err := LoadSettings(root)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := settings.ThemeLocale, "zh-CN"; got != want {
		t.Fatalf("theme locale = %q, want %q", got, want)
	}
	if got, want := strings.Join(settings.PluginOrder, ","), "extra-copy"; got != want {
		t.Fatalf("plugin order = %q, want %q", got, want)
	}
}

func TestLoadSettingsDefaultsMediaProcessing(t *testing.T) {
	settings, err := LoadSettings(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := settings.TimeZone, DefaultTimeZone; got != want {
		t.Fatalf("time zone = %q, want %q", got, want)
	}
	if settings.MediaProcessing.AutoWebP {
		t.Fatal("auto webp should be disabled by default")
	}
	if got, want := settings.MediaProcessing.WebPQuality, 82; got != want {
		t.Fatalf("webp quality = %d, want %d", got, want)
	}
	if settings.MediaProcessing.KeepOriginal {
		t.Fatal("keep original should be disabled by default")
	}
}

func TestTimeZoneOptionsIncludesCommonZones(t *testing.T) {
	options := TimeZoneOptions("Asia/Tokyo")
	for _, expected := range []string{"UTC", "Asia/Hong_Kong", "Asia/Tokyo", "America/New_York", "Europe/London"} {
		if !containsString(options, expected) {
			t.Fatalf("time zone options did not include %q", expected)
		}
	}
	for i := 1; i < len(options); i++ {
		if options[i-1] > options[i] {
			t.Fatalf("time zone options are not sorted near %q and %q", options[i-1], options[i])
		}
	}
}

func TestLoadSettingsNormalizesMediaProcessingQuality(t *testing.T) {
	root := t.TempDir()
	body := `{
  "media_processing": {
    "auto_webp": true,
    "webp_quality": 140,
    "keep_original": true
  }
}`
	if err := os.WriteFile(filepath.Join(root, "settings.json"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}

	settings, err := LoadSettings(root)
	if err != nil {
		t.Fatal(err)
	}
	if !settings.MediaProcessing.AutoWebP {
		t.Fatal("auto webp should be preserved")
	}
	if got, want := settings.MediaProcessing.WebPQuality, 100; got != want {
		t.Fatalf("webp quality = %d, want %d", got, want)
	}
	if !settings.MediaProcessing.KeepOriginal {
		t.Fatal("keep original should be preserved")
	}
}
