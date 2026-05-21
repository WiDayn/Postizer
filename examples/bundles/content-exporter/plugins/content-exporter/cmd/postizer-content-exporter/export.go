package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"postizer/pkg/pluginrpc"
)

func (s *server) exportAll(ctx context.Context) (*pluginrpc.InvokeActionResponse, error) {
	host, conn, err := s.hostClient(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	callCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	export, err := host.CreateContentExport(callCtx, &pluginrpc.CreateContentExportRequest{PluginID: pluginID})
	if err != nil {
		return nil, err
	}

	return &pluginrpc.InvokeActionResponse{
		Title:   "Export ready",
		Summary: fmt.Sprintf("Packed %d posts, %d pages, and %d media items into %s.", export.Posts, export.Pages, export.MediaItems, export.Filename),
		Sections: []pluginrpc.ResultSection{
			{
				Title: "Download",
				Kind:  "download",
				Rows: []pluginrpc.ResultRow{
					{Label: export.Filename, Value: export.DownloadURL},
				},
			},
			{
				Title: "Contents",
				Rows: []pluginrpc.ResultRow{
					{Label: "Posts", Value: strconv.Itoa(export.Posts)},
					{Label: "Pages", Value: strconv.Itoa(export.Pages)},
					{Label: "Media items", Value: strconv.Itoa(export.MediaItems)},
					{Label: "Media files", Value: strconv.Itoa(export.MediaFiles)},
					{Label: "Archive size", Value: formatBytes(export.Bytes)},
				},
			},
		},
	}, nil
}

func formatBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	value := float64(size)
	for _, suffix := range []string{"KB", "MB", "GB", "TB"} {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f PB", value/unit)
}
