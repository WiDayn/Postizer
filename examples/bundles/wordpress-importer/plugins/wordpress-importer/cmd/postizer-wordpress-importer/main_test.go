package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"postizer/pkg/pluginrpc"
)

func TestInspectWXRFixture(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "linxin039sblog.WordPress.2026-05-06.xml"))
	if os.IsNotExist(err) {
		t.Skip("fixture XML is not present")
	}
	if err != nil {
		t.Fatal(err)
	}
	export, err := parseWXR(body)
	if err != nil {
		t.Fatal(err)
	}
	result := inspect(export)

	counts := map[string]string{}
	for _, section := range result.Sections {
		if section.Title != "Content types" {
			continue
		}
		for _, row := range section.Rows {
			counts[row.Label] = row.Value
		}
	}
	if got, want := counts["post"], "38"; got != want {
		t.Fatalf("post count = %q, want %q", got, want)
	}
	if got, want := counts["page"], "5"; got != want {
		t.Fatalf("page count = %q, want %q", got, want)
	}
	if got, want := counts["attachment"], "63"; got != want {
		t.Fatalf("attachment count = %q, want %q", got, want)
	}
	if len(result.NextActions) != 1 || result.NextActions[0].ID != "import_wxr" {
		t.Fatalf("inspect should offer import action: %#v", result.NextActions)
	}
}

func TestBuildImportBatchFixture(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "linxin039sblog.WordPress.2026-05-06.xml"))
	if os.IsNotExist(err) {
		t.Skip("fixture XML is not present")
	}
	if err != nil {
		t.Fatal(err)
	}
	export, err := parseWXR(body)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := buildImportBatch(export)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := len(batch.Posts), 36; got != want {
		t.Fatalf("planned posts = %d, want %d", got, want)
	}
	if got, want := len(batch.Pages), 5; got != want {
		t.Fatalf("planned pages = %d, want %d", got, want)
	}
	if got, want := len(batch.Media), 63; got < want {
		t.Fatalf("planned media = %d, want at least %d", got, want)
	}
	if len(batch.Skipped) == 0 {
		t.Fatal("trash/unknown content should be reported as skipped")
	}
	for _, post := range batch.Posts {
		if post.Title == "" || post.Slug == "" {
			t.Fatalf("planned post should have title and slug: %#v", post)
		}
		if post.Summary != "" {
			t.Fatalf("WordPress import should leave summary empty, got %q for %q", post.Summary, post.Title)
		}
		if containsWordPressHTML(post.Body) {
			t.Fatalf("planned post %q still contains WordPress HTML:\n%s", post.Title, firstRunes(post.Body, 400))
		}
	}
	article := findDraft(batch.Posts, "May 2025 MIA")
	if article == nil {
		t.Fatal("fixture should include the May 2025 MIA article")
	}
	for _, want := range []string{`# 鏂规硶閮ㄥ垎`, `\begin{figure}`, `$P_i$`} {
		if !strings.Contains(article.Body, want) {
			t.Fatalf("converted article missing %q:\n%s", want, firstRunes(article.Body, 800))
		}
	}
}

func TestMarkdownConvertsWordPressBlocks(t *testing.T) {
	builder := &importBuilder{
		urlToMedia: map[string]string{},
		mediaByID:  map[string]bool{},
	}
	raw := `<p>Link: <a href="https://example.com/paper">https://example.com/paper</a></p>
<figure class="wp-block-image size-large"><img src="https://linxin.blog/wp-content/uploads/2026/04/example-1024x566.png" alt="" class="wp-image-731"/></figure>
<p><strong>Key question</strong></p>
<!--more-->
<h1 class="wp-block-heading">Methods</h1>
<blockquote class="wp-block-quote"><p>quoted text</p></blockquote>
<ul class="wp-block-list"><li>first item</li><li>second item</li></ul>`

	markdown := builder.markdown(raw)
	if containsWordPressHTML(markdown) {
		t.Fatalf("converted markdown still contains WordPress HTML:\n%s", markdown)
	}
	for _, want := range []string{
		`Link: [https://example.com/paper](https://example.com/paper)`,
		`\begin{figure}`,
		`![Image](postizer://media/`,
		`**Key question**`,
		`<!--more-->`,
		`# Methods`,
		`> quoted text`,
		`- first item`,
	} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("converted markdown missing %q:\n%s", want, markdown)
		}
	}
}

func findDraft(drafts []pluginrpc.ContentDraft, titlePrefix string) *pluginrpc.ContentDraft {
	for index := range drafts {
		if strings.HasPrefix(drafts[index].Title, titlePrefix) {
			return &drafts[index]
		}
	}
	return nil
}

var wordPressHTMLPattern = regexp.MustCompile(`(?i)</?(?:p|figure|h[1-6]|ul|ol|li|blockquote|div|img|a)(?:\s|>|/)`)

func containsWordPressHTML(value string) bool {
	return strings.Contains(value, "wp-block-") || wordPressHTMLPattern.MatchString(stripFencedCode(value))
}

func stripFencedCode(value string) string {
	re := regexp.MustCompile("(?s)```.*?```")
	return re.ReplaceAllString(value, "")
}

func firstRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
