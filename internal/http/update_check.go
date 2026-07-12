package http

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"postizer/internal/marketplace"
	"postizer/internal/updatecheck"
)

const defaultGitHubAPIBase = updatecheck.DefaultAPIBase

type updateCheckResponse struct {
	CurrentVersion  string    `json:"current_version"`
	LatestVersion   string    `json:"latest_version"`
	UpdateAvailable bool      `json:"update_available"`
	Comparable      bool      `json:"comparable"`
	ReleaseURL      string    `json:"release_url,omitempty"`
	PublishedAt     time.Time `json:"published_at,omitempty"`
	CheckedAt       time.Time `json:"checked_at"`
}

func (s *Server) checkForUpdates(w http.ResponseWriter, r *http.Request) {
	client := s.updateClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	apiBase := strings.TrimSpace(os.Getenv("POSTIZER_GITHUB_API_URL"))
	if apiBase == "" {
		apiBase = defaultGitHubAPIBase
	}
	result, err := fetchLatestRelease(
		r.Context(),
		client,
		env("POSTIZER_REPO_URL", "https://github.com/WiDayn/Postizer.git"),
		apiBase,
		strings.TrimSpace(os.Getenv("POSTIZER_GITHUB_TOKEN")),
		currentAppVersion(),
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, result)
}

func fetchLatestRelease(ctx context.Context, client *http.Client, repoURL, apiBase, token, currentVersion string) (updateCheckResponse, error) {
	slug, err := marketplace.GitHubRepoSlug(repoURL)
	if err != nil {
		return updateCheckResponse{}, fmt.Errorf("check updates: invalid POSTIZER_REPO_URL: %w", err)
	}
	release, err := (updatecheck.Client{HTTPClient: client, APIBase: apiBase, Token: token}).LatestRelease(ctx, repoURL)
	if err != nil {
		return updateCheckResponse{}, fmt.Errorf("check updates: %w", err)
	}
	latestVersion := release.TagName
	comparison, ok := marketplace.CompareVersions(latestVersion, currentVersion)
	if !ok {
		return updateCheckResponse{}, fmt.Errorf("check updates: cannot compare current version %q with latest version %q", currentVersion, latestVersion)
	}
	releaseURL := release.HTMLURL
	if releaseURL == "" {
		releaseURL = "https://github.com/" + slug + "/releases/tag/" + latestVersion
	}
	return updateCheckResponse{
		CurrentVersion:  currentVersion,
		LatestVersion:   latestVersion,
		UpdateAvailable: comparison > 0,
		Comparable:      true,
		ReleaseURL:      releaseURL,
		PublishedAt:     release.PublishedAt,
		CheckedAt:       time.Now().UTC(),
	}, nil
}
