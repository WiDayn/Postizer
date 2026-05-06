package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"postizer/pkg/pluginrpc"
)

type wxr struct {
	Channel wxrChannel `xml:"channel"`
}

type wxrChannel struct {
	Title       string    `xml:"title"`
	BaseBlogURL string    `xml:"http://wordpress.org/export/1.2/ base_blog_url"`
	Items       []wxrItem `xml:"item"`
}

type wxrItem struct {
	Title         string        `xml:"title"`
	Link          string        `xml:"link"`
	GUID          string        `xml:"guid"`
	Content       string        `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
	Excerpt       string        `xml:"http://wordpress.org/export/1.2/excerpt/ encoded"`
	PostID        string        `xml:"http://wordpress.org/export/1.2/ post_id"`
	PostDate      string        `xml:"http://wordpress.org/export/1.2/ post_date"`
	PostModified  string        `xml:"http://wordpress.org/export/1.2/ post_modified"`
	PostName      string        `xml:"http://wordpress.org/export/1.2/ post_name"`
	Status        string        `xml:"http://wordpress.org/export/1.2/ status"`
	PostType      string        `xml:"http://wordpress.org/export/1.2/ post_type"`
	AttachmentURL string        `xml:"http://wordpress.org/export/1.2/ attachment_url"`
	Categories    []wxrCategory `xml:"category"`
}

type wxrCategory struct {
	Domain   string `xml:"domain,attr"`
	Nicename string `xml:"nicename,attr"`
	Text     string `xml:",chardata"`
}

func wxrFile(req *pluginrpc.InvokeActionRequest) (pluginrpc.ActionFile, error) {
	for _, file := range req.Files {
		if file.Name == "wxr_file" || strings.HasSuffix(strings.ToLower(file.Filename), ".xml") {
			if len(file.Body) == 0 {
				return pluginrpc.ActionFile{}, fmt.Errorf("uploaded XML file is empty")
			}
			return file, nil
		}
	}
	return pluginrpc.ActionFile{}, fmt.Errorf("missing WordPress XML file")
}

func parseWXR(body []byte) (*wxr, error) {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	decoder.Strict = false
	var export wxr
	if err := decoder.Decode(&export); err != nil {
		return nil, err
	}
	return &export, nil
}

func inspect(export *wxr) *pluginrpc.InvokeActionResponse {
	typeCounts := map[string]int{}
	statusCounts := map[string]int{}
	mediaExts := map[string]int{}
	for _, item := range export.Channel.Items {
		postType := strings.TrimSpace(item.PostType)
		if postType == "" {
			postType = "unknown"
		}
		typeCounts[postType]++
		if postType == "post" || postType == "page" {
			statusCounts[strings.TrimSpace(item.Status)]++
		}
		if postType == "attachment" {
			ext := strings.ToLower(path.Ext(urlPath(item.AttachmentURL)))
			if ext == "" {
				ext = "unknown"
			}
			mediaExts[ext]++
		}
	}

	return &pluginrpc.InvokeActionResponse{
		Title:   "WordPress export inspected",
		Summary: fmt.Sprintf("%s contains %d exported items.", fallback(export.Channel.Title, "WordPress export"), len(export.Channel.Items)),
		Sections: []pluginrpc.ResultSection{
			{Title: "Content types", Rows: mapRows(typeCounts)},
			{Title: "Post/page statuses", Rows: mapRows(statusCounts)},
			{Title: "Media extensions", Rows: mapRows(mediaExts)},
		},
		NextActions: []pluginrpc.NextAction{
			{
				ID:      "import_wxr",
				Label:   "Import Now",
				Style:   "primary",
				Confirm: "Import this WordPress export into Postizer?",
				Fields: map[string]string{
					"upload_token": "__HOST_UPLOAD_TOKEN__",
				},
			},
		},
	}
}

func mapRows(values map[string]int) []pluginrpc.ResultRow {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	rows := make([]pluginrpc.ResultRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, pluginrpc.ResultRow{Label: fallback(key, "unknown"), Value: strconv.Itoa(values[key])})
	}
	return rows
}
