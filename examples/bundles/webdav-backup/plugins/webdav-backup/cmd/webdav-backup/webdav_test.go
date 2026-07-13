package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestWebDAVUploadAndPrune(t *testing.T) {
	var (
		mu      sync.Mutex
		methods []string
		paths   []string
		putBody string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "alice" || password != "app-password" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		mu.Lock()
		methods = append(methods, r.Method)
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		switch r.Method {
		case "MKCOL":
			w.WriteHeader(http.StatusCreated)
		case "PUT":
			body, _ := io.ReadAll(r.Body)
			putBody = string(body)
			w.WriteHeader(http.StatusCreated)
		case "PROPFIND":
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = fmt.Fprint(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:">
<d:response><d:href>/dav/Postizer/backups/</d:href></d:response>
<d:response><d:href>/dav/Postizer/backups/postizer-export-20260713-120000-utc.zip</d:href></d:response>
<d:response><d:href>/dav/Postizer/backups/postizer-export-20260712-120000-utc.zip</d:href></d:response>
<d:response><d:href>/dav/Postizer/backups/postizer-export-20260711-120000-utc.zip</d:href></d:response>
</d:multistatus>`)
		case "DELETE":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected method", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	client, err := newWebDAVClient(config{
		ServerURL:  server.URL + "/dav",
		RemotePath: "Postizer/backups",
		Username:   "alice",
		Password:   "app-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := client.ensureCollection(ctx); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "backup.zip")
	if err := os.WriteFile(archive, []byte("zip-body"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := client.upload(ctx, archive, "postizer-export-20260713-120000-utc.zip"); err != nil {
		t.Fatal(err)
	}
	if err := client.prune(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if putBody != "zip-body" {
		t.Fatalf("uploaded body = %q", putBody)
	}

	mu.Lock()
	defer mu.Unlock()
	wantMethods := []string{"MKCOL", "MKCOL", "PUT", "PROPFIND", "DELETE"}
	if !reflect.DeepEqual(methods, wantMethods) {
		t.Fatalf("methods = %#v, want %#v", methods, wantMethods)
	}
	if got := paths[len(paths)-1]; !strings.HasSuffix(got, "/postizer-export-20260711-120000-utc.zip") {
		t.Fatalf("deleted path = %q", got)
	}
}

func TestPruneDoesNothingWhenRetentionExceedsBackupCount(t *testing.T) {
	deleted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			deleted = true
		}
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = fmt.Fprint(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:"><d:response><d:href>/backups/postizer-export-20260713-120000-utc.zip</d:href></d:response></d:multistatus>`)
	}))
	defer server.Close()
	client, err := newWebDAVClient(config{ServerURL: server.URL, RemotePath: "backups"})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.prune(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
	if deleted {
		t.Fatal("prune should not delete a backup")
	}
}
