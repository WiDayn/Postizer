package marketplace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadIndexValidatesAndNormalizesThemes(t *testing.T) {
	body := `{
  "schema": 1,
  "items": [
    {
      "id": "editorial-tools",
      "name": " Editorial Tools ",
      "summary": "Theme and plugin bundle",
      "repo": "https://github.com/example/editorial-tools",
      "preview": "/marketplace/previews/editorial-tools.svg",
      "tags": ["minimal", "minimal", "blog"],
      "themes": [{"id": "minimal-paper", "name": "Minimal Paper", "version": "1.0.0"}],
      "plugins": [{"id": "content-exporter", "name": "Content Exporter", "version": "1.0.0"}],
      "release": {
        "tag": "v1.2.3",
        "asset": "editorial-tools-v1.2.3.zip",
        "sha256": "` + strings.Repeat("a", 64) + `"
      },
      "min_postizer": "1.0.0"
    }
  ]
}`
	path := filepath.Join(t.TempDir(), "index.json")
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}

	index, err := LoadIndex(context.Background(), path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(index.Items), 1; got != want {
		t.Fatalf("items = %d, want %d", got, want)
	}
	item := index.Items[0]
	if got, want := item.Name, "Editorial Tools"; got != want {
		t.Fatalf("name = %q, want %q", got, want)
	}
	if got, want := item.Tags, []string{"theme", "plugin", "minimal", "blog"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("tags = %#v, want %#v", got, want)
	}
	if got, want := len(item.Themes), 1; got != want {
		t.Fatalf("themes = %d, want %d", got, want)
	}
	if got, want := len(item.Plugins), 1; got != want {
		t.Fatalf("plugins = %d, want %d", got, want)
	}
}

func TestLoadIndexRejectsDuplicateIDs(t *testing.T) {
	body := `{
  "schema": 1,
  "items": [
    {"id":"paper","name":"Paper","summary":"Theme","repo":"https://github.com/example/paper","themes":[{"id":"paper-theme","name":"Paper Theme"}],"release":{"tag":"v1.0.0","asset":"paper.zip","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
    {"id":"paper","name":"Paper","summary":"Theme","repo":"https://github.com/example/paper","themes":[{"id":"paper-theme","name":"Paper Theme"}],"release":{"tag":"v1.0.0","asset":"paper.zip","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}
  ]
}`
	path := filepath.Join(t.TempDir(), "index.json")
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadIndex(context.Background(), path, nil); err == nil {
		t.Fatal("LoadIndex should reject duplicate IDs")
	}
}

func TestReleaseAssetURLRequiresGitHubReleaseZip(t *testing.T) {
	item := PackItem{
		ID:   "paper",
		Repo: "https://github.com/example/paper",
		Release: Release{
			Tag:   "v1.2.3",
			Asset: "paper-v1.2.3.zip",
		},
	}
	got, err := ReleaseAssetURL(item)
	if err != nil {
		t.Fatal(err)
	}
	want := "https://github.com/example/paper/releases/download/v1.2.3/paper-v1.2.3.zip"
	if got != want {
		t.Fatalf("release asset URL = %q, want %q", got, want)
	}

	item.Release.Asset = "../paper.zip"
	if _, err := ReleaseAssetURL(item); err == nil {
		t.Fatal("ReleaseAssetURL should reject asset paths")
	}
}

func TestDownloadAndVerifyReleaseAsset(t *testing.T) {
	body := []byte("zip-body")
	sum := sha256.Sum256(body)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer server.Close()

	client := server.Client()
	client.Transport = rewriteGitHubTransport{
		base:   client.Transport,
		target: server.URL,
	}

	item := PackItem{
		ID:   "paper",
		Repo: "https://github.com/example/paper",
		Release: Release{
			Tag:    "v1.0.0",
			Asset:  "paper.zip",
			SHA256: hex.EncodeToString(sum[:]),
		},
	}
	downloaded, _, err := DownloadReleaseAsset(context.Background(), client, item)
	if err != nil {
		t.Fatal(err)
	}
	if string(downloaded) != string(body) {
		t.Fatalf("downloaded body = %q, want %q", downloaded, body)
	}
	if err := VerifyReleaseAsset(item, downloaded); err != nil {
		t.Fatal(err)
	}

	item.Release.SHA256 = strings.Repeat("b", 64)
	if err := VerifyReleaseAsset(item, downloaded); err == nil {
		t.Fatal("VerifyReleaseAsset should reject checksum mismatch")
	}
}

type rewriteGitHubTransport struct {
	base   http.RoundTripper
	target string
}

func (t rewriteGitHubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host == "github.com" {
		target := strings.TrimRight(t.target, "/") + req.URL.Path
		rewritten, err := http.NewRequestWithContext(req.Context(), req.Method, target, req.Body)
		if err != nil {
			return nil, err
		}
		rewritten.Header = req.Header.Clone()
		req = rewritten
	}
	return t.base.RoundTrip(req)
}
