package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const (
	defaultRemotePath = "Postizer/backups"
	defaultInterval   = 24
	defaultRetention  = 7
)

type config struct {
	ServerURL      string `json:"server_url"`
	RemotePath     string `json:"remote_path"`
	Username       string `json:"username,omitempty"`
	Password       string `json:"password,omitempty"`
	Enabled        bool   `json:"enabled"`
	IntervalHours  int    `json:"interval_hours"`
	RetentionCount int    `json:"retention_count"`
	SkipTLSVerify  bool   `json:"skip_tls_verify"`
}

func (c config) withDefaults() config {
	if strings.TrimSpace(c.RemotePath) == "" {
		c.RemotePath = defaultRemotePath
	}
	if c.IntervalHours == 0 {
		c.IntervalHours = defaultInterval
	}
	if c.RetentionCount == 0 {
		c.RetentionCount = defaultRetention
	}
	return c
}

func (c config) validate() error {
	u, err := url.Parse(strings.TrimSpace(c.ServerURL))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("WebDAV 服务器地址必须是有效的 http 或 https URL")
	}
	if u.User != nil {
		return fmt.Errorf("请使用用户名和密码字段，不要把凭据写入 WebDAV URL")
	}
	if strings.TrimSpace(c.RemotePath) == "" {
		return fmt.Errorf("远程备份目录不能为空")
	}
	for _, segment := range strings.FieldsFunc(c.RemotePath, func(r rune) bool { return r == '/' || r == '\\' }) {
		if segment == "." || segment == ".." {
			return fmt.Errorf("远程备份目录不能包含 . 或 .. 路径段")
		}
	}
	if c.IntervalHours < 1 || c.IntervalHours > 720 {
		return fmt.Errorf("自动备份间隔必须是 1–720 小时")
	}
	if c.RetentionCount < 1 || c.RetentionCount > 100 {
		return fmt.Errorf("保留备份数量必须是 1–100")
	}
	return nil
}

func configFromFields(fields map[string]string, previous config) (config, error) {
	interval, err := strconv.Atoi(strings.TrimSpace(fields["interval_hours"]))
	if err != nil {
		return config{}, fmt.Errorf("自动备份间隔必须是整数")
	}
	retention, err := strconv.Atoi(strings.TrimSpace(fields["retention_count"]))
	if err != nil {
		return config{}, fmt.Errorf("保留备份数量必须是整数")
	}
	password := fields["password"]
	if password == "" {
		password = previous.Password
	}
	cfg := config{
		ServerURL:      strings.TrimRight(strings.TrimSpace(fields["server_url"]), "/"),
		RemotePath:     strings.Trim(strings.TrimSpace(fields["remote_path"]), "/"),
		Username:       strings.TrimSpace(fields["username"]),
		Password:       password,
		Enabled:        fieldBool(fields["enabled"]),
		IntervalHours:  interval,
		RetentionCount: retention,
		SkipTLSVerify:  fieldBool(fields["skip_tls_verify"]),
	}
	return cfg, cfg.validate()
}

func fieldBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (s *server) configPath() string { return filepath.Join(s.dataDir, "config.json") }

func (s *server) loadConfig() (config, error) {
	body, err := os.ReadFile(s.configPath())
	if os.IsNotExist(err) {
		return (config{}).withDefaults(), nil
	}
	if err != nil {
		return config{}, err
	}
	var cfg config
	if err := json.Unmarshal(body, &cfg); err != nil {
		return config{}, fmt.Errorf("读取 WebDAV 设置: %w", err)
	}
	return cfg.withDefaults(), nil
}

func (s *server) saveConfig(cfg config) error {
	body, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	temporary := s.configPath() + ".tmp"
	if err := os.WriteFile(temporary, append(body, '\n'), 0600); err != nil {
		return err
	}
	defer os.Remove(temporary)
	if err := os.Chmod(temporary, 0600); err != nil {
		return err
	}
	return replaceFile(temporary, s.configPath())
}

func replaceFile(temporary, destination string) error {
	if err := os.Rename(temporary, destination); err == nil {
		return nil
	} else if runtime.GOOS != "windows" {
		return err
	}
	if err := os.Remove(destination); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(temporary, destination)
}
