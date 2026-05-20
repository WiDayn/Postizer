package site

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

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
		`draft: false`,
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
	if store.AllPagesBySlug["about-lab"] == nil {
		t.Fatal("page was not available to admin index")
	}
	if got, want := FormatInputDateTime(page.Date), "2026-05-04T09:30"; got != want {
		t.Fatalf("loaded page date = %q, want %q", got, want)
	}
	if got := page.Source; strings.HasPrefix(got, "\n") {
		t.Fatalf("loaded page source had a leading blank line: %q", got)
	}
}

func TestSavePageAndLoadDraft(t *testing.T) {
	root := t.TempDir()
	_, err := SavePage(root, PageDraft{
		Title:   "Draft Page",
		Date:    "2026-05-04T10:30",
		Summary: "Hidden until published.",
		Draft:   true,
		TOC:     true,
		Body:    "## Draft\n\nPage content.",
	})
	if err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(filepath.Join(root, "pages", "draft-page.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `draft: true`) {
		t.Fatalf("saved page did not preserve draft state:\n%s", body)
	}

	store, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if store.AllPagesBySlug["draft-page"] == nil {
		t.Fatal("draft page was not available to admin index")
	}
	if store.PagesBySlug["draft-page"] != nil {
		t.Fatal("draft page leaked into public index")
	}
	if got := store.AllPagesBySlug["draft-page"].Source; strings.HasPrefix(got, "\n") {
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
	if store.AllPagesBySlug["remove-page"] != nil {
		t.Fatal("deleted page remained in admin index")
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

func TestLoadPostUsesMoreTagAsSummaryWhenSummaryEmpty(t *testing.T) {
	root := t.TempDir()
	postDir := filepath.Join(root, "posts")
	if err := os.MkdirAll(postDir, 0755); err != nil {
		t.Fatal(err)
	}
	body := `---
title: "More Summary"
slug: "more-summary"
date: "2026-05-04T08:30"
summary: ""
draft: false
toc: true
---

Lead **paragraph** with [link](https://example.com).

<!--more-->

Rest of the post.
`
	if err := os.WriteFile(filepath.Join(postDir, "more-summary.md"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}

	store, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	post := store.PostsBySlug["more-summary"]
	if post == nil {
		t.Fatal("post not loaded")
	}
	if got, want := post.Summary, "Lead paragraph with link."; got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
	if strings.Contains(string(post.HTML), "more") {
		t.Fatalf("MORE tag should not be rendered into post HTML:\n%s", post.HTML)
	}
	if !strings.Contains(post.Source, "<!--more-->") {
		t.Fatalf("editor source should preserve MORE tag: %q", post.Source)
	}
}

func TestLoadPostExplicitSummaryOverridesMoreTag(t *testing.T) {
	root := t.TempDir()
	postDir := filepath.Join(root, "posts")
	if err := os.MkdirAll(postDir, 0755); err != nil {
		t.Fatal(err)
	}
	body := `---
title: "Explicit Summary"
slug: "explicit-summary"
date: "2026-05-04T08:30"
summary: "Manual summary."
draft: false
toc: true
---

Lead from MORE.

<!--more-->

Rest.
`
	if err := os.WriteFile(filepath.Join(postDir, "explicit-summary.md"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}

	store, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	post := store.PostsBySlug["explicit-summary"]
	if post == nil {
		t.Fatal("post not loaded")
	}
	if got, want := post.Summary, "Manual summary."; got != want {
		t.Fatalf("summary = %q, want %q", got, want)
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
	if !strings.Contains(renderedText(rendered), `\eqref{eq:energy}`) {
		t.Fatalf("equation reference inside code was replaced:\n%s", rendered)
	}
}

func TestRenderMarkdownHighlightsFencedCode(t *testing.T) {
	html, err := RenderMarkdown("```go\npackage main\n\nfunc main() {\n\tfmt.Println(\"ok\")\n}\n```")
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(html)
	for _, expected := range []string{`<pre tabindex="0"`, `background-color:#272822`, `<span style=`} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("highlighted code block did not contain %q:\n%s", expected, rendered)
		}
	}
	if strings.Contains(rendered, `<pre><code>package main`) {
		t.Fatalf("code block used the plain code renderer:\n%s", rendered)
	}
}

func TestRenderMarkdownGuessesLanguageForPlainFence(t *testing.T) {
	html, err := RenderMarkdown("```\npackage main\n\nfunc main() {\n\tfmt.Println(\"ok\")\n}\n```")
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(html)
	if !strings.Contains(rendered, `<pre tabindex="0"`) || !strings.Contains(rendered, `<span style=`) {
		t.Fatalf("plain fenced code block was not highlighted:\n%s", rendered)
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
	if strings.Contains(rendered, `article-figure`) || !strings.Contains(renderedText(rendered), `\figref{fig:code}`) {
		t.Fatalf("figure syntax inside code was modified:\n%s", rendered)
	}
}

var renderedHTMLTagPattern = regexp.MustCompile(`<[^>]+>`)

func renderedText(value string) string {
	return renderedHTMLTagPattern.ReplaceAllString(value, "")
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

func TestLoadSettingsDefaultsSiteTitle(t *testing.T) {
	settings, err := LoadSettings(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := settings.SiteTitle.Main, DefaultSiteTitle; got != want {
		t.Fatalf("site title main = %q, want %q", got, want)
	}
	if settings.SiteTitle.Subtitle != "" {
		t.Fatalf("site title subtitle = %q, want empty", settings.SiteTitle.Subtitle)
	}
}

func TestLoadSettingsDefaultsAutoUpdateDisabled(t *testing.T) {
	settings, err := LoadSettings(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if settings.AutoUpdate.Enabled {
		t.Fatal("auto update should be disabled by default")
	}
}

func TestLoadSettingsDefaultsCommentsDisabled(t *testing.T) {
	settings, err := LoadSettings(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if settings.Comments.Enabled {
		t.Fatal("comments should be disabled by default")
	}
}

func TestSaveSettingsNormalizesSiteTitle(t *testing.T) {
	root := t.TempDir()
	settings := defaultSettings()
	settings.SiteTitle = SiteTitle{
		Main:     "  Field Notes  ",
		Subtitle: "  Daily log  ",
	}
	if err := SaveSettings(root, settings); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSettings(root)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := loaded.SiteTitle.Main, "Field Notes"; got != want {
		t.Fatalf("site title main = %q, want %q", got, want)
	}
	if got, want := loaded.SiteTitle.Subtitle, "Daily log"; got != want {
		t.Fatalf("site title subtitle = %q, want %q", got, want)
	}

	settings.SiteTitle = SiteTitle{Main: " ", Subtitle: "  Still here  "}
	if err := SaveSettings(root, settings); err != nil {
		t.Fatal(err)
	}
	loaded, err = LoadSettings(root)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := loaded.SiteTitle.Main, DefaultSiteTitle; got != want {
		t.Fatalf("site title main = %q, want %q", got, want)
	}
	if got, want := loaded.SiteTitle.Subtitle, "Still here"; got != want {
		t.Fatalf("site title subtitle = %q, want %q", got, want)
	}
}

func TestAddCommentAndReply(t *testing.T) {
	root := t.TempDir()
	created := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	comment, err := AddComment(root, CommentInput{
		PostSlug: "hello-world",
		Nickname: "Reader",
		Email:    "reader@example.com",
		Body:     "First line\n\nSecond line",
	}, created)
	if err != nil {
		t.Fatal(err)
	}
	if comment.ID == "" {
		t.Fatal("comment id should be generated")
	}

	comments, err := CommentsForPost(root, "hello-world")
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 1 {
		t.Fatalf("comments = %d, want 1", len(comments))
	}
	if got, want := comments[0].Nickname, "Reader"; got != want {
		t.Fatalf("nickname = %q, want %q", got, want)
	}

	replied, err := ReplyToComment(root, comment.ID, "Thanks for reading.", created.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := replied.Reply.Body, "Thanks for reading."; got != want {
		t.Fatalf("reply = %q, want %q", got, want)
	}
}

func TestLoadAppliesCustomPermalinkURLs(t *testing.T) {
	root := t.TempDir()
	post, err := SavePost(root, PostDraft{
		Title: "Sample Post",
		Date:  "2026-05-20T10:30",
		Tags:  []string{"Go News"},
		Body:  "Body.",
	})
	if err != nil {
		t.Fatal(err)
	}
	page, err := SavePage(root, PageDraft{
		Title: "About",
		Body:  "About.",
	})
	if err != nil {
		t.Fatal(err)
	}
	settings := defaultSettings()
	settings.Permalinks = PermalinkSettings{
		Post: "/notes/%year%/%monthnum%/%postname%/",
		Page: "/docs/%pagename%",
		Tag:  "/topics/%tag%",
	}
	if err := SaveSettings(root, settings); err != nil {
		t.Fatal(err)
	}
	store, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}

	loadedPost := store.PostsBySlug[post.Slug]
	if got, want := loadedPost.URL, "/notes/2026/05/sample-post/"; got != want {
		t.Fatalf("post url = %q, want %q", got, want)
	}
	if store.PostByPermalink("/notes/2026/05/sample-post") != loadedPost {
		t.Fatal("post permalink should match with or without trailing slash")
	}
	loadedPage := store.PagesBySlug[page.Slug]
	if got, want := loadedPage.URL, "/docs/about"; got != want {
		t.Fatalf("page url = %q, want %q", got, want)
	}
	if store.PageByPermalink("/docs/about/") != loadedPage {
		t.Fatal("page permalink should match with or without trailing slash")
	}
	tag := store.TagsBySlug["go-news"]
	if tag == nil {
		t.Fatal("tag should exist")
	}
	if got, want := tag.URL, "/topics/go-news"; got != want {
		t.Fatalf("tag url = %q, want %q", got, want)
	}
	if store.TagByPermalink("/topics/go-news") != tag {
		t.Fatal("tag permalink should resolve")
	}
}

func TestValidatePermalinksRejectsUnknownTokens(t *testing.T) {
	err := ValidatePermalinks(PermalinkSettings{
		Post: "/notes/%author%/%postname%",
		Page: DefaultPagePermalink,
		Tag:  DefaultTagPermalink,
	})
	if err == nil {
		t.Fatal("expected invalid permalink token error")
	}
	if !strings.Contains(err.Error(), "%author%") {
		t.Fatalf("error = %q, want unknown token", err.Error())
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

func TestLoadSettingsPreservesThemeCustomPrimitiveSettings(t *testing.T) {
	root := t.TempDir()
	body := []byte(`{
  "theme_settings": {
    "custom": {
      "pure-white": {
        "hero_title": "Hello",
        "items_per_page": 7,
        "opacity": 0.65,
        "unsupported": true
      }
    }
  }
}`)
	if err := os.WriteFile(filepath.Join(root, "settings.json"), body, 0644); err != nil {
		t.Fatal(err)
	}

	settings, err := LoadSettings(root)
	if err != nil {
		t.Fatal(err)
	}
	values := settings.ThemeSettings.Custom["pure-white"]
	if got, want := values["hero_title"].StringValue(), "Hello"; got != want {
		t.Fatalf("hero_title = %q, want %q", got, want)
	}
	if got, want := values["items_per_page"].IntegerValue(), int64(7); got != want {
		t.Fatalf("items_per_page = %d, want %d", got, want)
	}
	if got, want := values["opacity"].FloatValue(), 0.65; got != want {
		t.Fatalf("opacity = %v, want %v", got, want)
	}
	if _, ok := values["unsupported"]; ok {
		t.Fatal("unsupported custom theme setting should be discarded")
	}

	if err := SaveSettings(root, settings); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	savedText := string(saved)
	for _, snippet := range []string{
		`"hero_title": "Hello"`,
		`"items_per_page": 7`,
		`"opacity": 0.65`,
	} {
		if !strings.Contains(savedText, snippet) {
			t.Fatalf("saved settings should contain %s:\n%s", snippet, savedText)
		}
	}
}

func TestLoadSettingsMigratesLegacyPureWhiteHeroTextSettings(t *testing.T) {
	root := t.TempDir()
	settings := defaultSettings()
	settings.ThemeSettings = ThemeSettings{
		MenuLocations: map[string]string{
			"navbar":                                "",
			"pure-white-hero-title-48656c6c6f":      "",
			"pure-white-hero-subtitle-e4bda0e5a5bd": "",
		},
	}
	if err := SaveSettings(root, settings); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadSettings(root)
	if err != nil {
		t.Fatal(err)
	}
	values := loaded.ThemeSettings.Custom["pure-white"]
	if got, want := values["hero_title"].StringValue(), "Hello"; got != want {
		t.Fatalf("migrated hero_title = %q, want %q", got, want)
	}
	if got, want := values["hero_subtitle"].StringValue(), "你好"; got != want {
		t.Fatalf("migrated hero_subtitle = %q, want %q", got, want)
	}
	if _, ok := loaded.ThemeSettings.MenuLocations["pure-white-hero-title-48656c6c6f"]; ok {
		t.Fatal("legacy Pure White title key should be removed from menu_locations")
	}
	if _, ok := loaded.ThemeSettings.MenuLocations["navbar"]; !ok {
		t.Fatal("normal menu location should be preserved")
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
