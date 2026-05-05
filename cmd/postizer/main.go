package main

import (
	"log"
	"net/http"
	"os"
	"time"
	_ "time/tzdata"

	apphttp "postizer/internal/http"
	"postizer/internal/media"
	"postizer/internal/site"
)

func main() {
	addr := env("POSTIZER_ADDR", ":8080")

	store, err := site.Load("content")
	if err != nil {
		log.Fatalf("load content: %v", err)
	}

	mediaStore, err := media.Open("media")
	if err != nil {
		log.Fatalf("open media store: %v", err)
	}
	handler, err := apphttp.New(store, mediaStore, "content")
	if err != nil {
		log.Fatalf("create server: %v", err)
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("Postizer listening on http://localhost%s", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
