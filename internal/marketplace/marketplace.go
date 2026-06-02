package marketplace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	SchemaVersion           = 1
	MaxIndexBytes           = 1 << 20
	MaxReleaseAssetBytes    = 32 << 20
	MaxReleaseChecksumBytes = 1 << 20
	ReleaseChecksumAsset    = "SHA256SUMS"
	DefaultPackIndexURL     = "https://raw.githubusercontent.com/WiDayn/Postizer/main/marketplace/packs/index.json"
)

type Index struct {
	Schema    int        `json:"schema"`
	UpdatedAt string     `json:"updated_at"`
	Items     []PackItem `json:"items"`
}

type PackItem struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Summary     string       `json:"summary"`
	Description string       `json:"description"`
	Repo        string       `json:"repo"`
	Preview     string       `json:"preview"`
	Tags        []string     `json:"tags"`
	Themes      []PackMember `json:"themes"`
	Plugins     []PackMember `json:"plugins"`
	Release     Release      `json:"release"`
	MinPostizer string       `json:"min_postizer"`
}

type PackMember struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

type Release struct {
	Tag   string `json:"tag"`
	Asset string `json:"asset"`
}

func LoadIndex(ctx context.Context, source string, client *http.Client) (Index, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		source = DefaultPackIndexURL
	}
	body, err := readSource(ctx, source, MaxIndexBytes, client)
	if err != nil {
		return Index{}, err
	}

	var index Index
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(&index); err != nil {
		return Index{}, fmt.Errorf("parse marketplace index: %w", err)
	}
	if err := validateIndex(&index); err != nil {
		return Index{}, err
	}
	return index, nil
}

func FindPack(index Index, id string) (PackItem, bool) {
	id = strings.TrimSpace(id)
	for _, item := range index.Items {
		if item.ID == id {
			return item, true
		}
	}
	return PackItem{}, false
}

func ReleaseAssetURL(item PackItem) (string, error) {
	asset := strings.TrimSpace(item.Release.Asset)
	if !validReleaseAssetName(asset) {
		return "", fmt.Errorf("release asset must be a .zip filename: %q", asset)
	}
	return releaseAssetURL(item, asset)
}

func ReleaseChecksumURL(item PackItem) (string, error) {
	return releaseAssetURL(item, ReleaseChecksumAsset)
}

func releaseAssetURL(item PackItem, asset string) (string, error) {
	slug, err := GitHubRepoSlug(item.Repo)
	if err != nil {
		return "", err
	}
	tag := strings.TrimSpace(item.Release.Tag)
	if !validReleaseTag(tag) {
		return "", fmt.Errorf("release tag must match vX.X.X: %q", tag)
	}
	if !validReleaseDownloadName(asset) {
		return "", fmt.Errorf("release asset filename is invalid: %q", asset)
	}
	return "https://github.com/" + slug + "/releases/download/" + url.PathEscape(tag) + "/" + url.PathEscape(asset), nil
}

func DownloadReleaseAsset(ctx context.Context, client *http.Client, item PackItem) ([]byte, string, error) {
	assetURL, err := ReleaseAssetURL(item)
	if err != nil {
		return nil, "", err
	}
	body, err := downloadReleaseURL(ctx, client, assetURL, MaxReleaseAssetBytes, "download release asset")
	if err != nil {
		return nil, "", err
	}
	return body, assetURL, nil
}

func DownloadReleaseChecksums(ctx context.Context, client *http.Client, item PackItem) ([]byte, string, error) {
	checksumURL, err := ReleaseChecksumURL(item)
	if err != nil {
		return nil, "", err
	}
	body, err := downloadReleaseURL(ctx, client, checksumURL, MaxReleaseChecksumBytes, "download release checksums")
	if err != nil {
		return nil, "", err
	}
	return body, checksumURL, nil
}

func downloadReleaseURL(ctx context.Context, client *http.Client, assetURL string, limit int64, label string) ([]byte, error) {
	if client == nil {
		client = http.DefaultClient
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "Postizer Resource Marketplace")
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%s: GitHub returned %s", label, response.Status)
	}
	body, err := readLimited(response.Body, limit)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func VerifyReleaseAsset(item PackItem, body, checksumBody []byte) error {
	want, err := releaseAssetChecksum(item.Release.Asset, checksumBody)
	if err != nil {
		return fmt.Errorf("verify checksum for resource pack %q: %w", item.ID, err)
	}
	sum := sha256.Sum256(body)
	got := hex.EncodeToString(sum[:])
	if got != want {
		return fmt.Errorf("sha256 mismatch for resource pack %q", item.ID)
	}
	return nil
}

func releaseAssetChecksum(asset string, checksumBody []byte) (string, error) {
	asset = strings.TrimSpace(asset)
	for lineNumber, rawLine := range strings.Split(string(checksumBody), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if len(line) < sha256.Size*2+1 {
			return "", fmt.Errorf("%s line %d is invalid", ReleaseChecksumAsset, lineNumber+1)
		}
		digest := strings.ToLower(line[:sha256.Size*2])
		if err := validateSHA256(digest); err != nil {
			return "", fmt.Errorf("%s line %d has an invalid SHA-256 digest", ReleaseChecksumAsset, lineNumber+1)
		}
		filename := strings.TrimSpace(line[sha256.Size*2:])
		filename = strings.TrimPrefix(filename, "*")
		if filename == asset {
			return digest, nil
		}
	}
	return "", fmt.Errorf("%s does not include checksum for %q", ReleaseChecksumAsset, asset)
}

func GitHubRepoSlug(repo string) (string, error) {
	repo = strings.TrimSpace(repo)
	parsed, err := url.Parse(repo)
	if err != nil {
		return "", fmt.Errorf("repo must be a valid GitHub URL: %w", err)
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if parsed.Scheme != "https" || parsed.User != nil || (host != "github.com" && host != "www.github.com") {
		return "", errors.New("repo must be an https://github.com/... URL")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("repo URL must not include query or fragment")
	}
	parts := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	if len(parts) != 2 {
		return "", errors.New("repo URL must point to a GitHub owner/repository")
	}
	owner, err := url.PathUnescape(parts[0])
	if err != nil {
		return "", errors.New("repo owner is invalid")
	}
	name, err := url.PathUnescape(parts[1])
	if err != nil {
		return "", errors.New("repo name is invalid")
	}
	name = strings.TrimSuffix(name, ".git")
	if !validGitHubSegment(owner) || !validGitHubSegment(name) {
		return "", errors.New("repo URL contains an invalid GitHub owner or repository name")
	}
	return owner + "/" + name, nil
}

func SameGitHubRepo(left, right string) bool {
	leftSlug, leftErr := GitHubRepoSlug(left)
	rightSlug, rightErr := GitHubRepoSlug(right)
	return leftErr == nil && rightErr == nil && strings.EqualFold(leftSlug, rightSlug)
}

func ResolvePreviewURL(source, preview string) string {
	preview = strings.TrimSpace(preview)
	if preview == "" || strings.HasPrefix(preview, "/") {
		return preview
	}
	parsedPreview, err := url.Parse(preview)
	if err == nil && parsedPreview.IsAbs() {
		switch parsedPreview.Scheme {
		case "http", "https":
			return parsedPreview.String()
		default:
			return ""
		}
	}

	parsedSource, err := url.Parse(strings.TrimSpace(source))
	if err != nil || !parsedSource.IsAbs() || (parsedSource.Scheme != "http" && parsedSource.Scheme != "https") {
		return preview
	}
	resolved := parsedSource.ResolveReference(&url.URL{Path: preview})
	return resolved.String()
}

func CompareVersions(left, right string) (int, bool) {
	leftParts, ok := parseVersion(left)
	if !ok {
		return 0, false
	}
	rightParts, ok := parseVersion(right)
	if !ok {
		return 0, false
	}
	for i := 0; i < len(leftParts); i++ {
		if leftParts[i] > rightParts[i] {
			return 1, true
		}
		if leftParts[i] < rightParts[i] {
			return -1, true
		}
	}
	return 0, true
}

func validateIndex(index *Index) error {
	if index.Schema != SchemaVersion {
		return fmt.Errorf("unsupported marketplace schema %d", index.Schema)
	}
	if len(index.Items) > 200 {
		return errors.New("marketplace index contains too many items")
	}
	seen := map[string]bool{}
	for i := range index.Items {
		item := normalizePackItem(index.Items[i])
		if !validID(item.ID) {
			return fmt.Errorf("items[%d].id is invalid", i)
		}
		if seen[item.ID] {
			return fmt.Errorf("items[%d].id duplicates %q", i, item.ID)
		}
		seen[item.ID] = true
		if item.Name == "" {
			return fmt.Errorf("items[%d].name is required", i)
		}
		if item.Summary == "" {
			return fmt.Errorf("items[%d].summary is required", i)
		}
		if len(item.Themes) == 0 && len(item.Plugins) == 0 {
			return fmt.Errorf("items[%d] must list at least one theme or plugin", i)
		}
		if containsString(item.Tags, "theme") && len(item.Themes) == 0 {
			return fmt.Errorf("items[%d].tags includes theme but themes is empty", i)
		}
		if containsString(item.Tags, "plugin") && len(item.Plugins) == 0 {
			return fmt.Errorf("items[%d].tags includes plugin but plugins is empty", i)
		}
		for tagIndex, tag := range item.Tags {
			if !validID(tag) {
				return fmt.Errorf("items[%d].tags[%d] is invalid", i, tagIndex)
			}
		}
		if err := validatePackMembers(item.Themes, "themes"); err != nil {
			return fmt.Errorf("items[%d].%w", i, err)
		}
		if err := validatePackMembers(item.Plugins, "plugins"); err != nil {
			return fmt.Errorf("items[%d].%w", i, err)
		}
		if _, err := GitHubRepoSlug(item.Repo); err != nil {
			return fmt.Errorf("items[%d].repo: %w", i, err)
		}
		if _, err := ReleaseAssetURL(item); err != nil {
			return fmt.Errorf("items[%d].release: %w", i, err)
		}
		if item.MinPostizer != "" && !validLooseVersion(item.MinPostizer) {
			return fmt.Errorf("items[%d].min_postizer must match X.X.X or vX.X.X", i)
		}
		index.Items[i] = item
	}
	return nil
}

func normalizePackItem(item PackItem) PackItem {
	item.ID = strings.TrimSpace(item.ID)
	item.Name = strings.TrimSpace(item.Name)
	item.Summary = strings.TrimSpace(item.Summary)
	item.Description = strings.TrimSpace(item.Description)
	item.Repo = strings.TrimSpace(item.Repo)
	item.Preview = strings.TrimSpace(item.Preview)
	item.Themes = normalizePackMembers(item.Themes)
	item.Plugins = normalizePackMembers(item.Plugins)
	item.Release.Tag = strings.TrimSpace(item.Release.Tag)
	item.Release.Asset = strings.TrimSpace(item.Release.Asset)
	item.MinPostizer = strings.TrimSpace(item.MinPostizer)

	tags := make([]string, 0, len(item.Tags))
	seen := map[string]bool{}
	for _, tag := range item.Tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		tags = append(tags, tag)
	}
	if len(item.Themes) > 0 && !seen["theme"] {
		tags = append([]string{"theme"}, tags...)
		seen["theme"] = true
	}
	if len(item.Plugins) > 0 && !seen["plugin"] {
		if seen["theme"] {
			tags = append([]string{tags[0], "plugin"}, tags[1:]...)
		} else {
			tags = append([]string{"plugin"}, tags...)
		}
	}
	item.Tags = tags
	return item
}

func normalizePackMembers(members []PackMember) []PackMember {
	normalized := make([]PackMember, 0, len(members))
	for _, member := range members {
		member.ID = strings.TrimSpace(member.ID)
		member.Name = strings.TrimSpace(member.Name)
		member.Version = strings.TrimSpace(member.Version)
		member.Description = strings.TrimSpace(member.Description)
		if member.ID == "" && member.Name == "" {
			continue
		}
		normalized = append(normalized, member)
	}
	return normalized
}

func validatePackMembers(members []PackMember, field string) error {
	seen := map[string]bool{}
	for i, member := range members {
		if !validID(member.ID) {
			return fmt.Errorf("%s[%d].id is invalid", field, i)
		}
		if seen[member.ID] {
			return fmt.Errorf("%s[%d].id duplicates %q", field, i, member.ID)
		}
		seen[member.ID] = true
		if member.Name == "" {
			return fmt.Errorf("%s[%d].name is required", field, i)
		}
	}
	return nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func readSource(ctx context.Context, source string, limit int64, client *http.Client) ([]byte, error) {
	parsed, err := url.Parse(source)
	if err == nil {
		switch parsed.Scheme {
		case "http", "https":
			return readHTTP(ctx, source, limit, client)
		case "file":
			return readFile(fileURLPath(parsed), limit)
		}
	}
	return readFile(source, limit)
}

func readHTTP(ctx context.Context, source string, limit int64, client *http.Client) ([]byte, error) {
	if client == nil {
		client = http.DefaultClient
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "Postizer Resource Marketplace")
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("read marketplace index: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("read marketplace index: server returned %s", response.Status)
	}
	return readLimited(response.Body, limit)
}

func readFile(filename string, limit int64) ([]byte, error) {
	body, err := os.ReadFile(filepath.Clean(filename))
	if err != nil {
		return nil, fmt.Errorf("read marketplace index: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("marketplace index is too large: limit is %d bytes", limit)
	}
	return body, nil
}

func readLimited(reader io.Reader, maxBytes int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("download is too large: limit is %d bytes", maxBytes)
	}
	return body, nil
}

func fileURLPath(parsed *url.URL) string {
	value, err := url.PathUnescape(parsed.Path)
	if err != nil {
		value = parsed.Path
	}
	if len(value) >= 3 && value[0] == '/' && value[2] == ':' {
		value = value[1:]
	}
	return filepath.FromSlash(value)
}

func validateSHA256(value string) error {
	decoded, err := hex.DecodeString(strings.TrimSpace(value))
	if err != nil || len(decoded) != sha256.Size {
		return errors.New("must be a hex-encoded SHA-256 digest")
	}
	return nil
}

func validID(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return false
	}
	return true
}

func validGitHubSegment(value string) bool {
	if value == "" || strings.HasPrefix(value, "-") || strings.HasSuffix(value, "-") {
		return false
	}
	for _, r := range value {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func validReleaseAssetName(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || !strings.HasSuffix(strings.ToLower(value), ".zip") {
		return false
	}
	return !strings.ContainsAny(value, `/\`) && !containsControl(value)
}

func validReleaseDownloadName(value string) bool {
	value = strings.TrimSpace(value)
	if value == ReleaseChecksumAsset {
		return true
	}
	return validReleaseAssetName(value)
}

func validReleaseTag(value string) bool {
	if !strings.HasPrefix(value, "v") {
		return false
	}
	return validLooseVersion(value)
}

func validLooseVersion(value string) bool {
	_, ok := parseVersion(value)
	return ok
}

func parseVersion(value string) ([3]int, bool) {
	var parts [3]int
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	chunks := strings.Split(value, ".")
	if len(chunks) != 3 {
		return parts, false
	}
	for i, chunk := range chunks {
		if chunk == "" {
			return parts, false
		}
		for _, r := range chunk {
			if r < '0' || r > '9' {
				return parts, false
			}
			parts[i] = parts[i]*10 + int(r-'0')
		}
	}
	return parts, true
}

func containsControl(value string) bool {
	for _, r := range value {
		if r < 32 || r == 127 {
			return true
		}
	}
	return false
}
