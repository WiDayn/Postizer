package main

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"postizer/pkg/pluginrpc"
)

type importBuilder struct {
	media        []mediaFetch
	urlToMedia   map[string]string
	mediaByID    map[string]bool
	extraCounter int
}

type importBatch struct {
	Posts   []pluginrpc.ContentDraft
	Pages   []pluginrpc.ContentDraft
	Media   []mediaFetch
	Skipped []string
}

type mediaFetch struct {
	ID           string
	SourceURL    string
	OriginalName string
	Alt          string
	Caption      string
}

func buildImportBatch(export *wxr) (*importBatch, error) {
	builder := &importBuilder{
		urlToMedia: map[string]string{},
		mediaByID:  map[string]bool{},
	}
	for _, item := range export.Channel.Items {
		if item.PostType != "attachment" || strings.TrimSpace(item.AttachmentURL) == "" {
			continue
		}
		builder.addMedia(item.AttachmentURL, item.Title, item.Excerpt, item.PostID)
	}

	var posts []pluginrpc.ContentDraft
	var pages []pluginrpc.ContentDraft
	var skipped []string
	for _, item := range export.Channel.Items {
		switch item.PostType {
		case "post", "page":
			if item.Status == "trash" {
				skipped = append(skipped, fmt.Sprintf("%s: %s", item.PostType, fallback(item.Title, item.PostID)))
				continue
			}
			body := builder.markdown(item.Content)
			draft := pluginrpc.ContentDraft{
				Title:   fallback(item.Title, "Untitled"),
				Slug:    importSlug(item.PostName, item.Title, item.PostID),
				Date:    importTime(item.PostDate),
				Updated: importTime(item.PostModified),
				Tags:    itemTags(item),
				Draft:   item.Status != "publish",
				TOC:     item.PostType == "post",
				Body:    body,
			}
			if item.PostType == "post" {
				posts = append(posts, draft)
			} else {
				pages = append(pages, draft)
			}
		case "attachment", "nav_menu_item", "wp_template", "wp_template_part", "wp_navigation", "wp_global_styles", "custom_css":
			continue
		default:
			skipped = append(skipped, fmt.Sprintf("%s: %s", fallback(item.PostType, "unknown"), fallback(item.Title, item.PostID)))
		}
	}

	return &importBatch{
		Posts:   posts,
		Pages:   pages,
		Media:   builder.media,
		Skipped: skipped,
	}, nil
}

func (b *importBuilder) addMedia(sourceURL, title, caption, postID string) string {
	sourceURL = strings.TrimSpace(sourceURL)
	if sourceURL == "" {
		return ""
	}
	if placeholder := b.urlToMedia[sourceURL]; placeholder != "" {
		return placeholder
	}
	id := "wxr-" + strings.TrimSpace(postID)
	if id == "wxr-" {
		b.extraCounter++
		id = fmt.Sprintf("wxr-extra-%d", b.extraCounter)
	}
	for b.mediaByID[id] {
		b.extraCounter++
		id = fmt.Sprintf("%s-%d", id, b.extraCounter)
	}
	b.mediaByID[id] = true
	placeholder := mediaPlaceholderID(id)
	originalName := path.Base(urlPath(sourceURL))
	if originalName == "." || originalName == "/" || originalName == "" {
		originalName = id + ".bin"
	}
	b.media = append(b.media, mediaFetch{
		ID:           id,
		SourceURL:    sourceURL,
		OriginalName: originalName,
		Alt:          fallback(title, strings.TrimSuffix(originalName, path.Ext(originalName))),
		Caption:      strings.TrimSpace(stripHTML(caption)),
	})
	b.urlToMedia[sourceURL] = placeholder
	return placeholder
}

func (b *importBuilder) mediaPlaceholder(sourceURL, alt string) string {
	if placeholder := b.urlToMedia[sourceURL]; placeholder != "" {
		return placeholder
	}
	return b.addMedia(sourceURL, alt, "", "")
}

func itemTags(item wxrItem) []string {
	seen := map[string]bool{}
	var tags []string
	for _, category := range item.Categories {
		if category.Domain != "category" && category.Domain != "post_tag" {
			continue
		}
		value := strings.TrimSpace(category.Text)
		if value == "" || strings.EqualFold(value, "uncategorized") || seen[value] {
			continue
		}
		seen[value] = true
		tags = append(tags, value)
	}
	sort.Strings(tags)
	return tags
}

func importTime(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "0000-00-00") {
		return ""
	}
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC1123Z, time.RFC1123} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.Format("2006-01-02T15:04")
		}
	}
	return value
}

func importSlug(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if decoded, err := url.PathUnescape(value); err == nil {
			value = decoded
		}
		if slug := normalizeSlug(value); slug != "" {
			return slug
		}
	}
	return "imported"
}

func normalizeSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = regexp.MustCompile(`[^\p{L}\p{N}]+`).ReplaceAllString(value, "-")
	value = regexp.MustCompile(`-+`).ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}
