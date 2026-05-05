package site

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"postizer/internal/appearance"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

const (
	DefaultTimeZone      = "Asia/Hong_Kong"
	inputMinuteLayout    = "2006-01-02T15:04"
	displayMinuteLayout  = "2006-01-02 15:04"
	legacyDateOnlyLayout = "2006-01-02"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

type Store struct {
	Posts          []*Post
	AllPosts       []*Post
	Pages          []*Page
	Tags           []*Tag
	Settings       Settings
	PostsBySlug    map[string]*Post
	AllPostsBySlug map[string]*Post
	PagesBySlug    map[string]*Page
	TagsBySlug     map[string]*Tag
}

type Settings struct {
	HomeImage       HomeImage            `json:"home_image"`
	MediaProcessing MediaProcessing      `json:"media_processing"`
	TimeZone        string               `json:"time_zone"`
	ThemePack       appearance.Selection `json:"theme_pack"`
	ThemeLocale     string               `json:"theme_locale"`
	PluginOrder     []string             `json:"plugin_order"`

	// Theme / TextPack 仅用于兼容读取旧版配置，保存时会被清空。
	TextPack appearance.Selection `json:"text_pack,omitempty"`
	Theme    string               `json:"theme,omitempty"`
}

type HomeImage struct {
	Enabled bool   `json:"enabled"`
	Src     string `json:"src"`
	Alt     string `json:"alt"`
}

type MediaProcessing struct {
	AutoWebP     bool `json:"auto_webp"`
	WebPQuality  int  `json:"webp_quality"`
	KeepOriginal bool `json:"keep_original"`
}

type Post struct {
	Title       string
	Slug        string
	Date        time.Time
	Updated     time.Time
	Tags        []string
	Summary     string
	Draft       bool
	TOC         bool
	HTML        template.HTML
	Source      string
	FilePath    string
	ReadingTime int
}

type Page struct {
	Title    string
	Slug     string
	Date     time.Time
	Updated  time.Time
	Summary  string
	TOC      bool
	HTML     template.HTML
	Source   string
	FilePath string
}

type Tag struct {
	Name    string
	Title   string
	Slug    string
	Summary string
	Posts   []*Post
}

type frontMatter map[string]string

type PostDraft struct {
	Title         string   `json:"title"`
	Slug          string   `json:"slug"`
	Date          string   `json:"date"`
	Updated       string   `json:"updated"`
	UpdatedManual bool     `json:"updated_manual"`
	Tags          []string `json:"tags"`
	Summary       string   `json:"summary"`
	Draft         bool     `json:"draft"`
	TOC           bool     `json:"toc"`
	Body          string   `json:"body"`
}

type PageDraft struct {
	Title         string `json:"title"`
	Slug          string `json:"slug"`
	Date          string `json:"date"`
	Updated       string `json:"updated"`
	UpdatedManual bool   `json:"updated_manual"`
	Summary       string `json:"summary"`
	TOC           bool   `json:"toc"`
	Body          string `json:"body"`
}

func Load(root string) (*Store, error) {
	md := newMarkdown()
	settings, err := LoadSettings(root)
	if err != nil {
		return nil, err
	}
	location := TimeLocation(settings)

	store := &Store{
		Settings:       settings,
		PostsBySlug:    map[string]*Post{},
		AllPostsBySlug: map[string]*Post{},
		PagesBySlug:    map[string]*Page{},
		TagsBySlug:     map[string]*Tag{},
	}

	if err := loadPosts(filepath.Join(root, "posts"), md, store, location); err != nil {
		return nil, err
	}
	if err := loadPages(filepath.Join(root, "pages"), md, store, location); err != nil {
		return nil, err
	}
	if err := loadTagMetadata(filepath.Join(root, "tags"), store); err != nil {
		return nil, err
	}

	sort.Slice(store.Posts, func(i, j int) bool {
		return store.Posts[i].Date.After(store.Posts[j].Date)
	})
	sort.Slice(store.AllPosts, func(i, j int) bool {
		return store.AllPosts[i].Date.After(store.AllPosts[j].Date)
	})
	sort.Slice(store.Pages, func(i, j int) bool {
		return store.Pages[i].Title < store.Pages[j].Title
	})
	for _, tag := range store.TagsBySlug {
		sort.Slice(tag.Posts, func(i, j int) bool {
			return tag.Posts[i].Date.After(tag.Posts[j].Date)
		})
		store.Tags = append(store.Tags, tag)
	}
	sort.Slice(store.Tags, func(i, j int) bool {
		if len(store.Tags[i].Posts) == len(store.Tags[j].Posts) {
			return store.Tags[i].Slug < store.Tags[j].Slug
		}
		return len(store.Tags[i].Posts) > len(store.Tags[j].Posts)
	})

	return store, nil
}

func LoadSettings(root string) (Settings, error) {
	path := filepath.Join(root, "settings.json")
	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		settings := defaultSettings()
		return settings, nil
	}
	if err != nil {
		return Settings{}, err
	}
	var settings Settings
	if err := json.Unmarshal(body, &settings); err != nil {
		return Settings{}, err
	}
	normalizeSettings(&settings)
	return settings, nil
}

func SaveSettings(root string, settings Settings) error {
	normalizeSettings(&settings)
	if settings.HomeImage.Src != "" && !strings.HasPrefix(settings.HomeImage.Src, "/media/") && !strings.HasPrefix(settings.HomeImage.Src, "/static/") {
		return fmt.Errorf("home image must use a public /media or /static path")
	}
	if settings.HomeImage.Alt == "" {
		settings.HomeImage.Alt = "Home page image"
	}
	// 避免新格式保存时继续写出旧版遗留字段。
	settings.Theme = ""
	settings.TextPack = appearance.Selection{}
	if err := os.MkdirAll(root, 0755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "settings.json"), body, 0644)
}

// defaultSettings 返回系统启动时应采用的默认设置。
func defaultSettings() Settings {
	return Settings{
		MediaProcessing: defaultMediaProcessing(),
		TimeZone:        DefaultTimeZone,
		ThemePack: appearance.Selection{
			Enabled: false,
			PackID:  appearance.DefaultThemePackID,
		},
		ThemeLocale: "en",
	}
}

func defaultMediaProcessing() MediaProcessing {
	return MediaProcessing{
		AutoWebP:     false,
		WebPQuality:  82,
		KeepOriginal: false,
	}
}

// normalizeSettings 负责兼容旧字段，并补全新的默认值。
//
// 兼容策略：
// 1. 如果旧版 `theme` 字段存在而新版主题包还没写入，则把它迁移到主题包选择。
// 2. 如果旧版 `text_pack` 存在，则迁移成主题语言或插件顺序。
// 3. 无论来源如何，都会补上默认主题包 ID，并清洗插件顺序里的空值和重复值。
func normalizeSettings(settings *Settings) {
	defaults := defaultSettings()

	if strings.TrimSpace(settings.ThemePack.PackID) == "" {
		switch strings.TrimSpace(settings.Theme) {
		case "", "newspaper":
			settings.ThemePack = defaults.ThemePack
		default:
			settings.ThemePack = appearance.Selection{
				Enabled: true,
				PackID:  strings.TrimSpace(settings.Theme),
			}
		}
	}

	settings.ThemePack = normalizePackSelection(settings.ThemePack, appearance.DefaultThemePackID)
	settings.ThemeLocale = normalizeThemeLocaleValue(settings.ThemeLocale)
	settings.PluginOrder = normalizePluginOrder(settings.PluginOrder)
	settings.MediaProcessing = normalizeMediaProcessing(settings.MediaProcessing)
	settings.TimeZone = NormalizeTimeZone(settings.TimeZone)

	// 旧版文字包迁移：
	// - 只有旧配置明确启用了 text_pack，才把它迁移到新外观系统。
	// - 官方英文/中文包迁移成主题语言。
	// - 其他旧版文字包迁移成插件包顺序中的首位。
	legacyTextPackID := strings.TrimSpace(settings.TextPack.PackID)
	if settings.TextPack.Enabled && legacyTextPackID != "" {
		switch legacyTextPackID {
		case appearance.LegacyDefaultTextPackID:
			if strings.TrimSpace(settings.ThemeLocale) == "" || settings.ThemeLocale == defaults.ThemeLocale {
				settings.ThemeLocale = "en"
			}
		case appearance.LegacyChineseTextPackID:
			settings.ThemeLocale = "zh-CN"
		default:
			settings.PluginOrder = prependPluginID(settings.PluginOrder, legacyTextPackID)
		}
	}

	if strings.TrimSpace(settings.ThemeLocale) == "" {
		settings.ThemeLocale = defaults.ThemeLocale
	}
}

func normalizeMediaProcessing(settings MediaProcessing) MediaProcessing {
	defaults := defaultMediaProcessing()
	if settings.WebPQuality == 0 {
		settings.WebPQuality = defaults.WebPQuality
	}
	if settings.WebPQuality < 1 {
		settings.WebPQuality = 1
	}
	if settings.WebPQuality > 100 {
		settings.WebPQuality = 100
	}
	return settings
}

func NormalizeTimeZone(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return DefaultTimeZone
	}
	if _, err := time.LoadLocation(value); err != nil {
		return DefaultTimeZone
	}
	return value
}

func ValidTimeZone(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	_, err := time.LoadLocation(value)
	return err == nil
}

func TimeLocation(settings Settings) *time.Location {
	location, err := time.LoadLocation(NormalizeTimeZone(settings.TimeZone))
	if err != nil {
		return time.FixedZone(DefaultTimeZone, 8*60*60)
	}
	return location
}

// normalizePackSelection 把资源包选择归一成当前设置页使用的新语义：
// 1. 选择默认包 => 视为恢复默认，因此 Enabled=false
// 2. 选择非默认包 => 视为启用该资源包，因此 Enabled=true
func normalizePackSelection(selection appearance.Selection, defaultID string) appearance.Selection {
	if strings.TrimSpace(selection.PackID) == "" {
		selection.PackID = defaultID
	}
	selection.Enabled = strings.TrimSpace(selection.PackID) != defaultID
	return selection
}

func normalizeThemeLocaleValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	switch strings.ToLower(value) {
	case "zh-cn", "zh_cn":
		return "zh-CN"
	default:
		return value
	}
}

func normalizePluginOrder(values []string) []string {
	var normalized []string
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		normalized = append(normalized, value)
		seen[value] = true
	}
	return normalized
}

func prependPluginID(values []string, id string) []string {
	id = strings.TrimSpace(id)
	if id == "" {
		return normalizePluginOrder(values)
	}
	reordered := []string{id}
	reordered = append(reordered, values...)
	return normalizePluginOrder(reordered)
}

func loadPosts(dir string, md goldmark.Markdown, store *Store, location *time.Location) error {
	return walkMarkdown(dir, func(path string, body []byte) error {
		fm, markdown := splitFrontMatter(body)
		slug := fm.get("slug", slugFromPath(path))
		if !slugPattern.MatchString(slug) {
			return fmt.Errorf("invalid post slug %q in %s", slug, path)
		}
		htmlBody, err := renderMarkdown(md, markdown)
		if err != nil {
			return err
		}
		post := &Post{
			Title:       fm.get("title", slug),
			Slug:        slug,
			Date:        parseDate(fm.get("date", ""), location),
			Updated:     parseDate(fm.get("updated", ""), location),
			Tags:        parseList(fm.get("tags", "")),
			Summary:     fm.get("summary", ""),
			Draft:       fm.get("draft", "false") == "true",
			TOC:         fm.get("toc", "true") != "false",
			HTML:        htmlBody,
			Source:      string(markdown),
			FilePath:    path,
			ReadingTime: readingTime(markdown),
		}
		store.AllPosts = append(store.AllPosts, post)
		store.AllPostsBySlug[post.Slug] = post
		if post.Draft {
			return nil
		}
		store.Posts = append(store.Posts, post)
		store.PostsBySlug[post.Slug] = post
		for _, tagName := range post.Tags {
			tagSlug := normalizeSlug(tagName)
			tag := store.TagsBySlug[tagSlug]
			if tag == nil {
				tag = &Tag{Name: tagName, Title: tagName, Slug: tagSlug}
				store.TagsBySlug[tagSlug] = tag
			}
			tag.Posts = append(tag.Posts, post)
		}
		return nil
	})
}

func loadPages(dir string, md goldmark.Markdown, store *Store, location *time.Location) error {
	return walkMarkdown(dir, func(path string, body []byte) error {
		fm, markdown := splitFrontMatter(body)
		slug := fm.get("slug", slugFromPath(path))
		if !slugPattern.MatchString(slug) {
			return fmt.Errorf("invalid page slug %q in %s", slug, path)
		}
		htmlBody, err := renderMarkdown(md, markdown)
		if err != nil {
			return err
		}
		updated := parseDate(fm.get("updated", ""), location)
		date := parseDate(fm.get("date", ""), location)
		if date.IsZero() {
			date = updated
		}
		page := &Page{
			Title:    fm.get("title", slug),
			Slug:     slug,
			Date:     date,
			Updated:  updated,
			Summary:  fm.get("summary", ""),
			TOC:      fm.get("toc", "false") == "true",
			HTML:     htmlBody,
			Source:   string(markdown),
			FilePath: path,
		}
		store.Pages = append(store.Pages, page)
		store.PagesBySlug[page.Slug] = page
		return nil
	})
}

func loadTagMetadata(dir string, store *Store) error {
	return walkMarkdown(dir, func(path string, body []byte) error {
		fm, _ := splitFrontMatter(body)
		slug := fm.get("slug", slugFromPath(path))
		if !slugPattern.MatchString(slug) {
			return fmt.Errorf("invalid tag slug %q in %s", slug, path)
		}
		tag := store.TagsBySlug[slug]
		if tag == nil {
			tag = &Tag{Slug: slug}
			store.TagsBySlug[slug] = tag
		}
		tag.Name = fm.get("name", slug)
		tag.Title = fm.get("title", tag.Name)
		tag.Summary = fm.get("summary", "")
		return nil
	})
}

func walkMarkdown(dir string, fn func(string, []byte) error) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".md" {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return fn(path, body)
	})
}

func renderMarkdown(md goldmark.Markdown, source []byte) (template.HTML, error) {
	var out bytes.Buffer
	preparedFigures, figureHTML := prepareFigureReferences(source)
	prepared, equationAnchors := prepareEquationReferences(preparedFigures)
	protected := protectMathDelimiters(prepared)
	if err := md.Convert(protected, &out); err != nil {
		return "", err
	}
	html := out.String()
	for token, value := range mathTokens {
		html = strings.ReplaceAll(html, token, value)
	}
	for token, value := range figureHTML {
		html = strings.ReplaceAll(html, "<p>"+token+"</p>", value)
		html = strings.ReplaceAll(html, token, value)
	}
	for token, value := range equationAnchors {
		html = strings.ReplaceAll(html, "<p>"+token+"</p>", value)
		html = strings.ReplaceAll(html, token, value)
	}
	return template.HTML(html), nil
}

type figureReference struct {
	Number string
	Anchor string
}

type equationReference struct {
	Number string
	Anchor string
}

var (
	equationLabelPattern = regexp.MustCompile(`\\label\{([^}]+)\}`)
	equationTagPattern   = regexp.MustCompile(`\\tag\*?\{([^}]+)\}`)
	equationEqrefPattern = regexp.MustCompile(`\\eqref\{([^}]+)\}`)
	equationRefPattern   = regexp.MustCompile(`\\ref\{([^}]+)\}`)
	figureCaptionPattern = regexp.MustCompile(`(?s)\\caption\{([^}]*)\}`)
	figureImagePattern   = regexp.MustCompile(`(?m)!\[([^\]]*)\]\(([^)]+)\)`)
	figureLinePattern    = regexp.MustCompile(`^!\[([^\]]*)\]\(([^)]+)\)$`)
	figureRefPattern     = regexp.MustCompile(`\\figref\{([^}]+)\}`)
)

func prepareFigureReferences(source []byte) ([]byte, map[string]string) {
	text := strings.ReplaceAll(string(source), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	prepared, references, figures := collectFigureReferences(text)
	if len(references) == 0 {
		return []byte(prepared), figures
	}
	return []byte(replaceFigureReferences(prepared, references)), figures
}

func collectFigureReferences(text string) (string, map[string]figureReference, map[string]string) {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	references := map[string]figureReference{}
	figures := map[string]string{}
	figureNumber := 0
	inFence := false
	fenceMarker := ""
	inEquation := false
	equationEnd := ""

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		if marker := markdownFenceMarker(trimmed); marker != "" {
			if inFence {
				if closesMarkdownFence(trimmed, fenceMarker) {
					inFence = false
					fenceMarker = ""
				}
			} else {
				inFence = true
				fenceMarker = marker
			}
			out = append(out, line)
			continue
		}
		if inFence {
			out = append(out, line)
			continue
		}
		if inEquation {
			out = append(out, line)
			if trimmed == equationEnd {
				inEquation = false
				equationEnd = ""
			}
			continue
		}
		if _, endDelimiter, ok := equationBlockDelimiters(trimmed); ok {
			inEquation = true
			equationEnd = endDelimiter
			out = append(out, line)
			continue
		}

		if trimmed == `\begin{figure}` {
			end := findLine(lines, i+1, `\end{figure}`)
			if end == -1 {
				out = append(out, line)
				continue
			}
			figure, ok := parseFigureBody(strings.Join(lines[i+1:end], "\n"))
			if !ok {
				out = append(out, lines[i:end+1]...)
				i = end
				continue
			}
			figureNumber++
			appendFigureToken(&out, references, figures, figure, figureNumber)
			i = end
			continue
		}

		if figure, ok := parseStandaloneFigureLine(trimmed); ok {
			figureNumber++
			appendFigureToken(&out, references, figures, figure, figureNumber)
			continue
		}

		out = append(out, line)
	}

	return strings.Join(out, "\n"), references, figures
}

type articleFigure struct {
	Alt     string
	Src     string
	Caption string
	Label   string
}

func parseFigureBody(body string) (articleFigure, bool) {
	imageMatch := figureImagePattern.FindStringSubmatch(body)
	if len(imageMatch) == 0 {
		return articleFigure{}, false
	}
	figure := articleFigure{
		Alt: strings.TrimSpace(imageMatch[1]),
		Src: markdownImageDestination(imageMatch[2]),
	}
	captionMatch := figureCaptionPattern.FindStringSubmatch(body)
	if len(captionMatch) > 0 {
		figure.Caption = strings.TrimSpace(captionMatch[1])
	}
	if figure.Caption == "" {
		figure.Caption = figure.Alt
	}
	labelMatch := equationLabelPattern.FindStringSubmatch(body)
	if len(labelMatch) > 0 {
		figure.Label = strings.TrimSpace(labelMatch[1])
	}
	return figure, figure.Src != ""
}

func parseStandaloneFigureLine(line string) (articleFigure, bool) {
	match := figureLinePattern.FindStringSubmatch(line)
	if len(match) == 0 {
		return articleFigure{}, false
	}
	alt := strings.TrimSpace(match[1])
	src := markdownImageDestination(match[2])
	if src == "" {
		return articleFigure{}, false
	}
	return articleFigure{Alt: alt, Src: src, Caption: alt}, true
}

func markdownImageDestination(raw string) string {
	value := strings.TrimSpace(raw)
	if before, _, found := strings.Cut(value, " "); found {
		value = before
	}
	return strings.Trim(value, "<>")
}

func appendFigureToken(out *[]string, references map[string]figureReference, figures map[string]string, figure articleFigure, number int) {
	if len(*out) > 0 && strings.TrimSpace((*out)[len(*out)-1]) != "" {
		*out = append(*out, "")
	}
	anchor := ""
	if figure.Label != "" {
		anchor = figureAnchorID(figure.Label)
		if _, exists := references[figure.Label]; !exists {
			references[figure.Label] = figureReference{Number: fmt.Sprintf("%d", number), Anchor: anchor}
		} else {
			anchor = fmt.Sprintf("%s-%d", anchor, len(figures)+1)
		}
	}
	token := fmt.Sprintf("POSTIZER_FIGURE_%d", len(figures))
	figures[token] = renderFigureHTML(figure, number, anchor)
	*out = append(*out, token, "")
}

func renderFigureHTML(figure articleFigure, number int, anchor string) string {
	var b strings.Builder
	id := ""
	if anchor != "" {
		id = fmt.Sprintf(` id="%s"`, template.HTMLEscapeString(anchor))
	}
	fmt.Fprintf(&b, `<figure%s class="article-figure">`, id)
	fmt.Fprintf(
		&b,
		`<img src="%s" alt="%s" loading="lazy">`,
		template.HTMLEscapeString(figure.Src),
		template.HTMLEscapeString(figure.Alt),
	)
	b.WriteString(`<figcaption><span class="figure-number">Figure `)
	b.WriteString(fmt.Sprintf("%d", number))
	b.WriteString(`.</span>`)
	if strings.TrimSpace(figure.Caption) != "" {
		b.WriteByte(' ')
		b.WriteString(template.HTMLEscapeString(figure.Caption))
	}
	b.WriteString(`</figcaption></figure>`)
	return b.String()
}

func replaceFigureReferences(text string, references map[string]figureReference) string {
	lines := strings.Split(text, "\n")
	inFence := false
	fenceMarker := ""
	inEquation := false
	equationEnd := ""

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if marker := markdownFenceMarker(trimmed); marker != "" {
			if inFence {
				if closesMarkdownFence(trimmed, fenceMarker) {
					inFence = false
					fenceMarker = ""
				}
			} else {
				inFence = true
				fenceMarker = marker
			}
			continue
		}
		if inFence {
			continue
		}
		if inEquation {
			if trimmed == equationEnd {
				inEquation = false
				equationEnd = ""
			}
			continue
		}
		if _, endDelimiter, ok := equationBlockDelimiters(trimmed); ok {
			inEquation = true
			equationEnd = endDelimiter
			continue
		}

		line = figureRefPattern.ReplaceAllStringFunc(line, func(raw string) string {
			return figureReferenceMarkdown(raw, figureRefPattern, references, true)
		})
		line = equationRefPattern.ReplaceAllStringFunc(line, func(raw string) string {
			return figureReferenceMarkdown(raw, equationRefPattern, references, false)
		})
		lines[i] = line
	}

	return strings.Join(lines, "\n")
}

func figureReferenceMarkdown(raw string, pattern *regexp.Regexp, references map[string]figureReference, named bool) string {
	match := pattern.FindStringSubmatch(raw)
	if len(match) == 0 {
		return raw
	}
	label := strings.TrimSpace(match[1])
	reference, ok := references[label]
	if !ok {
		if named || strings.HasPrefix(label, "fig:") {
			if named {
				return "Figure ??"
			}
			return "??"
		}
		return raw
	}
	text := reference.Number
	if named {
		text = "Figure " + text
	}
	return fmt.Sprintf("[%s](#%s)", escapeMarkdownLinkText(text), reference.Anchor)
}

func prepareEquationReferences(source []byte) ([]byte, map[string]string) {
	text := strings.ReplaceAll(string(source), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	prepared, references, anchors := collectEquationReferences(text)
	if len(references) == 0 {
		return []byte(prepared), anchors
	}
	return []byte(replaceEquationReferences(prepared, references)), anchors
}

func collectEquationReferences(text string) (string, map[string]equationReference, map[string]string) {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	references := map[string]equationReference{}
	anchors := map[string]string{}
	equationNumber := 0
	inFence := false
	fenceMarker := ""

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		if marker := markdownFenceMarker(trimmed); marker != "" {
			if inFence {
				if closesMarkdownFence(trimmed, fenceMarker) {
					inFence = false
					fenceMarker = ""
				}
			} else {
				inFence = true
				fenceMarker = marker
			}
			out = append(out, line)
			continue
		}
		if inFence {
			out = append(out, line)
			continue
		}

		startDelimiter, endDelimiter, ok := equationBlockDelimiters(trimmed)
		if !ok {
			out = append(out, line)
			continue
		}

		end := findEquationBlockEnd(lines, i+1, endDelimiter)
		if end == -1 {
			out = append(out, line)
			continue
		}

		body := strings.Join(lines[i+1:end], "\n")
		labelMatch := equationLabelPattern.FindStringSubmatch(body)
		label := ""
		if len(labelMatch) > 0 {
			label = strings.TrimSpace(labelMatch[1])
			body = equationLabelPattern.ReplaceAllString(body, "")
		}
		tagMatch := equationTagPattern.FindStringSubmatch(body)
		number := ""
		if len(tagMatch) > 0 {
			number = strings.TrimSpace(tagMatch[1])
		} else {
			equationNumber++
			number = fmt.Sprintf("%d", equationNumber)
			body = strings.TrimRight(body, "\n")
			if strings.TrimSpace(body) != "" {
				body += "\n"
			}
			body += `\tag{` + number + `}`
		}

		if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
			out = append(out, "")
		}
		if label != "" {
			anchor := equationAnchorID(label)
			if _, exists := references[label]; !exists {
				references[label] = equationReference{Number: number, Anchor: anchor}
			} else {
				anchor = fmt.Sprintf("%s-%d", anchor, len(anchors)+1)
			}
			token := fmt.Sprintf("POSTIZER_EQ_ANCHOR_%d", len(anchors))
			anchors[token] = fmt.Sprintf(`<span id="%s" class="equation-anchor"></span>`, anchor)
			out = append(out, token, "")
		}
		out = append(out, startDelimiter)
		if body != "" {
			out = append(out, strings.Split(body, "\n")...)
		}
		out = append(out, endDelimiter)
		i = end
	}

	return strings.Join(out, "\n"), references, anchors
}

func replaceEquationReferences(text string, references map[string]equationReference) string {
	lines := strings.Split(text, "\n")
	inFence := false
	fenceMarker := ""
	inEquation := false
	equationEnd := ""

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if marker := markdownFenceMarker(trimmed); marker != "" {
			if inFence {
				if closesMarkdownFence(trimmed, fenceMarker) {
					inFence = false
					fenceMarker = ""
				}
			} else {
				inFence = true
				fenceMarker = marker
			}
			continue
		}
		if inFence {
			continue
		}
		if inEquation {
			if trimmed == equationEnd {
				inEquation = false
				equationEnd = ""
			}
			continue
		}
		if _, endDelimiter, ok := equationBlockDelimiters(trimmed); ok {
			inEquation = true
			equationEnd = endDelimiter
			continue
		}

		line = equationEqrefPattern.ReplaceAllStringFunc(line, func(raw string) string {
			return equationReferenceMarkdown(raw, equationEqrefPattern, references, true)
		})
		line = equationRefPattern.ReplaceAllStringFunc(line, func(raw string) string {
			return equationReferenceMarkdown(raw, equationRefPattern, references, false)
		})
		lines[i] = line
	}

	return strings.Join(lines, "\n")
}

func equationReferenceMarkdown(raw string, pattern *regexp.Regexp, references map[string]equationReference, parenthesized bool) string {
	match := pattern.FindStringSubmatch(raw)
	if len(match) == 0 {
		return raw
	}
	reference, ok := references[strings.TrimSpace(match[1])]
	if !ok {
		if parenthesized {
			return "(??)"
		}
		return "??"
	}
	text := reference.Number
	if parenthesized {
		text = "(" + text + ")"
	}
	return fmt.Sprintf("[%s](#%s)", escapeMarkdownLinkText(text), reference.Anchor)
}

func escapeMarkdownLinkText(text string) string {
	text = strings.ReplaceAll(text, `\`, `\\`)
	text = strings.ReplaceAll(text, `[`, `\[`)
	text = strings.ReplaceAll(text, `]`, `\]`)
	return text
}

func equationBlockDelimiters(trimmed string) (string, string, bool) {
	switch trimmed {
	case "$$":
		return "$$", "$$", true
	case `\[`:
		return `\[`, `\]`, true
	default:
		return "", "", false
	}
}

func findEquationBlockEnd(lines []string, start int, endDelimiter string) int {
	for i := start; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == endDelimiter {
			return i
		}
	}
	return -1
}

func findLine(lines []string, start int, target string) int {
	for i := start; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == target {
			return i
		}
	}
	return -1
}

func equationAnchorID(label string) string {
	return prefixedAnchorID("eq-", label)
}

func figureAnchorID(label string) string {
	return prefixedAnchorID("fig-", label)
}

func prefixedAnchorID(prefix, label string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(label)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	id := strings.Trim(b.String(), "-")
	if id == "" {
		id = strings.TrimSuffix(prefix, "-")
	}
	return prefix + id
}

func markdownFenceMarker(trimmed string) string {
	if len(trimmed) < 3 {
		return ""
	}
	ch := trimmed[0]
	if ch != '`' && ch != '~' {
		return ""
	}
	count := 0
	for count < len(trimmed) && trimmed[count] == ch {
		count++
	}
	if count < 3 {
		return ""
	}
	return strings.Repeat(string(ch), count)
}

func closesMarkdownFence(trimmed, marker string) bool {
	if marker == "" || len(trimmed) < len(marker) {
		return false
	}
	for i := range marker {
		if trimmed[i] != marker[i] {
			return false
		}
	}
	return true
}

var mathTokens = map[string]string{
	"POSTIZER_MATH_INLINE_OPEN":  `\(`,
	"POSTIZER_MATH_INLINE_CLOSE": `\)`,
	"POSTIZER_MATH_BLOCK_OPEN":   `\[`,
	"POSTIZER_MATH_BLOCK_CLOSE":  `\]`,
}

func protectMathDelimiters(source []byte) []byte {
	text := string(source)
	text = strings.ReplaceAll(text, `\(`, "POSTIZER_MATH_INLINE_OPEN")
	text = strings.ReplaceAll(text, `\)`, "POSTIZER_MATH_INLINE_CLOSE")
	text = strings.ReplaceAll(text, `\[`, "POSTIZER_MATH_BLOCK_OPEN")
	text = strings.ReplaceAll(text, `\]`, "POSTIZER_MATH_BLOCK_CLOSE")
	return []byte(text)
}

func RenderMarkdown(source string) (template.HTML, error) {
	return renderMarkdown(newMarkdown(), []byte(source))
}

func SavePost(root string, draft PostDraft) (PostDraft, error) {
	draft.Slug = NormalizeSlug(draft.Slug)
	if draft.Slug == "" {
		draft.Slug = NormalizeSlug(draft.Title)
	}
	if !ValidSlug(draft.Slug) {
		return PostDraft{}, fmt.Errorf("invalid post slug %q", draft.Slug)
	}
	if strings.TrimSpace(draft.Title) == "" {
		return PostDraft{}, fmt.Errorf("title is required")
	}

	settings, err := LoadSettings(root)
	if err != nil {
		return PostDraft{}, err
	}
	location := TimeLocation(settings)
	now := time.Now().In(location).Truncate(time.Minute)

	if draft.Date == "" {
		draft.Date = FormatInputDateTime(now)
	}
	date, err := parseContentTime(draft.Date, location)
	if err != nil {
		return PostDraft{}, fmt.Errorf("invalid date %q", draft.Date)
	}
	draft.Date = FormatInputDateTime(date)

	if !draft.UpdatedManual || strings.TrimSpace(draft.Updated) == "" {
		draft.Updated = FormatInputDateTime(now)
	} else {
		updated, err := parseContentTime(draft.Updated, location)
		if err != nil {
			return PostDraft{}, fmt.Errorf("invalid updated date %q", draft.Updated)
		}
		draft.Updated = FormatInputDateTime(updated)
	}

	if err := writeContentFile(root, "posts", draft.Slug, serializePost(draft)); err != nil {
		return PostDraft{}, err
	}
	return draft, nil
}

func SavePage(root string, draft PageDraft) (PageDraft, error) {
	draft.Slug = NormalizeSlug(draft.Slug)
	if draft.Slug == "" {
		draft.Slug = NormalizeSlug(draft.Title)
	}
	if !ValidSlug(draft.Slug) {
		return PageDraft{}, fmt.Errorf("invalid page slug %q", draft.Slug)
	}
	if strings.TrimSpace(draft.Title) == "" {
		return PageDraft{}, fmt.Errorf("title is required")
	}

	settings, err := LoadSettings(root)
	if err != nil {
		return PageDraft{}, err
	}
	location := TimeLocation(settings)
	now := time.Now().In(location).Truncate(time.Minute)

	if draft.Date == "" {
		draft.Date = FormatInputDateTime(now)
	}
	date, err := parseContentTime(draft.Date, location)
	if err != nil {
		return PageDraft{}, fmt.Errorf("invalid date %q", draft.Date)
	}
	draft.Date = FormatInputDateTime(date)

	if !draft.UpdatedManual || strings.TrimSpace(draft.Updated) == "" {
		draft.Updated = FormatInputDateTime(now)
	} else {
		updated, err := parseContentTime(draft.Updated, location)
		if err != nil {
			return PageDraft{}, fmt.Errorf("invalid updated date %q", draft.Updated)
		}
		draft.Updated = FormatInputDateTime(updated)
	}

	if err := writeContentFile(root, "pages", draft.Slug, serializePage(draft)); err != nil {
		return PageDraft{}, err
	}
	return draft, nil
}

func writeContentFile(root, section, slug, content string) error {
	contentDir := filepath.Join(root, section)
	if err := os.MkdirAll(contentDir, 0755); err != nil {
		return err
	}
	target := filepath.Join(contentDir, slug+".md")
	cleanContentDir, err := filepath.Abs(contentDir)
	if err != nil {
		return err
	}
	cleanTarget, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(cleanTarget, cleanContentDir+string(os.PathSeparator)) {
		return fmt.Errorf("%s path escapes content directory", strings.TrimSuffix(section, "s"))
	}
	return os.WriteFile(cleanTarget, []byte(content), 0644)
}

func serializePost(draft PostDraft) string {
	var b strings.Builder
	b.WriteString("---\n")
	writeFrontMatter(&b, "title", draft.Title)
	writeFrontMatter(&b, "slug", draft.Slug)
	writeFrontMatter(&b, "date", draft.Date)
	if draft.Updated != "" {
		writeFrontMatter(&b, "updated", draft.Updated)
	}
	b.WriteString("tags: [")
	for i, tag := range draft.Tags {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(strconvQuote(strings.TrimSpace(tag)))
	}
	b.WriteString("]\n")
	writeFrontMatter(&b, "summary", draft.Summary)
	fmt.Fprintf(&b, "draft: %t\n", draft.Draft)
	fmt.Fprintf(&b, "toc: %t\n", draft.TOC)
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(draft.Body))
	b.WriteString("\n")
	return b.String()
}

func serializePage(draft PageDraft) string {
	var b strings.Builder
	b.WriteString("---\n")
	writeFrontMatter(&b, "title", draft.Title)
	writeFrontMatter(&b, "slug", draft.Slug)
	writeFrontMatter(&b, "date", draft.Date)
	if draft.Updated != "" {
		writeFrontMatter(&b, "updated", draft.Updated)
	}
	writeFrontMatter(&b, "summary", draft.Summary)
	fmt.Fprintf(&b, "toc: %t\n", draft.TOC)
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(draft.Body))
	b.WriteString("\n")
	return b.String()
}

func writeFrontMatter(b *strings.Builder, key, value string) {
	fmt.Fprintf(b, "%s: %s\n", key, strconvQuote(value))
}

func strconvQuote(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

func newMarkdown() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(extension.GFM, extension.Footnote, extension.Typographer),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(html.WithXHTML()),
	)
}

func splitFrontMatter(body []byte) (frontMatter, []byte) {
	text := strings.ReplaceAll(string(body), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return frontMatter{}, body
	}
	rest := strings.TrimPrefix(text, "---\n")
	parts := strings.SplitN(rest, "\n---\n", 2)
	if len(parts) != 2 {
		return frontMatter{}, body
	}
	fm := frontMatter{}
	for _, line := range strings.Split(parts[0], "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if ok {
			fm[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"`)
		}
	}
	markdown := strings.TrimPrefix(parts[1], "\n")
	return fm, []byte(markdown)
}

func (fm frontMatter) get(key, fallback string) string {
	if value := strings.TrimSpace(fm[key]); value != "" {
		return value
	}
	return fallback
}

func parseDate(value string, location *time.Location) time.Time {
	if value == "" {
		return time.Time{}
	}
	t, err := parseContentTime(value, location)
	if err != nil {
		return time.Time{}
	}
	return t
}

func parseContentTime(value string, location *time.Location) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("empty time")
	}
	if location == nil {
		location = time.UTC
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, value); err == nil {
			return t.In(location).Truncate(time.Minute), nil
		}
	}
	for _, layout := range []string{inputMinuteLayout, displayMinuteLayout, legacyDateOnlyLayout} {
		if t, err := time.ParseInLocation(layout, value, location); err == nil {
			return t.Truncate(time.Minute), nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported time %q", value)
}

func FormatInputDateTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(inputMinuteLayout)
}

func FormatDisplayTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(displayMinuteLayout)
}

func parseList(value string) []string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "[")
	value = strings.TrimSuffix(value, "]")
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.Trim(strings.TrimSpace(part), `"`)
		if item != "" {
			items = append(items, item)
		}
	}
	return items
}

func slugFromPath(path string) string {
	return normalizeSlug(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
}

func NormalizeSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "-")
	value = regexp.MustCompile(`[^a-z0-9-]+`).ReplaceAllString(value, "")
	value = regexp.MustCompile(`-+`).ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}

func ValidSlug(value string) bool {
	return slugPattern.MatchString(value)
}

func normalizeSlug(value string) string {
	return NormalizeSlug(value)
}

func readingTime(body []byte) int {
	words := len(strings.Fields(string(body)))
	minutes := words / 220
	if words%220 != 0 || minutes == 0 {
		minutes++
	}
	return minutes
}
