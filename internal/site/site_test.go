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
	if got := store.AllPostsBySlug["editor-smoke"].Source; strings.HasPrefix(got, "\n") {
		t.Fatalf("loaded editor source had a leading blank line: %q", got)
	}
}

func TestSavePostUsesTitleForURL(t *testing.T) {
	root := t.TempDir()
	saved, err := SavePost(root, PostDraft{
		Title: "Title Driven URL",
		Slug:  "custom-url",
		Body:  "Body.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := saved.Slug, "title-driven-url"; got != want {
		t.Fatalf("slug = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(root, "posts", "title-driven-url.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "posts", "custom-url.md")); !os.IsNotExist(err) {
		t.Fatalf("custom URL file should not exist, stat err = %v", err)
	}
}

func TestSavePostUsesUnicodeTitleForURL(t *testing.T) {
	root := t.TempDir()
	saved, err := SavePost(root, PostDraft{
		Title: "测试-123",
		Body:  "Body.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := saved.Slug, "测试-123"; got != want {
		t.Fatalf("slug = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(root, "posts", "测试-123.md")); err != nil {
		t.Fatal(err)
	}
}

func TestSavePostRenamesTitleURL(t *testing.T) {
	root := t.TempDir()
	if _, err := SavePost(root, PostDraft{
		Title: "Old Title",
		Body:  "Old body.",
	}); err != nil {
		t.Fatal(err)
	}
	saved, err := SavePost(root, PostDraft{
		Title:        "New Title",
		OriginalSlug: "old-title",
		Body:         "New body.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := saved.Slug, "new-title"; got != want {
		t.Fatalf("slug = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(root, "posts", "new-title.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "posts", "old-title.md")); !os.IsNotExist(err) {
		t.Fatalf("old title URL file should be removed, stat err = %v", err)
	}
}

func TestSavePostAddsNumericSuffixForTitleURLCollision(t *testing.T) {
	root := t.TempDir()
	if _, err := SavePost(root, PostDraft{
		Title: "Existing Title",
		Body:  "Existing body.",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := SavePost(root, PostDraft{
		Title: "Old Title",
		Body:  "Old body.",
	}); err != nil {
		t.Fatal(err)
	}
	saved, err := SavePost(root, PostDraft{
		Title:        "Existing Title",
		OriginalSlug: "old-title",
		Body:         "Should not overwrite.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := saved.Slug, "existing-title-2"; got != want {
		t.Fatalf("slug = %q, want %q", got, want)
	}
	body, readErr := os.ReadFile(filepath.Join(root, "posts", "existing-title.md"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if strings.Contains(string(body), "Should not overwrite.") {
		t.Fatalf("existing post was overwritten:\n%s", body)
	}
	if _, err := os.Stat(filepath.Join(root, "posts", "existing-title-2.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "posts", "old-title.md")); !os.IsNotExist(err) {
		t.Fatalf("old title URL file should be removed, stat err = %v", err)
	}
}

func TestSavePostSkipsUsedNumericSuffixes(t *testing.T) {
	root := t.TempDir()
	for _, title := range []string{"Same Title", "Same Title", "Same Title"} {
		if _, err := SavePost(root, PostDraft{Title: title, Body: "Body."}); err != nil {
			t.Fatal(err)
		}
	}
	for _, slug := range []string{"same-title", "same-title-2", "same-title-3"} {
		if _, err := os.Stat(filepath.Join(root, "posts", slug+".md")); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDeletePostRemovesMarkdownFile(t *testing.T) {
	root := t.TempDir()
	if _, err := SavePost(root, PostDraft{
		Title: "Delete Me",
		Body:  "Body.",
	}); err != nil {
		t.Fatal(err)
	}
	if err := DeletePost(root, "delete-me"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "posts", "delete-me.md")); !os.IsNotExist(err) {
		t.Fatalf("deleted post should be gone, stat err = %v", err)
	}
	store, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if store.AllPostsBySlug["delete-me"] != nil {
		t.Fatal("deleted post remained in loaded store")
	}
}

func TestDeletePostRejectsInvalidSlug(t *testing.T) {
	root := t.TempDir()
	if err := DeletePost(root, "../outside"); err == nil {
		t.Fatal("expected invalid slug error")
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

func TestSavePageAndLoad(t *testing.T) {
	root := t.TempDir()
	saved, err := SavePage(root, PageDraft{
		Title:         "About Lab",
		Slug:          "about-lab",
		Date:          "2026-05-04 09:30",
		Updated:       "2026-05-05 18:45",
		UpdatedManual: true,
		Summary:       "A page edited from the admin.",
		TOC:           true,
		Body:          "## About\n\nPage content.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := saved.Updated, "2026-05-05T18:45"; got != want {
		t.Fatalf("updated = %q, want %q", got, want)
	}
	if got, want := saved.Date, "2026-05-04T09:30"; got != want {
		t.Fatalf("date = %q, want %q", got, want)
	}

	body, err := os.ReadFile(filepath.Join(root, "pages", "about-lab.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`title: "About Lab"`,
		`slug: "about-lab"`,
		`date: "2026-05-04T09:30"`,
		`updated: "2026-05-05T18:45"`,
		`toc: true`,
		"## About",
	} {
		if !strings.Contains(string(body), expected) {
			t.Fatalf("saved page did not contain %q:\n%s", expected, body)
		}
	}

	store, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	page := store.PagesBySlug["about-lab"]
	if page == nil {
		t.Fatal("page was not loaded")
	}
	if got, want := FormatInputDateTime(page.Date), "2026-05-04T09:30"; got != want {
		t.Fatalf("loaded page date = %q, want %q", got, want)
	}
	if got := page.Source; strings.HasPrefix(got, "\n") {
		t.Fatalf("loaded page source had a leading blank line: %q", got)
	}
}

func TestSavePageUsesTitleForURL(t *testing.T) {
	root := t.TempDir()
	saved, err := SavePage(root, PageDraft{
		Title: "Page Title URL",
		Slug:  "custom-page",
		Body:  "Body.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := saved.Slug, "page-title-url"; got != want {
		t.Fatalf("slug = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(root, "pages", "page-title-url.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "pages", "custom-page.md")); !os.IsNotExist(err) {
		t.Fatalf("custom URL file should not exist, stat err = %v", err)
	}
}

func TestSavePageAddsNumericSuffixForTitleURLCollision(t *testing.T) {
	root := t.TempDir()
	if _, err := SavePage(root, PageDraft{Title: "About", Body: "Body."}); err != nil {
		t.Fatal(err)
	}
	saved, err := SavePage(root, PageDraft{Title: "About", Body: "Body."})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := saved.Slug, "about-2"; got != want {
		t.Fatalf("slug = %q, want %q", got, want)
	}
}

func TestDeletePageRemovesMarkdownFile(t *testing.T) {
	root := t.TempDir()
	if _, err := SavePage(root, PageDraft{
		Title: "Remove Page",
		Body:  "Body.",
	}); err != nil {
		t.Fatal(err)
	}
	if err := DeletePage(root, "remove-page"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "pages", "remove-page.md")); !os.IsNotExist(err) {
		t.Fatalf("deleted page should be gone, stat err = %v", err)
	}
	store, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if store.PagesBySlug["remove-page"] != nil {
		t.Fatal("deleted page remained in loaded store")
	}
}

func TestLoadPageFallsBackToUpdatedForLegacyDate(t *testing.T) {
	root := t.TempDir()
	pageDir := filepath.Join(root, "pages")
	if err := os.MkdirAll(pageDir, 0755); err != nil {
		t.Fatal(err)
	}
	body := `---
title: "Legacy Page"
slug: "legacy-page"
updated: "2026-05-05T12:30"
summary: ""
toc: false
---

Body.
`
	if err := os.WriteFile(filepath.Join(pageDir, "legacy-page.md"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	store, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	page := store.PagesBySlug["legacy-page"]
	if page == nil {
		t.Fatal("page was not loaded")
	}
	if got, want := FormatInputDateTime(page.Date), "2026-05-05T12:30"; got != want {
		t.Fatalf("legacy page date = %q, want %q", got, want)
	}
}

func TestSplitFrontMatterRemovesSeparatorBlankLine(t *testing.T) {
	_, markdown := splitFrontMatter([]byte("---\ntitle: \"Sample\"\n---\n\n## Heading\n\nBody."))
	if got, want := string(markdown), "## Heading\n\nBody."; got != want {
		t.Fatalf("markdown = %q, want %q", got, want)
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

func TestThemeMenuLinksResolveSupportedItems(t *testing.T) {
	root := t.TempDir()
	page, err := SavePage(root, PageDraft{
		Title: "About Us",
		Body:  "Page body.",
	})
	if err != nil {
		t.Fatal(err)
	}
	post, err := SavePost(root, PostDraft{
		Title: "Menu Post",
		Tags:  []string{"Go News"},
		Body:  "Post body.",
	})
	if err != nil {
		t.Fatal(err)
	}

	settings := defaultSettings()
	settings.ThemeSettings = ThemeSettings{
		Menus: []Menu{
			{
				ID:   "main",
				Name: "Main",
				Items: []MenuItem{
					{Type: "page", Target: page.Slug},
					{Type: "post", Target: post.Slug, Label: "Featured"},
					{Type: "tag", Target: "go-news"},
					{Type: "custom", Label: "External", URL: "https://example.com"},
					{Type: "custom", Label: "Bad", URL: "javascript:alert(1)"},
				},
			},
		},
		MenuLocations: map[string]string{"navbar": "main"},
	}
	if err := SaveSettings(root, settings); err != nil {
		t.Fatal(err)
	}

	store, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if !store.MenuLocationAssigned("navbar") {
		t.Fatal("navbar menu location should be marked assigned")
	}
	links := store.MenuLinks("navbar")
	want := []MenuLink{
		{Label: "About Us", URL: "/pages/" + page.Slug},
		{Label: "Featured", URL: "/posts/" + post.Slug},
		{Label: "Go News", URL: "/tags/go-news"},
		{Label: "External", URL: "https://example.com"},
	}
	if len(links) != len(want) {
		t.Fatalf("menu links = %#v, want %#v", links, want)
	}
	for index := range want {
		if links[index] != want[index] {
			t.Fatalf("menu link[%d] = %#v, want %#v", index, links[index], want[index])
		}
	}
}

func TestLoadSettingsPreservesExplicitEmptyMenuLocation(t *testing.T) {
	root := t.TempDir()
	settings := defaultSettings()
	settings.ThemeSettings = ThemeSettings{
		MenuLocations: map[string]string{"navbar": ""},
	}
	if err := SaveSettings(root, settings); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadSettings(root)
	if err != nil {
		t.Fatal(err)
	}
	value, ok := loaded.ThemeSettings.MenuLocations["navbar"]
	if !ok {
		t.Fatal("explicit empty menu location should be preserved")
	}
	if value != "" {
		t.Fatalf("explicit empty menu location = %q, want empty", value)
	}
}

func TestValidMenuURLAllowsPublicTargetsOnly(t *testing.T) {
	cases := []struct {
		value string
		want  bool
	}{
		{"/about", true},
		{"https://example.com", true},
		{"http://example.com", true},
		{"mailto:editor@example.com", true},
		{"//example.com", false},
		{"javascript:alert(1)", false},
		{"", false},
	}
	for _, test := range cases {
		if got := ValidMenuURL(test.value); got != test.want {
			t.Fatalf("ValidMenuURL(%q) = %v, want %v", test.value, got, test.want)
		}
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
