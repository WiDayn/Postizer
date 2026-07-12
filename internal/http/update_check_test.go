package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchLatestReleaseDetectsUpdate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/repos/WiDayn/Postizer/releases/latest"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("Authorization"), "Bearer secret-token"; got != want {
			t.Fatalf("authorization = %q, want %q", got, want)
		}
		if got := r.Header.Get("User-Agent"); got != "Postizer Update Checker" {
			t.Fatalf("user agent = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v0.1.9","html_url":"https://github.com/WiDayn/Postizer/releases/tag/v0.1.9"}`))
	}))
	defer server.Close()

	result, err := fetchLatestRelease(context.Background(), server.Client(), "https://github.com/WiDayn/Postizer.git", server.URL, "secret-token", "v0.1.8")
	if err != nil {
		t.Fatal(err)
	}
	if !result.UpdateAvailable || result.LatestVersion != "v0.1.9" {
		t.Fatalf("result = %#v", result)
	}
}

func TestFetchLatestReleaseReportsUpToDate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v0.1.8"}`))
	}))
	defer server.Close()

	result, err := fetchLatestRelease(context.Background(), server.Client(), "https://github.com/WiDayn/Postizer", server.URL, "", "v0.1.8")
	if err != nil {
		t.Fatal(err)
	}
	if result.UpdateAvailable {
		t.Fatalf("result = %#v, want no update", result)
	}
	if !strings.HasSuffix(result.ReleaseURL, "/releases/tag/v0.1.8") {
		t.Fatalf("release url = %q", result.ReleaseURL)
	}
}

func TestFetchLatestReleasePreservesGitHubError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	}))
	defer server.Close()

	_, err := fetchLatestRelease(context.Background(), server.Client(), "https://github.com/WiDayn/Postizer", server.URL, "", "v0.1.8")
	if err == nil || !strings.Contains(err.Error(), "API rate limit exceeded") || !strings.Contains(err.Error(), "403") {
		t.Fatalf("error = %v", err)
	}
}

func TestFetchLatestReleaseRejectsSuccessfulResponseWithoutTag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"message":"proxy response"}`))
	}))
	defer server.Close()

	_, err := fetchLatestRelease(context.Background(), server.Client(), "https://github.com/WiDayn/Postizer", server.URL, "", "v0.1.8")
	if err == nil || !strings.Contains(err.Error(), "did not include tag_name") {
		t.Fatalf("error = %v", err)
	}
}

func TestFetchLatestReleaseRetriesBadTokenAnonymously(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
			return
		}
		_, _ = w.Write([]byte(`{"tag_name":"v0.1.8"}`))
	}))
	defer server.Close()

	result, err := fetchLatestRelease(context.Background(), server.Client(), "https://github.com/WiDayn/Postizer", server.URL, "stale-token", "v0.1.7")
	if err != nil {
		t.Fatal(err)
	}
	if !result.UpdateAvailable || calls != 2 {
		t.Fatalf("result = %#v, calls = %d", result, calls)
	}
}
