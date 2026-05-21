package http

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"postizer/internal/media"
	"postizer/internal/site"
	"postizer/pkg/pluginrpc"
)

func TestCreateContentExportIncludesPostsPagesAndMedia(t *testing.T) {
	contentRoot := t.TempDir()
	mediaRoot := t.TempDir()

	if _, err := site.SavePost(contentRoot, site.PostDraft{
		Title: "Hello Export",
		Slug:  "hello-export",
		Date:  "2026-05-22T10:00",
		Body:  "Post body.",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := site.SavePage(contentRoot, site.PageDraft{
		Title: "About Export",
		Slug:  "about-export",
		Date:  "2026-05-22T10:00",
		Body:  "Page body.",
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contentRoot, ".session_secret"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}

	mediaStore, err := media.Open(mediaRoot)
	if err != nil {
		t.Fatal(err)
	}
	item, err := mediaStore.SaveUpload(strings.NewReader("media body"), "asset.txt")
	if err != nil {
		t.Fatal(err)
	}

	s := &Server{
		contentRoot:     contentRoot,
		media:           mediaStore,
		pluginDownloads: map[string]pluginDownload{},
	}
	response, err := s.CreateContentExport(context.Background(), &pluginrpc.CreateContentExportRequest{PluginID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if response.Posts != 1 || response.Pages != 1 || response.MediaItems != 1 {
		t.Fatalf("export counts = posts %d pages %d media %d", response.Posts, response.Pages, response.MediaItems)
	}
	if response.Bytes <= 0 {
		t.Fatal("export should report archive size")
	}
	if !strings.HasPrefix(response.DownloadURL, "/admin/api/plugin-downloads/") {
		t.Fatalf("download URL = %q", response.DownloadURL)
	}

	token := strings.TrimPrefix(response.DownloadURL, "/admin/api/plugin-downloads/")
	download, ok := s.pluginDownload(token)
	if !ok {
		t.Fatal("export should be registered as a plugin download")
	}
	defer os.Remove(download.path)

	entries := zipEntryNames(t, download.path)
	for _, want := range []string{
		"manifest.json",
		"content/posts/hello-export.md",
		"content/pages/about-export.md",
		"media/index.json",
		"media/public/" + strings.TrimPrefix(item.Path, "/media/"),
	} {
		if !entries[want] {
			t.Fatalf("export is missing %s; entries = %#v", want, entries)
		}
	}
	if entries["content/.session_secret"] {
		t.Fatal("export should not include unrelated content secrets")
	}
}

func zipEntryNames(t *testing.T, filename string) map[string]bool {
	t.Helper()
	reader, err := zip.OpenReader(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	entries := map[string]bool{}
	for _, file := range reader.File {
		entries[file.Name] = true
	}
	return entries
}
