package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"postizer/pkg/pluginrpc"
)

func (s *server) configResult() (*pluginrpc.InvokeActionResponse, error) {
	cfg, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	return &pluginrpc.InvokeActionResponse{FieldValues: map[string]string{
		"server_url":      cfg.ServerURL,
		"remote_path":     cfg.RemotePath,
		"username":        cfg.Username,
		"password":        "",
		"enabled":         strconv.FormatBool(cfg.Enabled),
		"interval_hours":  strconv.Itoa(cfg.IntervalHours),
		"retention_count": strconv.Itoa(cfg.RetentionCount),
		"skip_tls_verify": strconv.FormatBool(cfg.SkipTLSVerify),
	}}, nil
}

func (s *server) configure(fields map[string]string) (*pluginrpc.InvokeActionResponse, error) {
	previous, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	cfg, err := configFromFields(fields, previous)
	if err != nil {
		return nil, err
	}
	if err := s.saveConfig(cfg); err != nil {
		return nil, err
	}
	s.signalScheduler()
	mode := "自动备份未启用"
	if cfg.Enabled {
		mode = fmt.Sprintf("每 %d 小时自动备份一次", cfg.IntervalHours)
	}
	return &pluginrpc.InvokeActionResponse{
		Title:   "WebDAV 设置已保存",
		Summary: fmt.Sprintf("%s；远程保留最近 %d 份备份。", mode, cfg.RetentionCount),
		Level:   "success",
	}, nil
}

func (s *server) testConnection(ctx context.Context) (*pluginrpc.InvokeActionResponse, error) {
	cfg, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	client, err := newWebDAVClient(cfg)
	if err != nil {
		return nil, err
	}
	if err := client.ensureCollection(ctx); err != nil {
		return nil, err
	}
	return &pluginrpc.InvokeActionResponse{
		Title:   "WebDAV 连接正常",
		Summary: "认证成功，远程备份目录可以访问。",
		Level:   "success",
	}, nil
}

func (s *server) backupNow(_ context.Context) (*pluginrpc.InvokeActionResponse, error) {
	cfg, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if !s.backupMu.TryLock() {
		return nil, fmt.Errorf("已有备份任务正在运行")
	}
	go func() {
		defer s.backupMu.Unlock()
		backupContext, cancel := context.WithTimeout(context.Background(), 12*time.Hour)
		defer cancel()
		if err := s.performBackupLocked(backupContext, cfg); err != nil {
			// The error is persisted in state and shown by get_status.
			return
		}
	}()
	return &pluginrpc.InvokeActionResponse{
		Title:   "备份任务已开始",
		Summary: "正在后台生成并上传完整备份；稍后点击“刷新备份状态”查看结果。",
		Level:   "success",
	}, nil
}

func (s *server) performBackup(ctx context.Context, cfg config) error {
	if !s.backupMu.TryLock() {
		return fmt.Errorf("已有备份任务正在运行")
	}
	defer s.backupMu.Unlock()
	return s.performBackupLocked(ctx, cfg)
}

func (s *server) performBackupLocked(ctx context.Context, cfg config) error {
	if err := cfg.validate(); err != nil {
		return err
	}
	started := time.Now().UTC()
	_ = s.updateState(func(state *backupState) {
		state.LastAttempt = started
		state.LastError = ""
	})

	fail := func(err error) error {
		_ = s.updateState(func(state *backupState) { state.LastError = err.Error() })
		return err
	}
	host, conn, err := s.hostClient(ctx)
	if err != nil {
		return fail(err)
	}
	defer conn.Close()
	exported, err := host.CreateContentExport(ctx, &pluginrpc.CreateContentExportRequest{
		PluginID:  pluginID,
		LocalFile: true,
	})
	if err != nil {
		return fail(fmt.Errorf("生成 Postizer 内容归档: %w", err))
	}
	if strings.TrimSpace(exported.LocalPath) == "" {
		return fail(fmt.Errorf("Postizer 未返回临时归档路径，请升级宿主程序"))
	}
	defer os.Remove(exported.LocalPath)

	client, err := newWebDAVClient(cfg)
	if err != nil {
		return fail(err)
	}
	if err := client.ensureCollection(ctx); err != nil {
		return fail(err)
	}
	if err := client.upload(ctx, exported.LocalPath, exported.Filename); err != nil {
		return fail(err)
	}
	if err := client.prune(ctx, cfg.RetentionCount); err != nil {
		return fail(fmt.Errorf("备份已上传，但清理旧备份失败: %w", err))
	}
	completed := time.Now().UTC()
	return s.updateState(func(state *backupState) {
		state.LastSuccess = completed
		state.LastFilename = exported.Filename
		state.LastBytes = exported.Bytes
		state.LastError = ""
	})
}

func (s *server) statusResult() (*pluginrpc.InvokeActionResponse, error) {
	cfg, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	state, err := s.loadState()
	if err != nil {
		return nil, err
	}
	rows := []pluginrpc.ResultRow{
		{Label: "自动备份", Value: enabledLabel(cfg.Enabled)},
		{Label: "备份间隔", Value: fmt.Sprintf("%d 小时", cfg.IntervalHours)},
		{Label: "远程目录", Value: cfg.RemotePath},
	}
	if !state.LastAttempt.IsZero() {
		rows = append(rows, pluginrpc.ResultRow{Label: "最近尝试", Value: state.LastAttempt.Local().Format("2006-01-02 15:04:05")})
	}
	if !state.LastSuccess.IsZero() {
		rows = append(rows,
			pluginrpc.ResultRow{Label: "最近成功", Value: state.LastSuccess.Local().Format("2006-01-02 15:04:05")},
			pluginrpc.ResultRow{Label: "备份文件", Value: state.LastFilename},
			pluginrpc.ResultRow{Label: "文件大小", Value: formatBytes(state.LastBytes)},
		)
		if cfg.Enabled {
			next := state.LastSuccess.Add(time.Duration(cfg.IntervalHours) * time.Hour)
			rows = append(rows, pluginrpc.ResultRow{Label: "下次自动备份", Value: next.Local().Format("2006-01-02 15:04:05")})
		}
	}
	level := "success"
	title := "WebDAV 备份状态"
	summary := "尚未创建备份。"
	if !state.LastSuccess.IsZero() {
		summary = "最近一次备份已成功上传。"
	}
	if state.LastError != "" {
		level = "error"
		title = "最近一次备份失败"
		summary = state.LastError
		rows = append(rows, pluginrpc.ResultRow{Label: "错误", Value: state.LastError})
	}
	return &pluginrpc.InvokeActionResponse{
		Title:    title,
		Summary:  summary,
		Level:    level,
		Sections: []pluginrpc.ResultSection{{Title: "运行信息", Kind: "details", Rows: rows}},
	}, nil
}

func enabledLabel(enabled bool) string {
	if enabled {
		return "已启用"
	}
	return "未启用"
}

func formatBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(div), "KMGTPE"[exp])
}
