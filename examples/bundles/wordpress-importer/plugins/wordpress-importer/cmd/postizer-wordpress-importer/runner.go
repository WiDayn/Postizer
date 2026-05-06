package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"postizer/pkg/pluginrpc"

	"google.golang.org/grpc"
)

func (s *server) runImport(ctx context.Context, host pluginrpc.HostServiceClient, conn *grpc.ClientConn, jobID string, batch *importBatch) {
	defer conn.Close()
	total := len(batch.Media) + len(batch.Posts) + len(batch.Pages)
	if total == 0 {
		total = 1
	}
	done := 0
	errors := 0
	mediaPaths := map[string]string{}
	mediaRows := []pluginrpc.ResultRow{}
	postRows := []pluginrpc.ResultRow{}
	pageRows := []pluginrpc.ResultRow{}

	update := func(req *pluginrpc.UpdateJobRequest) {
		req.PluginID = pluginID
		req.JobID = jobID
		if req.Total == 0 {
			req.Total = total
		}
		if req.Done == 0 {
			req.Done = done
		}
		callCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		if _, err := host.UpdateJob(callCtx, req); err != nil {
			log.Printf("update import job %s: %v", jobID, err)
		}
	}

	update(&pluginrpc.UpdateJobRequest{
		Log: fmt.Sprintf("Prepared %d media files, %d posts, and %d pages.", len(batch.Media), len(batch.Posts), len(batch.Pages)),
	})
	for _, item := range batch.Media {
		update(&pluginrpc.UpdateJobRequest{Log: "Downloading media: " + item.SourceURL})
		saved, err := downloadAndSaveMedia(ctx, host, item)
		done++
		placeholder := mediaPlaceholderID(item.ID)
		if err != nil {
			errors++
			mediaPaths[placeholder] = item.SourceURL
			update(&pluginrpc.UpdateJobRequest{
				Error: fmt.Sprintf("Media failed: %s (%v)", item.SourceURL, err),
			})
			continue
		}
		mediaPaths[placeholder] = saved.Path
		mediaRows = append(mediaRows, pluginrpc.ResultRow{
			Label: fallback(item.OriginalName, item.ID),
			Value: saved.Path,
		})
		update(&pluginrpc.UpdateJobRequest{Log: "Media saved: " + saved.Path})
	}

	for _, draft := range batch.Posts {
		draft.Body = replaceMediaPlaceholders(draft.Body, mediaPaths)
		saved, err := savePost(ctx, host, draft)
		done++
		if err != nil {
			errors++
			update(&pluginrpc.UpdateJobRequest{
				Error: fmt.Sprintf("Post failed: %s (%v)", fallback(draft.Title, draft.Slug), err),
			})
			continue
		}
		postRows = append(postRows, pluginrpc.ResultRow{Label: saved.Title, Value: saved.URL})
		update(&pluginrpc.UpdateJobRequest{Log: "Post saved: " + saved.URL})
	}

	for _, draft := range batch.Pages {
		draft.Body = replaceMediaPlaceholders(draft.Body, mediaPaths)
		saved, err := savePage(ctx, host, draft)
		done++
		if err != nil {
			errors++
			update(&pluginrpc.UpdateJobRequest{
				Error: fmt.Sprintf("Page failed: %s (%v)", fallback(draft.Title, draft.Slug), err),
			})
			continue
		}
		pageRows = append(pageRows, pluginrpc.ResultRow{Label: saved.Title, Value: saved.URL})
		update(&pluginrpc.UpdateJobRequest{Log: "Page saved: " + saved.URL})
	}

	sections := importResultSections(mediaRows, postRows, pageRows, batch.Skipped)
	callCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	if _, err := host.ReloadRuntime(callCtx, &pluginrpc.ReloadRuntimeRequest{PluginID: pluginID}); err != nil {
		errors++
		update(&pluginrpc.UpdateJobRequest{Error: "Reload failed: " + err.Error()})
	} else {
		update(&pluginrpc.UpdateJobRequest{Log: "Postizer runtime reloaded."})
	}
	cancel()

	status := "completed"
	message := "Import completed."
	if errors > 0 {
		status = "completed_with_errors"
		message = fmt.Sprintf("Import completed with %d error(s).", errors)
	}
	update(&pluginrpc.UpdateJobRequest{
		Status:   status,
		Done:     total,
		Total:    total,
		Log:      message,
		Sections: sections,
	})
}

func downloadAndSaveMedia(ctx context.Context, host pluginrpc.HostServiceClient, item mediaFetch) (pluginrpc.MediaItem, error) {
	callCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(callCtx, http.MethodGet, item.SourceURL, nil)
	if err != nil {
		return pluginrpc.MediaItem{}, err
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return pluginrpc.MediaItem{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return pluginrpc.MediaItem{}, fmt.Errorf("download returned %s", response.Status)
	}
	if response.ContentLength > maxPluginMediaBytes {
		return pluginrpc.MediaItem{}, fmt.Errorf("media exceeds %d bytes", maxPluginMediaBytes)
	}
	body, err := readLimited(response.Body, maxPluginMediaBytes)
	if err != nil {
		return pluginrpc.MediaItem{}, err
	}
	saved, err := host.SaveMedia(callCtx, &pluginrpc.SaveMediaRequest{
		PluginID:     pluginID,
		OriginalName: item.OriginalName,
		Alt:          item.Alt,
		Caption:      item.Caption,
		ContentType:  response.Header.Get("Content-Type"),
		Body:         body,
	})
	if err != nil {
		return pluginrpc.MediaItem{}, err
	}
	return saved.Item, nil
}

func savePost(ctx context.Context, host pluginrpc.HostServiceClient, draft pluginrpc.ContentDraft) (*pluginrpc.SaveContentResponse, error) {
	callCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	return host.SavePost(callCtx, &draft)
}

func savePage(ctx context.Context, host pluginrpc.HostServiceClient, draft pluginrpc.ContentDraft) (*pluginrpc.SaveContentResponse, error) {
	callCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	return host.SavePage(callCtx, &draft)
}

func importResultSections(mediaRows, postRows, pageRows []pluginrpc.ResultRow, skipped []string) []pluginrpc.ResultSection {
	sections := []pluginrpc.ResultSection{
		{Title: "Imported media", Rows: mediaRows},
		{Title: "Imported posts", Rows: postRows},
		{Title: "Imported pages", Rows: pageRows},
	}
	if len(skipped) > 0 {
		rows := make([]pluginrpc.ResultRow, 0, len(skipped))
		for _, item := range skipped {
			rows = append(rows, pluginrpc.ResultRow{Label: item, Value: "skipped"})
		}
		sections = append(sections, pluginrpc.ResultSection{Title: "Skipped content", Rows: rows})
	}
	return sections
}
