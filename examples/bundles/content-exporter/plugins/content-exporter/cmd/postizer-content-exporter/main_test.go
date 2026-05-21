package main

import "testing"

func TestFormatBytes(t *testing.T) {
	if got, want := formatBytes(1536), "1.5 KB"; got != want {
		t.Fatalf("formatBytes = %q, want %q", got, want)
	}
}
