package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
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
	handler, err := apphttp.New(store, mediaStore, contentRoot)
	if err != nil {
		log.Fatalf("create server: %v", err)
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	startSelfUpdateLoop(contentRoot)

	log.Printf("Postizer listening on http://localhost%s", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func startSelfUpdateLoop(contentRoot string) {
	command := strings.TrimSpace(os.Getenv("POSTIZER_SELF_UPDATE_COMMAND"))
	if command == "" {
		return
	}
	interval := envDuration("POSTIZER_SELF_UPDATE_INTERVAL", 15*time.Minute)
	initialDelay := envDuration("POSTIZER_SELF_UPDATE_INITIAL_DELAY", 5*time.Minute)
	timeout := envDuration("POSTIZER_SELF_UPDATE_TIMEOUT", 45*time.Minute)

	go func() {
		if initialDelay > 0 {
			time.Sleep(initialDelay)
		}
		for {
			runSelfUpdateIfEnabled(contentRoot, command, timeout)
			time.Sleep(interval)
		}
	}()
}

func runSelfUpdateIfEnabled(contentRoot, command string, timeout time.Duration) {
	settings, err := site.LoadSettings(contentRoot)
	if err != nil {
		log.Printf("self-update settings: %v", err)
		return
	}
	if !settings.AutoUpdate.Enabled {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if trimmed := strings.TrimSpace(string(output)); trimmed != "" {
		log.Printf("self-update output:\n%s", trimmed)
	}
	if err == nil {
		return
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 42 {
		log.Print("self-update completed; restarting Postizer")
		time.Sleep(2 * time.Second)
		os.Exit(0)
	}
	if ctx.Err() != nil {
		log.Printf("self-update timed out: %v", ctx.Err())
		return
	}
	log.Printf("self-update failed: %v", err)
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
