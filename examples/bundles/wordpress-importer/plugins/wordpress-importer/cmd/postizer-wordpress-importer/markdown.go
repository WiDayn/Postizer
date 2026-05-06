package main

import (
	"fmt"
	"html"
	"path"
	"regexp"
	"strconv"
	"strings"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func (b *importBuilder) markdown(raw string) string {
	raw = stripWPComments(raw)
	nodes, err := parseHTMLFragment(raw)
	if err != nil {
		return strings.TrimSpace(stripHTML(raw))
	}
	var parts []string
	for _, node := range nodes {
		rendered := strings.TrimSpace(b.block(node, 0))
		if rendered != "" {
			parts = append(parts, rendered)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func (b *importBuilder) block(node *xhtml.Node, depth int) string {
	if node.Type == xhtml.TextNode {
		return strings.TrimSpace(node.Data)
	}
	if node.Type == xhtml.CommentNode {
		if isMoreComment(node.Data) {
			return "<!--more-->"
		}
		return ""
	}
	if node.Type != xhtml.ElementNode {
		return b.children(node, depth)
	}
	switch node.Data {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		level, _ := strconv.Atoi(strings.TrimPrefix(node.Data, "h"))
		return strings.Repeat("#", level) + " " + strings.TrimSpace(b.inlineChildren(node))
	case "p", "div", "section", "article":
		return strings.TrimSpace(b.inlineChildren(node))
	case "blockquote":
		text := strings.TrimSpace(b.children(node, depth))
		lines := strings.Split(text, "\n")
		for index, line := range lines {
			lines[index] = "> " + line
		}
		return strings.Join(lines, "\n")
	case "ul":
		return b.list(node, false, depth)
	case "ol":
		return b.list(node, true, depth)
	case "pre":
		return "```\n" + strings.TrimSpace(nodeTextWithBreaks(node)) + "\n```"
	case "hr":
		return "---"
	case "math":
		return strings.TrimSpace(mathTeX(node))
	case "figure":
		if rendered := b.figure(node); rendered != "" {
			return rendered
		}
	case "img":
		return b.image(node, "")
	case "video", "object":
		return b.mediaLink(node, "")
	case "br":
		return "\n"
	}
	return b.children(node, depth)
}

func (b *importBuilder) children(node *xhtml.Node, depth int) string {
	var parts []string
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if rendered := strings.TrimSpace(b.block(child, depth)); rendered != "" {
			parts = append(parts, rendered)
		}
	}
	return strings.Join(parts, "\n\n")
}

func (b *importBuilder) inlineChildren(node *xhtml.Node) string {
	var out strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		out.WriteString(b.inline(child))
	}
	return collapseInlineSpace(out.String())
}

func (b *importBuilder) inline(node *xhtml.Node) string {
	if node.Type == xhtml.TextNode {
		return node.Data
	}
	if node.Type != xhtml.ElementNode {
		return b.inlineChildren(node)
	}
	switch node.Data {
	case "strong", "b":
		return "**" + strings.TrimSpace(b.inlineChildren(node)) + "**"
	case "em", "i":
		return "*" + strings.TrimSpace(b.inlineChildren(node)) + "*"
	case "code":
		return "`" + strings.TrimSpace(nodeTextWithBreaks(node)) + "`"
	case "a":
		href := attr(node, "href")
		text := strings.TrimSpace(b.inlineChildren(node))
		if href == "" {
			return text
		}
		return "[" + escapeLinkText(fallback(text, href)) + "](" + b.linkDestination(href, text) + ")"
	case "img":
		return "\n\n" + b.image(node, "") + "\n\n"
	case "video", "object":
		return "\n\n" + b.mediaLink(node, "") + "\n\n"
	case "math":
		if tex := mathTeX(node); tex != "" {
			return "$" + tex + "$"
		}
		return strings.TrimSpace(nodeText(node))
	case "br":
		return "\n"
	}
	return b.inlineChildren(node)
}

func (b *importBuilder) list(node *xhtml.Node, ordered bool, depth int) string {
	var lines []string
	index := 1
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != xhtml.ElementNode || child.Data != "li" {
			continue
		}
		prefix := "- "
		if ordered {
			prefix = fmt.Sprintf("%d. ", index)
			index++
		}
		text := strings.TrimSpace(b.inlineChildren(child))
		if text == "" {
			text = strings.TrimSpace(b.children(child, depth+1))
		}
		lines = append(lines, strings.Repeat("  ", depth)+prefix+text)
	}
	return strings.Join(lines, "\n")
}

func (b *importBuilder) figure(node *xhtml.Node) string {
	img := firstElement(node, "img")
	caption := ""
	if figcaption := firstElement(node, "figcaption"); figcaption != nil {
		caption = strings.TrimSpace(nodeText(figcaption))
	}
	if img == nil {
		for _, name := range []string{"video", "object"} {
			if media := firstElement(node, name); media != nil {
				return b.mediaLink(media, caption)
			}
		}
		return ""
	}
	return b.image(img, caption)
}

func (b *importBuilder) image(node *xhtml.Node, caption string) string {
	src := attr(node, "src")
	if src == "" {
		return ""
	}
	alt := attr(node, "alt")
	if alt == "" {
		alt = "Image"
	}
	placeholder := b.mediaPlaceholder(src, alt)
	labelBase := importSlug(strings.TrimSuffix(path.Base(urlPath(src)), path.Ext(urlPath(src))), "image", "")
	if labelBase == "" {
		labelBase = "image"
	}
	if caption == "" {
		caption = alt
	}
	return fmt.Sprintf("\\begin{figure}\n![%s](%s)\n\\caption{%s}\n\\label{fig:%s}\n\\end{figure}",
		escapeAlt(alt), placeholder, escapeCaption(caption), labelBase)
}

func (b *importBuilder) mediaLink(node *xhtml.Node, caption string) string {
	source := fallback(attr(node, "src"), attr(node, "data"), attr(node, "href"))
	if source == "" {
		return ""
	}
	label := fallback(caption, attr(node, "aria-label"), path.Base(urlPath(source)), source)
	return "[" + escapeLinkText(label) + "](" + b.linkDestination(source, label) + ")"
}

func (b *importBuilder) linkDestination(href, label string) string {
	if placeholder := b.urlToMedia[href]; placeholder != "" {
		return placeholder
	}
	if isWordPressUploadURL(href) {
		return b.mediaPlaceholder(href, label)
	}
	return href
}

func firstElement(node *xhtml.Node, name string) *xhtml.Node {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == xhtml.ElementNode && child.Data == name {
			return child
		}
		if nested := firstElement(child, name); nested != nil {
			return nested
		}
	}
	return nil
}

func attr(node *xhtml.Node, name string) string {
	for _, attr := range node.Attr {
		if attr.Key == name {
			return strings.TrimSpace(attr.Val)
		}
	}
	return ""
}

func nodeText(node *xhtml.Node) string {
	if node.Type == xhtml.TextNode {
		return node.Data
	}
	var out strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		out.WriteString(nodeText(child))
	}
	return html.UnescapeString(out.String())
}

func stripWPComments(value string) string {
	re := regexp.MustCompile(`(?is)<!--\s*/?wp:[^>]*-->`)
	return re.ReplaceAllString(value, "")
}

func isMoreComment(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "more")
}

func stripHTML(value string) string {
	value = stripWPComments(value)
	nodes, err := parseHTMLFragment(value)
	if err != nil {
		return html.UnescapeString(value)
	}
	var out strings.Builder
	for _, node := range nodes {
		out.WriteString(nodeText(node))
		out.WriteByte(' ')
	}
	return collapseSpace(out.String())
}

func parseHTMLFragment(value string) ([]*xhtml.Node, error) {
	return xhtml.ParseFragment(strings.NewReader(value), &xhtml.Node{
		Type:     xhtml.ElementNode,
		Data:     "body",
		DataAtom: atom.Body,
	})
}

func nodeTextWithBreaks(node *xhtml.Node) string {
	if node.Type == xhtml.TextNode {
		return node.Data
	}
	if node.Type == xhtml.ElementNode && node.Data == "br" {
		return "\n"
	}
	var out strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		out.WriteString(nodeTextWithBreaks(child))
	}
	return html.UnescapeString(out.String())
}

func mathTeX(node *xhtml.Node) string {
	if node.Type == xhtml.ElementNode && node.Data == "annotation" && strings.EqualFold(attr(node, "encoding"), "application/x-tex") {
		return strings.TrimSpace(nodeText(node))
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if tex := mathTeX(child); tex != "" {
			return tex
		}
	}
	return ""
}

func collapseSpace(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func collapseInlineSpace(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	lines := strings.Split(value, "\n")
	for index, line := range lines {
		lines[index] = collapseSpace(line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
