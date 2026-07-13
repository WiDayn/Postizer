package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"postizer/pkg/pluginrpc"

	"google.golang.org/grpc"
)

const (
	pluginID      = "webdav-backup"
	pluginVersion = "1.0.0"

	maxPluginRPCMessageBytes = 4 << 20
)

type server struct {
	grpcServer *grpc.Server
	dataDir    string
	hostAddr   string
	backupMu   sync.Mutex
	stateMu    sync.Mutex
	wake       chan struct{}
	stop       chan struct{}
	stopOnce   sync.Once
}

func main() {
	addr := strings.TrimSpace(os.Getenv("POSTIZER_PLUGIN_ADDR"))
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	dataDir := strings.TrimSpace(os.Getenv("POSTIZER_PLUGIN_DATA_DIR"))
	if dataDir == "" {
		dataDir = filepath.Join(strings.TrimSpace(os.Getenv("POSTIZER_PLUGIN_ROOT")), "data")
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		log.Fatal(err)
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	grpcServer := grpc.NewServer(
		grpc.ForceServerCodec(pluginrpc.Codec),
		grpc.MaxRecvMsgSize(maxPluginRPCMessageBytes),
		grpc.MaxSendMsgSize(maxPluginRPCMessageBytes),
	)
	srv := &server{
		grpcServer: grpcServer,
		dataDir:    dataDir,
		hostAddr:   strings.TrimSpace(os.Getenv("POSTIZER_HOST_ADDR")),
		wake:       make(chan struct{}, 1),
		stop:       make(chan struct{}),
	}
	pluginrpc.RegisterPluginServiceServer(grpcServer, srv)
	go srv.runScheduler()

	startup, _ := json.Marshal(map[string]string{
		"protocol": pluginrpc.ProtocolVersion,
		"endpoint": listener.Addr().String(),
	})
	fmt.Println(string(startup))
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatal(err)
	}
}

func (s *server) Handshake(context.Context, *pluginrpc.HandshakeRequest) (*pluginrpc.HandshakeResponse, error) {
	return &pluginrpc.HandshakeResponse{
		ProtocolVersion: pluginrpc.ProtocolVersion,
		PluginID:        pluginID,
		PluginVersion:   pluginVersion,
		Ready:           true,
	}, nil
}

func (s *server) InvokeAction(ctx context.Context, req *pluginrpc.InvokeActionRequest) (*pluginrpc.InvokeActionResponse, error) {
	fields := req.Fields
	if fields == nil {
		fields = map[string]string{}
	}
	var (
		result *pluginrpc.InvokeActionResponse
		err    error
	)
	switch req.ActionID {
	case "get_config":
		result, err = s.configResult()
	case "configure":
		result, err = s.configure(fields)
	case "test_connection":
		result, err = s.testConnection(ctx)
	case "backup_now":
		result, err = s.backupNow(ctx)
	case "get_status":
		result, err = s.statusResult()
	default:
		return nil, fmt.Errorf("unknown action %q", req.ActionID)
	}
	if err != nil {
		return actionError(req.ActionID, err), nil
	}
	return result, nil
}

func actionError(action string, err error) *pluginrpc.InvokeActionResponse {
	title := "操作失败"
	switch action {
	case "configure":
		title = "保存设置失败"
	case "test_connection":
		title = "WebDAV 连接失败"
	case "backup_now":
		title = "备份失败"
	case "get_status", "get_config":
		title = "读取备份状态失败"
	}
	message := strings.TrimSpace(err.Error())
	if len(message) > 1200 {
		message = message[:1200] + "..."
	}
	return &pluginrpc.InvokeActionResponse{Title: title, Summary: message, Level: "error"}
}

func (s *server) Shutdown(context.Context, *pluginrpc.ShutdownRequest) (*pluginrpc.ShutdownResponse, error) {
	s.stopOnce.Do(func() { close(s.stop) })
	go s.grpcServer.GracefulStop()
	return &pluginrpc.ShutdownResponse{OK: true}, nil
}

func (s *server) runScheduler() {
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-s.wake:
		case <-timer.C:
		}

		cfg, err := s.loadConfig()
		if err == nil && cfg.Enabled && s.backupDue(cfg, time.Now()) {
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Hour)
			if err := s.performBackup(ctx, cfg); err != nil {
				log.Printf("automatic WebDAV backup: %v", err)
			}
			cancel()
		}
		timer.Reset(time.Minute)
	}
}

func (s *server) signalScheduler() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}
