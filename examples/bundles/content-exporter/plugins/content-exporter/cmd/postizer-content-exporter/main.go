package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"strings"

	"postizer/pkg/pluginrpc"

	"google.golang.org/grpc"
)

const (
	pluginID      = "content-exporter"
	pluginVersion = "1.0.0"

	maxPluginRPCMessageBytes = 96 << 20
)

type server struct {
	grpcServer *grpc.Server
	hostAddr   string
}

func main() {
	addr := strings.TrimSpace(os.Getenv("POSTIZER_PLUGIN_ADDR"))
	if addr == "" {
		addr = "127.0.0.1:0"
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
		hostAddr:   strings.TrimSpace(os.Getenv("POSTIZER_HOST_ADDR")),
	}
	pluginrpc.RegisterPluginServiceServer(grpcServer, srv)

	startup := map[string]string{
		"protocol": pluginrpc.ProtocolVersion,
		"endpoint": listener.Addr().String(),
	}
	body, _ := json.Marshal(startup)
	fmt.Println(string(body))

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
	switch req.ActionID {
	case "export_all":
		return s.exportAll(ctx)
	default:
		return nil, fmt.Errorf("unknown action %q", req.ActionID)
	}
}

func (s *server) Shutdown(context.Context, *pluginrpc.ShutdownRequest) (*pluginrpc.ShutdownResponse, error) {
	go s.grpcServer.GracefulStop()
	return &pluginrpc.ShutdownResponse{OK: true}, nil
}
