package media

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/gif"
	"image/png"
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

func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	img.Set(1, 0, color.NRGBA{B: 255, A: 255})
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return out.Bytes()
}

func testGIF(t *testing.T) []byte {
	t.Helper()
	img := image.NewPaletted(image.Rect(0, 0, 2, 1), color.Palette{
		color.Black,
		color.White,
	})
	img.SetColorIndex(0, 0, 0)
	img.SetColorIndex(1, 0, 1)
	var out bytes.Buffer
	if err := gif.Encode(&out, img, nil); err != nil {
		t.Fatalf("encode gif: %v", err)
	}
	return out.Bytes()
}

func requireStoreFile(t *testing.T, store *Store, publicPath string) string {
	t.Helper()
	path, ok := store.publicPathFilePath(publicPath)
	if !ok {
		t.Fatalf("invalid public path %q", publicPath)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file %s to exist: %v", path, err)
	}
	return path
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

func TestSaveUploadConvertsPNGToWebP(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	item, err := store.SaveUploadWithOptions(bytes.NewReader(testPNG(t)), "sample.png", ProcessingOptions{
		ConvertToWebP: true,
		WebPQuality:   70,
	})
	if err != nil {
		t.Fatalf("save png as webp: %v", err)
	}

	if !item.Optimized {
		t.Fatal("item should be marked optimized")
	}
	if item.MIMEType != "image/webp" {
		t.Fatalf("mime type = %q, want image/webp", item.MIMEType)
	}
	if !strings.HasSuffix(item.Path, ".webp") {
		t.Fatalf("path = %q, want .webp suffix", item.Path)
	}
	if item.OriginalPath != "" {
		t.Fatalf("original path = %q, want empty when keep original is disabled", item.OriginalPath)
	}
	if item.Width != 2 || item.Height != 1 {
		t.Fatalf("dimensions = %dx%d, want 2x1", item.Width, item.Height)
	}
	requireStoreFile(t, store, item.Path)
}

func TestSaveUploadKeepsOriginalWhenConverting(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	item, err := store.SaveUploadWithOptions(bytes.NewReader(testPNG(t)), "sample.png", ProcessingOptions{
		ConvertToWebP: true,
		WebPQuality:   70,
		KeepOriginal:  true,
	})
	if err != nil {
		t.Fatalf("save png as webp: %v", err)
	}

	if item.OriginalPath == "" {
		t.Fatal("original path should be set")
	}
	if item.OriginalMIME != "image/png" {
		t.Fatalf("original mime = %q, want image/png", item.OriginalMIME)
	}
	optimizedPath := requireStoreFile(t, store, item.Path)
	originalPath := requireStoreFile(t, store, item.OriginalPath)

	if err := store.Delete(item.ID); err != nil {
		t.Fatalf("delete item: %v", err)
	}
	if _, err := os.Stat(optimizedPath); !os.IsNotExist(err) {
		t.Fatalf("optimized file still exists or stat failed unexpectedly: %v", err)
	}
	if _, err := os.Stat(originalPath); !os.IsNotExist(err) {
		t.Fatalf("original file still exists or stat failed unexpectedly: %v", err)
	}
}

func TestSaveUploadLeavesWebPUnchangedWhenConversionEnabled(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	item, err := store.SaveUploadWithOptions(bytes.NewReader(tinyWebP(t)), "tiny.webp", ProcessingOptions{
		ConvertToWebP: true,
		WebPQuality:   60,
		KeepOriginal:  true,
	})
	if err != nil {
		t.Fatalf("save webp: %v", err)
	}

	if item.Optimized {
		t.Fatal("webp upload should not be marked optimized")
	}
	if item.OriginalPath != "" {
		t.Fatalf("webp upload should not keep an extra original, got %q", item.OriginalPath)
	}
	if !strings.HasSuffix(item.Path, ".webp") {
		t.Fatalf("path = %q, want .webp suffix", item.Path)
	}
	if item.MIMEType != "image/webp" {
		t.Fatalf("mime type = %q, want image/webp", item.MIMEType)
	}
}

func TestSaveUploadLeavesGIFUnchangedWhenConversionEnabled(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	item, err := store.SaveUploadWithOptions(bytes.NewReader(testGIF(t)), "animated.gif", ProcessingOptions{
		ConvertToWebP: true,
		WebPQuality:   60,
		KeepOriginal:  true,
	})
	if err != nil {
		t.Fatalf("save gif: %v", err)
	}

	if item.Optimized {
		t.Fatal("gif upload should not be marked optimized")
	}
	if item.OriginalPath != "" {
		t.Fatalf("gif upload should not keep an extra original, got %q", item.OriginalPath)
	}
	if !strings.HasSuffix(item.Path, ".gif") {
		t.Fatalf("path = %q, want .gif suffix", item.Path)
	}
	if item.MIMEType != "image/gif" {
		t.Fatalf("mime type = %q, want image/gif", item.MIMEType)
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
