package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"postizer/internal/site"
)

func TestWriteUpdateRequest(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "nested", "update-request")
	if err := writeUpdateRequest(filename, " v0.1.11 "); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(body), "v0.1.11\n"; got != want {
		t.Fatalf("request = %q, want %q", got, want)
	}
	info, err := os.Stat(filename)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
}

func TestSelfUpdateFailureMessagePrefersReadableOutput(t *testing.T) {
	output := "POSTIZER_UPDATE_EVENT\t2026-06-02T15:28:00Z\tupdate_failed\t\tStructured message\nCould not find tag_name in GitHub latest release response."

	got := selfUpdateFailureMessage(errors.New("exit status 1"), output)
	want := "Could not find tag_name in GitHub latest release response."
	if got != want {
		t.Fatalf("selfUpdateFailureMessage = %q, want %q", got, want)
	}
}

func TestUpdateLogEntriesFromOutputPreservesMessageTabs(t *testing.T) {
	output := "POSTIZER_UPDATE_EVENT\t2026-06-02T15:28:00Z\tupdate_failed\tv0.1.4\tGitHub message\twith detail"

	entries := updateLogEntriesFromOutput(output)
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if entries[0].Event != site.UpdateEventFailed {
		t.Fatalf("event = %q, want %q", entries[0].Event, site.UpdateEventFailed)
	}
	if entries[0].Version != "v0.1.4" {
		t.Fatalf("version = %q, want v0.1.4", entries[0].Version)
	}
	if entries[0].Message != "GitHub message\twith detail" {
		t.Fatalf("message = %q, want preserved tab detail", entries[0].Message)
	}
	if want := time.Date(2026, 6, 2, 15, 28, 0, 0, time.UTC); !entries[0].Time.Equal(want) {
		t.Fatalf("time = %s, want %s", entries[0].Time, want)
	}
}
