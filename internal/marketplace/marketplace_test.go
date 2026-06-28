package marketplace

import (
	"context"
	"crypto/sha256"
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
        "asset": "editorial-tools-v1.2.3.zip"
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
    {"id":"paper","name":"Paper","summary":"Theme","repo":"https://github.com/example/paper","themes":[{"id":"paper-theme","name":"Paper Theme"}],"release":{"tag":"v1.0.0","asset":"paper.zip"}},
    {"id":"paper","name":"Paper","summary":"Theme","repo":"https://github.com/example/paper","themes":[{"id":"paper-theme","name":"Paper Theme"}],"release":{"tag":"v1.0.0","asset":"paper.zip"}}
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

	checksumURL, err := ReleaseChecksumURL(item)
	if err != nil {
		t.Fatal(err)
	}
	wantChecksumURL := "https://github.com/example/paper/releases/download/v1.2.3/SHA256SUMS"
	if checksumURL != wantChecksumURL {
		t.Fatalf("release checksum URL = %q, want %q", checksumURL, wantChecksumURL)
	}

	item.Release.Asset = "../paper.zip"
	if _, err := ReleaseAssetURL(item); err == nil {
		t.Fatal("ReleaseAssetURL should reject asset paths")
	}
}

func TestDownloadAndVerifyReleaseAsset(t *testing.T) {
	body := []byte("zip-body")
	sum := sha256.Sum256(body)
	checksums := []byte("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff  other.zip\n" +
		hexString(sum[:]) + "  paper.zip\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/paper.zip"):
			_, _ = w.Write(body)
		case strings.HasSuffix(r.URL.Path, "/SHA256SUMS"):
			_, _ = w.Write(checksums)
		default:
			http.NotFound(w, r)
		}
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
			Tag:   "v1.0.0",
			Asset: "paper.zip",
		},
	}
	downloaded, _, err := DownloadReleaseAsset(context.Background(), client, item)
	if err != nil {
		t.Fatal(err)
	}
	if string(downloaded) != string(body) {
		t.Fatalf("downloaded body = %q, want %q", downloaded, body)
	}
	downloadedChecksums, _, err := DownloadReleaseChecksums(context.Background(), client, item)
	if err != nil {
		t.Fatal(err)
	}
	if string(downloadedChecksums) != string(checksums) {
		t.Fatalf("downloaded checksums = %q, want %q", downloadedChecksums, checksums)
	}
	if err := VerifyReleaseAsset(item, downloaded, downloadedChecksums); err != nil {
		t.Fatal(err)
	}

	badChecksums := []byte(strings.Repeat("b", 64) + "  paper.zip\n")
	if err := VerifyReleaseAsset(item, downloaded, badChecksums); err == nil {
		t.Fatal("VerifyReleaseAsset should reject checksum mismatch")
	}
}

func TestVerifyReleaseAssetRequiresChecksumEntry(t *testing.T) {
	item := PackItem{
		ID:   "paper",
		Repo: "https://github.com/example/paper",
		Release: Release{
			Tag:   "v1.0.0",
			Asset: "paper.zip",
		},
	}
	checksums := []byte(strings.Repeat("a", 64) + "  other.zip\n")
	if err := VerifyReleaseAsset(item, []byte("zip-body"), checksums); err == nil {
		t.Fatal("VerifyReleaseAsset should reject missing checksum entries")
	}
}

func hexString(body []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, len(body)*2)
	for i, b := range body {
		out[i*2] = digits[b>>4]
		out[i*2+1] = digits[b&0x0f]
	}
	return string(out)
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
