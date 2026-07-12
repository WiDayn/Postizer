package updatecheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"postizer/internal/marketplace"
)

const (
	DefaultAPIBase = "https://api.github.com"
	maxBodyBytes   = 1 << 20
)

type Release struct {
	TagName     string    `json:"tag_name"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
}

type Client struct {
	HTTPClient *http.Client
	APIBase    string
	Token      string
}

func (c Client) LatestRelease(ctx context.Context, repoURL string) (Release, error) {
	slug, err := marketplace.GitHubRepoSlug(repoURL)
	if err != nil {
		return Release{}, fmt.Errorf("GitHub repository: %w", err)
	}
	apiBase := strings.TrimRight(strings.TrimSpace(c.APIBase), "/")
	if apiBase == "" {
		apiBase = DefaultAPIBase
	}
	endpoint := apiBase + "/repos/" + slug + "/releases/latest"

	release, status, err := c.request(ctx, endpoint, strings.TrimSpace(c.Token))
	if err == nil {
		return release, nil
	}
	// A stale or incorrectly scoped token should not break update checks for a
	// public repository. Retry once anonymously while preserving other errors.
	if strings.TrimSpace(c.Token) != "" && (status == http.StatusUnauthorized || status == http.StatusForbidden) {
		if anonymous, _, anonymousErr := c.request(ctx, endpoint, ""); anonymousErr == nil {
			return anonymous, nil
		} else {
			return Release{}, fmt.Errorf("%v; anonymous retry failed: %w", err, anonymousErr)
		}
	}
	return Release{}, err
}

func (c Client) request(ctx context.Context, endpoint, token string) (Release, int, error) {
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Release{}, 0, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "Postizer Update Checker")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		return Release{}, 0, fmt.Errorf("request GitHub latest release: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBodyBytes+1))
	if err != nil {
		return Release{}, response.StatusCode, fmt.Errorf("read GitHub latest release: %w", err)
	}
	if len(body) > maxBodyBytes {
		return Release{}, response.StatusCode, fmt.Errorf("GitHub latest release response exceeds %d bytes", maxBodyBytes)
	}

	var payload struct {
		Release
		Message          string `json:"message"`
		DocumentationURL string `json:"documentation_url"`
	}
	decodeErr := json.Unmarshal(body, &payload)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		detail := strings.TrimSpace(payload.Message)
		if detail == "" {
			detail = responsePreview(body)
		}
		if detail == "" {
			detail = response.Status
		}
		if payload.DocumentationURL != "" {
			detail += " (" + payload.DocumentationURL + ")"
		}
		return Release{}, response.StatusCode, fmt.Errorf("GitHub latest release returned %s: %s", response.Status, detail)
	}
	if decodeErr != nil {
		return Release{}, response.StatusCode, fmt.Errorf("GitHub latest release returned invalid JSON (%s): %s", response.Header.Get("Content-Type"), responsePreview(body))
	}
	payload.TagName = strings.TrimSpace(payload.TagName)
	payload.HTMLURL = strings.TrimSpace(payload.HTMLURL)
	if payload.TagName == "" {
		return Release{}, response.StatusCode, errors.New("GitHub latest release response did not include tag_name: " + responsePreview(body))
	}
	return payload.Release, response.StatusCode, nil
}

func responsePreview(body []byte) string {
	compact := strings.Join(strings.Fields(string(bytes.TrimSpace(body))), " ")
	if len(compact) > 240 {
		compact = compact[:240] + "..."
	}
	return compact
}
