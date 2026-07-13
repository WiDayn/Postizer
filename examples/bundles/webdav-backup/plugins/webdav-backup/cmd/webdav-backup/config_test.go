package main

import (
	"os"
	"testing"
)

func TestConfigFromFieldsPreservesPasswordAndParsesCheckboxes(t *testing.T) {
	cfg, err := configFromFields(map[string]string{
		"server_url":      "https://dav.example.test/root/",
		"remote_path":     "/Postizer/backups/",
		"username":        "alice",
		"password":        "",
		"enabled":         "true",
		"interval_hours":  "12",
		"retention_count": "5",
		"skip_tls_verify": "on",
	}, config{Password: "existing-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Password != "existing-secret" {
		t.Fatalf("password = %q", cfg.Password)
	}
	if !cfg.Enabled || !cfg.SkipTLSVerify {
		t.Fatalf("checkbox values were not parsed: %#v", cfg)
	}
	if cfg.ServerURL != "https://dav.example.test/root" || cfg.RemotePath != "Postizer/backups" {
		t.Fatalf("paths were not normalized: %#v", cfg)
	}
}

func TestSaveConfigUsesPrivateFilePermissions(t *testing.T) {
	srv := &server{dataDir: t.TempDir()}
	cfg := config{ServerURL: "https://dav.example.test", RemotePath: "backups", IntervalHours: 24, RetentionCount: 7}
	if err := srv.saveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(srv.configPath())
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("config permissions = %o, want 600", got)
	}
}
