package main

import (
	"fmt"
	"io"
	"net/url"
	"strings"
)

func isWordPressUploadURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	return strings.Contains(parsed.Path, "/wp-content/uploads/")
}

func urlPath(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		return value
	}
	return parsed.Path
}

func mediaPlaceholderID(id string) string {
	return "postizer://media/" + strings.TrimSpace(id)
}

func replaceMediaPlaceholders(body string, replacements map[string]string) string {
	for placeholder, path := range replacements {
		body = strings.ReplaceAll(body, placeholder, path)
	}
	return body
}

func readLimited(reader io.Reader, maxBytes int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("file exceeds %d bytes", maxBytes)
	}
	return body, nil
}

func fallback(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func escapeLinkText(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `[`, `\[`)
	return strings.ReplaceAll(value, `]`, `\]`)
}

func escapeAlt(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `]`, `\]`)
}

func escapeCaption(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `{`, "")
	return strings.ReplaceAll(value, `}`, "")
}
