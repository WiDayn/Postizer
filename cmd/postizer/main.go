package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
	_ "time/tzdata"

	apphttp "postizer/internal/http"
	"postizer/internal/media"
	"postizer/internal/site"
)

func main() {
	addr := env("POSTIZER_ADDR", ":8080")
	contentRoot := env("POSTIZER_CONTENT_ROOT", "content")
	mediaRoot := env("POSTIZER_MEDIA_ROOT", "media")

	store, err := site.Load(contentRoot)
	if err != nil {
		log.Fatalf("load content: %v", err)
	}

	mediaStore, err := media.Open(mediaRoot)
	if err != nil {
		log.Fatalf("open media store: %v", err)
	}
	updateCommand := strings.TrimSpace(os.Getenv("POSTIZER_SELF_UPDATE_COMMAND"))
	updateRequestFile := strings.TrimSpace(os.Getenv("POSTIZER_UPDATE_REQUEST_FILE"))
	updateRequests := make(chan string, 1)
	var updatePending atomic.Bool
	var updateTrigger func(string) error
	if updateCommand != "" {
		updateTrigger = func(version string) error {
			if !updatePending.CompareAndSwap(false, true) {
				return fmt.Errorf("an update is already in progress")
			}
			updateRequests <- version
			return nil
		}
	} else if updateRequestFile != "" {
		updateTrigger = func(version string) error {
			return writeUpdateRequest(updateRequestFile, version)
		}
	}
	handler, err := apphttp.New(store, mediaStore, contentRoot, updateTrigger)
	if err != nil {
		log.Fatalf("create server: %v", err)
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	startSelfUpdateLoop(contentRoot, updateCommand, updateRequests, &updatePending)

	log.Printf("Postizer listening on http://localhost%s", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func writeUpdateRequest(filename, version string) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(filename), ".postizer-update-request-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.WriteString(strings.TrimSpace(version) + "\n"); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, filename)
}

func startSelfUpdateLoop(contentRoot, command string, requests <-chan string, pending *atomic.Bool) {
	if command == "" {
		return
	}
	interval := envDuration("POSTIZER_SELF_UPDATE_INTERVAL", 15*time.Minute)
	initialDelay := envDuration("POSTIZER_SELF_UPDATE_INITIAL_DELAY", 5*time.Minute)
	timeout := envDuration("POSTIZER_SELF_UPDATE_TIMEOUT", 45*time.Minute)

	go func() {
		timer := time.NewTimer(initialDelay)
		defer timer.Stop()
		for {
			select {
			case version := <-requests:
				runSelfUpdate(contentRoot, command, timeout, true, version)
				pending.Store(false)
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(interval)
			case <-timer.C:
				runSelfUpdate(contentRoot, command, timeout, false, "")
				timer.Reset(interval)
			}
		}
	}()
}

func runSelfUpdateIfEnabled(contentRoot, command string, timeout time.Duration) {
	runSelfUpdate(contentRoot, command, timeout, false, "")
}

func runSelfUpdate(contentRoot, command string, timeout time.Duration, manual bool, version string) {
	if !manual {
		settings, err := site.LoadSettings(contentRoot)
		if err != nil {
			log.Printf("self-update settings: %v", err)
			return
		}
		if !settings.AutoUpdate.Enabled {
			return
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	startedAt := time.Now().UTC()

	cmd := exec.CommandContext(ctx, command)
	cmd.Env = os.Environ()
	if strings.TrimSpace(version) != "" {
		cmd.Env = append(cmd.Env, "POSTIZER_RELEASE_VERSION="+strings.TrimSpace(version))
	}
	output, err := cmd.CombinedOutput()
	outputText := strings.TrimSpace(string(output))
	entries := updateLogEntriesFromOutput(outputText)
	for _, entry := range entries {
		appendUpdateLog(contentRoot, entry)
	}
	if outputText != "" {
		log.Printf("self-update output:\n%s", outputText)
	}
	if err == nil {
		return
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 42 {
		if !updateLogHasEvent(entries, site.UpdateEventDetected) {
			appendUpdateLog(contentRoot, site.UpdateLogEntry{
				Time:    startedAt,
				Event:   site.UpdateEventDetected,
				Version: updateVersionFromEntries(entries),
				Message: "Detected an available update and started applying it.",
			})
		}
		if !updateLogHasEvent(entries, site.UpdateEventCompleted) {
			appendUpdateLog(contentRoot, site.UpdateLogEntry{
				Time:    time.Now().UTC(),
				Event:   site.UpdateEventCompleted,
				Version: updateVersionFromEntries(entries),
				Message: "Updated the local runtime; restarting Postizer.",
			})
		}
		log.Print("self-update completed; restarting Postizer")
		time.Sleep(2 * time.Second)
		os.Exit(0)
	}
	if !updateLogHasEvent(entries, site.UpdateEventFailed) {
		appendUpdateLog(contentRoot, site.UpdateLogEntry{
			Time:    time.Now().UTC(),
			Event:   site.UpdateEventFailed,
			Version: updateVersionFromEntries(entries),
			Message: selfUpdateFailureMessage(err, outputText),
		})
	}
	if ctx.Err() != nil {
		log.Printf("self-update timed out: %v", ctx.Err())
		return
	}
	log.Printf("self-update failed: %v", err)
}

func updateLogEntriesFromOutput(output string) []site.UpdateLogEntry {
	entries := []site.UpdateLogEntry{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "POSTIZER_UPDATE_EVENT\t") {
			continue
		}
		parts := strings.SplitN(line, "\t", 5)
		if len(parts) < 4 {
			continue
		}
		eventTime, err := time.Parse(time.RFC3339, strings.TrimSpace(parts[1]))
		if err != nil {
			eventTime = time.Now().UTC()
		}
		event := strings.TrimSpace(parts[2])
		if event != site.UpdateEventDetected && event != site.UpdateEventCompleted && event != site.UpdateEventFailed {
			continue
		}
		entry := site.UpdateLogEntry{
			Time:    eventTime,
			Event:   event,
			Version: strings.TrimSpace(parts[3]),
		}
		if len(parts) >= 5 {
			entry.Message = strings.TrimSpace(parts[4])
		}
		entries = append(entries, entry)
	}
	return entries
}

func appendUpdateLog(contentRoot string, entry site.UpdateLogEntry) {
	if err := site.AppendUpdateLogEntry(contentRoot, entry); err != nil {
		log.Printf("append update log: %v", err)
	}
}

func updateLogHasEvent(entries []site.UpdateLogEntry, event string) bool {
	for _, entry := range entries {
		if entry.Event == event {
			return true
		}
	}
	return false
}

func updateVersionFromEntries(entries []site.UpdateLogEntry) string {
	for _, entry := range entries {
		if entry.Version != "" {
			return entry.Version
		}
	}
	return ""
}

func selfUpdateFailureMessage(err error, output string) string {
	if output == "" {
		return err.Error()
	}
	lines := strings.Split(output, "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		line := strings.TrimSpace(lines[index])
		if line != "" && !strings.HasPrefix(line, "POSTIZER_UPDATE_EVENT\t") {
			return line
		}
	}
	return err.Error()
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return fallback
	}
	return duration
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
