package updatecheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestLatestRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("User-Agent"), "Postizer Update Checker"; got != want {
			t.Fatalf("user agent = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v0.1.8","html_url":"https://github.com/WiDayn/Postizer/releases/tag/v0.1.8","published_at":"2026-07-12T20:29:47Z"}`))
	}))
	defer server.Close()

	release, err := (Client{HTTPClient: server.Client(), APIBase: server.URL}).LatestRelease(context.Background(), "https://github.com/WiDayn/Postizer.git")
	if err != nil {
		t.Fatal(err)
	}
	if release.TagName != "v0.1.8" || release.HTMLURL == "" || release.PublishedAt.IsZero() {
		t.Fatalf("release = %#v", release)
	}
}

func TestLatestReleaseRetriesInvalidTokenAnonymously(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v0.1.8"}`))
	}))
	defer server.Close()

	release, err := (Client{HTTPClient: server.Client(), APIBase: server.URL, Token: "stale"}).LatestRelease(context.Background(), "https://github.com/WiDayn/Postizer.git")
	if err != nil {
		t.Fatal(err)
	}
	if release.TagName != "v0.1.8" || calls.Load() != 2 {
		t.Fatalf("release = %#v, calls = %d", release, calls.Load())
	}
}

func TestLatestReleaseExplainsHTMLResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><title>Proxy authentication required</title></html>`))
	}))
	defer server.Close()

	_, err := (Client{HTTPClient: server.Client(), APIBase: server.URL}).LatestRelease(context.Background(), "https://github.com/WiDayn/Postizer")
	if err == nil || !strings.Contains(err.Error(), "invalid JSON") || !strings.Contains(err.Error(), "Proxy authentication required") {
		t.Fatalf("error = %v", err)
	}
}
