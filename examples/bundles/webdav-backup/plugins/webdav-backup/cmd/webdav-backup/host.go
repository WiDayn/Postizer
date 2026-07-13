package main

import (
	"context"
	"fmt"
	"strings"

	"postizer/pkg/pluginrpc"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func (s *server) hostClient(ctx context.Context) (pluginrpc.HostServiceClient, *grpc.ClientConn, error) {
	if strings.TrimSpace(s.hostAddr) == "" {
		return nil, nil, fmt.Errorf("Postizer 宿主服务不可用")
	}
	conn, err := grpc.NewClient(
		s.hostAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.ForceCodec(pluginrpc.Codec),
			grpc.MaxCallRecvMsgSize(maxPluginRPCMessageBytes),
			grpc.MaxCallSendMsgSize(maxPluginRPCMessageBytes),
		),
	)
	if err != nil {
		return nil, nil, err
	}
	return pluginrpc.NewHostServiceClient(conn), conn, nil
}
