package media

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const tinyWebPBase64 = "UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA"

func tinyWebP(t *testing.T) []byte {
	t.Helper()
	body, err := base64.StdEncoding.DecodeString(tinyWebPBase64)
	if err != nil {
		t.Fatalf("decode tiny webp: %v", err)
	}
	return body
}

func TestSaveUploadReadsWebPDimensions(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	item, err := store.SaveUpload(bytes.NewReader(tinyWebP(t)), "tiny.webp")
	if err != nil {
		t.Fatalf("save webp: %v", err)
	}

	if item.MIMEType != "image/webp" {
		t.Fatalf("mime type = %q, want image/webp", item.MIMEType)
	}
	if item.Width != 1 || item.Height != 1 {
		t.Fatalf("dimensions = %dx%d, want 1x1", item.Width, item.Height)
	}
}

func TestOpenBackfillsMissingWebPDimensions(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "public", "2026", "05", "tiny.webp")
	if err := os.MkdirAll(filepath.Dir(mediaPath), 0755); err != nil {
		t.Fatalf("create media dir: %v", err)
	}
	if err := os.WriteFile(mediaPath, tinyWebP(t), 0644); err != nil {
		t.Fatalf("write webp: %v", err)
	}

	indexPath := filepath.Join(root, "index.json")
	index := `[
  {
    "id": "tiny",
    "path": "/media/2026/05/tiny.webp",
    "original_name": "tiny.webp",
    "alt": "tiny",
    "caption": "",
    "mime_type": "image/webp",
    "width": 0,
    "height": 0,
    "created_at": "2026-05-05T00:00:00Z"
  }
]`
	if err := os.WriteFile(indexPath, []byte(index), 0644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	store, err := Open(root)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	items := store.Items()
	if len(items) != 1 {
		t.Fatalf("items len = %d, want 1", len(items))
	}
	if items[0].Width != 1 || items[0].Height != 1 {
		t.Fatalf("dimensions = %dx%d, want 1x1", items[0].Width, items[0].Height)
	}

	body, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if !strings.Contains(string(body), `"width": 1`) || !strings.Contains(string(body), `"height": 1`) {
		t.Fatalf("index was not backfilled:\n%s", body)
	}
}
