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
	pluginID      = "wordpress-importer"
	pluginVersion = "1.0.0"

	maxPluginMediaBytes      = 64 << 20
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
	file, err := wxrFile(req)
	if err != nil {
		return nil, err
	}
	export, err := parseWXR(file.Body)
	if err != nil {
		return nil, err
	}

	switch req.ActionID {
	case "inspect_wxr":
		return inspect(export), nil
	case "import_wxr":
		batch, err := buildImportBatch(export)
		if err != nil {
			return nil, err
		}
		host, conn, err := s.hostClient(ctx)
		if err != nil {
			return nil, err
		}
		total := len(batch.Media) + len(batch.Posts) + len(batch.Pages)
		if total == 0 {
			total = 1
		}
		job, err := host.CreateJob(ctx, &pluginrpc.CreateJobRequest{
			PluginID: pluginID,
			Title:    "WordPress import started.",
			Total:    total,
		})
		if err != nil {
			_ = conn.Close()
			return nil, err
		}
		go s.runImport(context.Background(), host, conn, job.ID, batch)

		return &pluginrpc.InvokeActionResponse{
			Title:   "WordPress import started",
			Summary: fmt.Sprintf("Importing %d media files, %d posts, and %d pages.", len(batch.Media), len(batch.Posts), len(batch.Pages)),
			Job:     job,
		}, nil
	default:
		return nil, fmt.Errorf("unknown action %q", req.ActionID)
	}
}

func (s *server) Shutdown(context.Context, *pluginrpc.ShutdownRequest) (*pluginrpc.ShutdownResponse, error) {
	go s.grpcServer.GracefulStop()
	return &pluginrpc.ShutdownResponse{OK: true}, nil
}
