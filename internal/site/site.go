package site

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	stdhtml "html"
	"html/template"
	"io/fs"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"postizer/internal/appearance"

	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

const (
	DefaultTimeZone      = "Asia/Hong_Kong"
	DefaultSiteTitle     = "Postizer"
	DefaultPostPermalink = "/posts/%postname%"
	DefaultPagePermalink = "/pages/%pagename%"
	DefaultTagPermalink  = "/tags/%tag%"
	inputMinuteLayout    = "2006-01-02T15:04"
	displayMinuteLayout  = "2006-01-02 15:04"
	legacyDateOnlyLayout = "2006-01-02"
)

var (
	slugPattern             = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N}-]*$`)
	moreTagPattern          = regexp.MustCompile(`(?is)<!--\s*more\s*-->`)
	summaryFencePattern     = regexp.MustCompile("(?s)```.*?```")
	summaryHTMLComment      = regexp.MustCompile(`(?s)<!--.*?-->`)
	summaryImagePattern     = regexp.MustCompile(`!\[([^\]]*)\]\([^)]+\)`)
	summaryLinkPattern      = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
	summaryHeadingPattern   = regexp.MustCompile(`(?m)^\s{0,3}#{1,6}\s+`)
	summaryQuotePattern     = regexp.MustCompile(`(?m)^\s*>\s?`)
	summaryListPattern      = regexp.MustCompile(`(?m)^\s*(?:[-*+]\s+|\d+[.]\s+)`)
	summaryHTMLTagPattern   = regexp.MustCompile(`(?s)<[^>]+>`)
	summaryEmphasisPattern  = regexp.MustCompile(`[*_~]{1,3}`)
	summaryLineBreakPattern = regexp.MustCompile(`(?m)\s{2,}$`)
	themeSettingKeyPattern  = regexp.MustCompile(`[^a-z0-9_]+`)
	themeSettingKeyDivider  = regexp.MustCompile(`_+`)
	permalinkTokenPattern   = regexp.MustCompile(`%([A-Za-z0-9_]+)%`)
)

const (
	ThemeSettingTypeString  = "string"
	ThemeSettingTypeInteger = "integer"
	ThemeSettingTypeFloat   = "float"

	defaultThemeMenuID = "default-menu"

	currentSettingsVersion            = "v0.1.5"
	legacyPureWhiteHeroTitlePrefix    = "pure-white-hero-title-"
	legacyPureWhiteHeroSubtitlePrefix = "pure-white-hero-subtitle-"
)

type Store struct {
	Posts          []*Post
	AllPosts       []*Post
	Pages          []*Page
	AllPages       []*Page
	Tags           []*Tag
	Settings       Settings
	PostsBySlug    map[string]*Post
	AllPostsBySlug map[string]*Post
	PagesBySlug    map[string]*Page
	AllPagesBySlug map[string]*Page
	TagsBySlug     map[string]*Tag
}

type Settings struct {
	Version         string               `json:"version,omitempty"`
	SiteTitle       SiteTitle            `json:"site_title"`
	Permalinks      PermalinkSettings    `json:"permalinks"`
	AutoUpdate      AutoUpdateSettings   `json:"auto_update"`
	Comments        CommentSettings      `json:"comments"`
	HomePage        HomePageSettings     `json:"home_page"`
	HomeImage       HomeImage            `json:"home_image"`
	MediaProcessing MediaProcessing      `json:"media_processing"`
	ThemeSettings   ThemeSettings        `json:"theme_settings"`
	TimeZone        string               `json:"time_zone"`
	ThemePack       appearance.Selection `json:"theme_pack"`
	ThemeLocale     string               `json:"theme_locale"`
	PluginOrder     []string             `json:"plugin_order"`

	// Theme / TextPack 仅用于兼容读取旧版配置，保存时会被清空。
	TextPack appearance.Selection `json:"text_pack,omitempty"`
	Theme    string               `json:"theme,omitempty"`
}

type SiteTitle struct {
	Main     string `json:"main"`
	Subtitle string `json:"subtitle"`
}

type PermalinkSettings struct {
	Post string `json:"post"`
	Page string `json:"page"`
	Tag  string `json:"tag"`
}

type AutoUpdateSettings struct {
	Enabled bool `json:"enabled"`
}

type HomePageSettings struct {
	PageSize int `json:"page_size"`
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

type ThemeSettings struct {
	Menus         []Menu              `json:"menus"`
	MenuLocations map[string]string   `json:"menu_locations"`
	Sidebars      []Menu              `json:"sidebars"`
	Sidebar       *string             `json:"sidebar,omitempty"`
	Custom        ThemeCustomSettings `json:"custom,omitempty"`
}

// ThemeCustomSettings 保存主题专属的轻量配置。
//
// 数据结构按主题 ID 分组，第二层 key 是主题自己的设置名。值只允许 JSON 原生
// 字符串、整数和浮点数，既方便手动查看 settings.json，也避免主题设置混入
// 任意复杂对象导致后续迁移困难。
type ThemeCustomSettings map[string]map[string]ThemeSettingValue

// ThemeSettingValue 表示一个主题自定义设置值。
//
// Type 用于在 Go 侧保留数字类型差异；JSON 保存时仍直接写出原生值：
// - string  保存为 "text"
// - integer 保存为 12
// - float   保存为 12.5
type ThemeSettingValue struct {
	Type    string  `json:"-"`
	String  string  `json:"-"`
	Integer int64   `json:"-"`
	Float   float64 `json:"-"`
}

// StringThemeSetting 创建字符串类型的主题设置值。
//
// @param value 字符串设置内容。
// @returns 返回可保存到 ThemeCustomSettings 的字符串设置值。
func StringThemeSetting(value string) ThemeSettingValue {
	return ThemeSettingValue{Type: ThemeSettingTypeString, String: value}
}

// IntegerThemeSetting 创建整数类型的主题设置值。
//
// @param value 整数设置内容。
// @returns 返回可保存到 ThemeCustomSettings 的整数设置值。
func IntegerThemeSetting(value int64) ThemeSettingValue {
	return ThemeSettingValue{Type: ThemeSettingTypeInteger, Integer: value}
}

// FloatThemeSetting 创建浮点类型的主题设置值。
//
// @param value 浮点设置内容。
// @returns 返回可保存到 ThemeCustomSettings 的浮点设置值。
func FloatThemeSetting(value float64) ThemeSettingValue {
	return ThemeSettingValue{Type: ThemeSettingTypeFloat, Float: value}
}

// UnmarshalJSON 从 JSON 原生值解析主题设置。
//
// 设计说明：
//   - 字符串直接进入 string 类型。
//   - 不带小数点/指数的数字进入 integer 类型。
//   - 带小数点或科学计数法的数字进入 float 类型。
//   - 其他 JSON 类型不报错，但会在 normalizeThemeSettings 阶段被丢弃，
//     这样手动编辑 settings.json 出错时不会让整个站点加载失败。
func (value *ThemeSettingValue) UnmarshalJSON(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var raw any
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	switch typed := raw.(type) {
	case string:
		*value = StringThemeSetting(typed)
	case json.Number:
		number := typed.String()
		if strings.ContainsAny(number, ".eE") {
			parsed, err := strconv.ParseFloat(number, 64)
			if err == nil {
				*value = FloatThemeSetting(parsed)
			}
			return nil
		}
		parsed, err := strconv.ParseInt(number, 10, 64)
		if err == nil {
			*value = IntegerThemeSetting(parsed)
			return nil
		}
		if parsedFloat, floatErr := strconv.ParseFloat(number, 64); floatErr == nil {
			*value = FloatThemeSetting(parsedFloat)
		}
	default:
		*value = ThemeSettingValue{}
	}
	return nil
}

// MarshalJSON 把主题设置写回 JSON 原生值。
//
// @returns 返回字符串、整数或浮点数的 JSON 表示。
func (value ThemeSettingValue) MarshalJSON() ([]byte, error) {
	switch value.Type {
	case ThemeSettingTypeString:
		return json.Marshal(value.String)
	case ThemeSettingTypeInteger:
		return json.Marshal(value.Integer)
	case ThemeSettingTypeFloat:
		return json.Marshal(value.Float)
	default:
		return []byte("null"), nil
	}
}

// StringValue 返回字符串设置值。
//
// @returns 当设置类型为 string 时返回保存的字符串，否则返回空字符串。
func (value ThemeSettingValue) StringValue() string {
	if value.Type != ThemeSettingTypeString {
		return ""
	}
	return value.String
}

// IntegerValue 返回整数设置值。
//
// @returns 当设置类型为 integer 时返回保存的整数，否则返回 0。
func (value ThemeSettingValue) IntegerValue() int64 {
	if value.Type != ThemeSettingTypeInteger {
		return 0
	}
	return value.Integer
}

// FloatValue 返回浮点设置值。
//
// @returns float 设置直接返回浮点值；integer 设置会转换为浮点值；其他类型返回 0。
func (value ThemeSettingValue) FloatValue() float64 {
	switch value.Type {
	case ThemeSettingTypeFloat:
		return value.Float
	case ThemeSettingTypeInteger:
		return float64(value.Integer)
	default:
		return 0
	}
}

type Menu struct {
	ID    string     `json:"id"`
	Name  string     `json:"name"`
	Items []MenuItem `json:"items"`
}

type MenuItem struct {
	Type   string     `json:"-"`
	Label  string     `json:"-"`
	Target string     `json:"-"`
	URL    string     `json:"-"`
	Items  []MenuItem `json:"-"`
}

// MenuItemMain 保存菜单项真正指向的资源信息。
//
// 设计说明：
// - label 留在 MenuItem 顶层，表示后台结构里显示的名称。
// - main 聚合 type/target/url，避免 settings.json 里菜单项字段继续扁平扩散。
// - Go 内部仍保留 MenuItem.Type/MenuItem.Target/MenuItem.URL，减少调用点改动。
type MenuItemMain struct {
	Type   string `json:"type"`
	Target string `json:"target,omitempty"`
	URL    string `json:"url,omitempty"`
}

// MarshalJSON 把菜单项写成新的 settings.json 结构。
//
// @returns 返回形如 {"label":"内容","main":{"type":"page","target":"about"}} 的 JSON。
func (item MenuItem) MarshalJSON() ([]byte, error) {
	payload := struct {
		Label string       `json:"label"`
		Main  MenuItemMain `json:"main"`
		Items []MenuItem   `json:"items,omitempty"`
	}{
		Label: item.Label,
		Main: MenuItemMain{
			Type:   item.Type,
			Target: item.Target,
			URL:    item.URL,
		},
	}
	if len(item.Items) > 0 {
		payload.Items = item.Items
	}
	return json.Marshal(payload)
}

// UnmarshalJSON 读取新旧两种菜单项结构。
//
// 兼容策略：
// - 新结构优先读取 main.type/main.target/main.url。
// - 如果没有 main，则读取旧版顶层 type/target/url。
//
// @param body settings.json 中单个菜单项的 JSON 内容。
// @returns 解析失败时返回 JSON 错误。
func (item *MenuItem) UnmarshalJSON(body []byte) error {
	var raw struct {
		Label  string        `json:"label"`
		Main   *MenuItemMain `json:"main"`
		Type   string        `json:"type"`
		Target string        `json:"target"`
		URL    string        `json:"url"`
		Items  []MenuItem    `json:"items"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return err
	}
	main := MenuItemMain{
		Type:   raw.Type,
		Target: raw.Target,
		URL:    raw.URL,
	}
	if raw.Main != nil {
		main = *raw.Main
	}
	*item = MenuItem{
		Type:   main.Type,
		Label:  raw.Label,
		Target: main.Target,
		URL:    main.URL,
		Items:  raw.Items,
	}
	return nil
}

type MenuLink struct {
	Label string
	URL   string
	Count int
}

type SidebarLinkSection struct {
	Type  string
	Title string
	Links []MenuLink
}

const (
	SidebarSectionTypeNone        = ""
	SidebarSectionTypeCustom      = "custom"
	SidebarSectionTypeTopics      = "topics"
	SidebarSectionTypePages       = "pages"
	SidebarSectionTypeFeeds       = "feeds"
	SidebarSectionTypeRecentPosts = "recent-posts"
)

const recentSidebarPostLimit = 5

// SidebarLabels 保存侧边栏自动区块的默认标题和链接标签。
//
// Store 本身不持有当前主题语言；模板渲染时会传入本地化文案，纯 Store 测试
// 则使用这里的中文默认值。
type SidebarLabels struct {
	TopicsTitle      string
	PagesTitle       string
	FeedsTitle       string
	RecentPostsTitle string
	CustomTitle      string
	RSSLabel         string
	SitemapLabel     string
}

type Post struct {
	Title       string
	Slug        string
	URL         string
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
	URL      string
	Date     time.Time
	Updated  time.Time
	Summary  string
	Draft    bool
	TOC      bool
	HTML     template.HTML
	Source   string
	FilePath string
}

type Tag struct {
	Name    string
	Title   string
	Slug    string
	URL     string
	Summary string
	Posts   []*Post
}

type frontMatter map[string]string

type PostDraft struct {
	Title         string   `json:"title"`
	Slug          string   `json:"slug"`
	OriginalSlug  string   `json:"original_slug,omitempty"`
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
	OriginalSlug  string `json:"original_slug,omitempty"`
	Date          string `json:"date"`
	Updated       string `json:"updated"`
	UpdatedManual bool   `json:"updated_manual"`
	Summary       string `json:"summary"`
	Draft         bool   `json:"draft"`
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
		AllPagesBySlug: map[string]*Page{},
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
	sort.Slice(store.AllPages, func(i, j int) bool {
		return store.AllPages[i].Title < store.AllPages[j].Title
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

	store.applyPermalinks()

	return store, nil
}

func (s *Store) applyPermalinks() {
	if s == nil {
		return
	}
	for _, post := range s.AllPosts {
		post.URL = PostURL(s.Settings, post)
	}
	for _, page := range s.AllPages {
		page.URL = PageURL(s.Settings, page)
	}
	for _, tag := range s.TagsBySlug {
		tag.URL = TagURL(s.Settings, tag)
	}
}

func (s *Store) PostByPermalink(requestPath string) *Post {
	if s == nil {
		return nil
	}
	for _, post := range s.Posts {
		if samePermalinkPath(post.URL, requestPath) {
			return post
		}
	}
	return nil
}

func (s *Store) PageByPermalink(requestPath string) *Page {
	if s == nil {
		return nil
	}
	for _, page := range s.Pages {
		if samePermalinkPath(page.URL, requestPath) {
			return page
		}
	}
	return nil
}

func (s *Store) TagByPermalink(requestPath string) *Tag {
	if s == nil {
		return nil
	}
	for _, tag := range s.Tags {
		if samePermalinkPath(tag.URL, requestPath) {
			return tag
		}
	}
	return nil
}

func (s *Store) MenuLinks(location string) []MenuLink {
	if s == nil {
		return nil
	}
	location = normalizeMenuID(location)
	if location == "" {
		return nil
	}
	menuID := normalizeMenuID(s.Settings.ThemeSettings.MenuLocations[location])
	if menuID == "" {
		return nil
	}
	var menu *Menu
	for i := range s.Settings.ThemeSettings.Menus {
		if s.Settings.ThemeSettings.Menus[i].ID == menuID {
			menu = &s.Settings.ThemeSettings.Menus[i]
			break
		}
	}
	if menu == nil {
		return nil
	}
	links := make([]MenuLink, 0, len(menu.Items))
	for _, item := range menu.Items {
		if link, ok := s.resolveMenuItem(item); ok {
			links = append(links, link)
		}
	}
	return links
}

func (s *Store) MenuLocationAssigned(location string) bool {
	if s == nil {
		return false
	}
	location = normalizeMenuID(location)
	if location == "" {
		return false
	}
	_, ok := s.Settings.ThemeSettings.MenuLocations[location]
	return ok
}

// SidebarSections 使用默认文案解析前台右侧栏区块。
//
// @returns 返回可渲染的侧边栏区块列表。
func (s *Store) SidebarSections() []SidebarLinkSection {
	return s.SidebarSectionsWithLabels(DefaultSidebarLabels())
}

// DefaultSidebarLabels 返回侧边栏自动区块默认文案。
//
// @returns 返回中文默认文案；模板渲染时通常会用当前语言覆盖。
func DefaultSidebarLabels() SidebarLabels {
	return SidebarLabels{
		TopicsTitle:      "标签",
		PagesTitle:       "页面",
		FeedsTitle:       "订阅",
		RecentPostsTitle: "最近文章",
		CustomTitle:      "自定义区域",
		RSSLabel:         "RSS",
		SitemapLabel:     "站点地图",
	}
}

// SidebarSectionsWithLabels 解析前台右侧栏区块。
//
// 设计说明：
//   - settings.theme_settings.sidebars 仍然表示“侧边栏列表”，和自定义菜单列表一致。
//   - 每个侧边栏的 items 才表示这个侧边栏里的区块列表，例如标签、页面、订阅、
//     最近文章和自定义区域。
//   - theme_settings.sidebar 和菜单投放一样控制实际使用哪个侧边栏：nil 表示默认
//     侧边栏，空字符串表示不使用侧边栏，其他值表示自定义侧边栏 ID。
//   - 旧数据的 items 是直接链接而不是区块；这里只会把旧链接包装成一个自定义区域，
//     不额外插入默认区块。
//
// @param labels 当前语言的侧边栏文案。
// @returns 返回过滤失效链接后的可渲染区块。
func (s *Store) SidebarSectionsWithLabels(labels SidebarLabels) []SidebarLinkSection {
	if s == nil {
		return nil
	}
	labels = labels.withDefaults()
	sidebars := s.selectedSidebarMenus(labels)
	if len(sidebars) == 0 {
		return nil
	}
	sections := make([]SidebarLinkSection, 0, len(sidebars)*4)
	for _, sidebar := range sidebars {
		items := sidebar.Items
		if legacySidebarItems(items) {
			items = []MenuItem{{
				Type:  SidebarSectionTypeCustom,
				Label: strings.TrimSpace(sidebar.Name),
				Items: normalizeLegacySidebarMenuItems(items),
			}}
		} else {
			items = normalizeSidebarCollectionItems(items)
		}
		for _, item := range items {
			if section, ok := s.resolveSidebarSectionItem(item, labels); ok {
				sections = append(sections, section)
			}
		}
	}
	return sections
}

// selectedSidebarMenus 返回当前主题设置实际投放的侧边栏。
//
// @param labels 当前语言的默认标题文案。
// @returns 返回一个待解析的侧边栏；显式禁用或无效选择时返回空切片。
func (s *Store) selectedSidebarMenus(labels SidebarLabels) []Menu {
	selected := s.Settings.ThemeSettings.Sidebar
	if selected == nil {
		return []Menu{defaultSidebarMenu(labels)}
	}
	sidebarID := normalizeMenuID(*selected)
	if sidebarID == "" {
		return nil
	}
	for _, sidebar := range s.Settings.ThemeSettings.Sidebars {
		if sidebar.ID == sidebarID {
			return []Menu{sidebar}
		}
	}
	return nil
}

// withDefaults 补齐调用方没有提供的侧边栏文案。
//
// @returns 返回所有字段都有值的文案集合。
func (labels SidebarLabels) withDefaults() SidebarLabels {
	defaults := DefaultSidebarLabels()
	if strings.TrimSpace(labels.TopicsTitle) == "" {
		labels.TopicsTitle = defaults.TopicsTitle
	}
	if strings.TrimSpace(labels.PagesTitle) == "" {
		labels.PagesTitle = defaults.PagesTitle
	}
	if strings.TrimSpace(labels.FeedsTitle) == "" {
		labels.FeedsTitle = defaults.FeedsTitle
	}
	if strings.TrimSpace(labels.RecentPostsTitle) == "" {
		labels.RecentPostsTitle = defaults.RecentPostsTitle
	}
	if strings.TrimSpace(labels.CustomTitle) == "" {
		labels.CustomTitle = defaults.CustomTitle
	}
	if strings.TrimSpace(labels.RSSLabel) == "" {
		labels.RSSLabel = defaults.RSSLabel
	}
	if strings.TrimSpace(labels.SitemapLabel) == "" {
		labels.SitemapLabel = defaults.SitemapLabel
	}
	return labels
}

// defaultSidebarMenu 返回没有任何侧边栏配置时的默认侧边栏。
//
// @param labels 当前语言的默认标题文案。
// @returns 返回一个包含默认区块的侧边栏。
func defaultSidebarMenu(labels SidebarLabels) Menu {
	return Menu{
		ID:    "default-sidebar",
		Name:  labels.CustomTitle,
		Items: defaultSidebarItems(labels),
	}
}

// defaultSidebarItems 返回旧模板原本固定显示的侧边栏区块。
//
// @param labels 当前语言的默认标题文案。
// @returns 返回话题、页面、订阅三个默认区块。
func defaultSidebarItems(labels SidebarLabels) []MenuItem {
	return []MenuItem{
		{Type: SidebarSectionTypeTopics, Label: labels.TopicsTitle},
		{Type: SidebarSectionTypePages, Label: labels.PagesTitle},
		{Type: SidebarSectionTypeFeeds, Label: labels.FeedsTitle},
	}
}

// legacySidebarItems 判断侧边栏 items 是否仍是旧版直接链接列表。
//
// @param items 侧边栏下保存的项目。
// @returns 如果项目整体仍像旧版直接链接列表则返回 true。
func legacySidebarItems(items []MenuItem) bool {
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		if !legacySidebarDirectMenuItem(item) {
			return false
		}
	}
	return true
}

// legacySidebarDirectMenuItem 判断项目是否是旧版侧边栏里的直接菜单链接。
//
// @param item 侧边栏中的一个项目。
// @returns 如果该项目可按旧版菜单链接迁移则返回 true。
func legacySidebarDirectMenuItem(item MenuItem) bool {
	if legacySidebarDirectCustomLink(item) {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(item.Type)) {
	case "page", "post", "tag", "tagindex", "url", "home", "archive", "tags", "search", "admin":
		return true
	default:
		return false
	}
}

// legacySidebarDirectCustomLink 判断项目是否是旧版侧边栏里的直接自定义链接。
//
// 设计说明：
// 旧版侧边栏曾经和菜单一样直接保存链接，且自定义链接写作 type="custom"。
// 新版 type="custom" 表示“自定义区块”，所以只有带地址、没有子项的 custom
// 才能判断为旧链接；空的自定义区块或包含子链接的自定义区块仍保留为新版结构。
//
// @param item 侧边栏中的一个项目。
// @returns 如果该项目应按旧版直接链接迁移则返回 true。
func legacySidebarDirectCustomLink(item MenuItem) bool {
	if !strings.EqualFold(strings.TrimSpace(item.Type), SidebarSectionTypeCustom) {
		return false
	}
	if len(item.Items) > 0 {
		return false
	}
	return strings.TrimSpace(item.URL) != "" || strings.TrimSpace(item.Target) != ""
}

// normalizeLegacySidebarMenuItems 把旧版侧边栏直接链接清洗为当前菜单链接结构。
//
// 设计说明：
// 旧版侧边栏 items 保存的是菜单链接；这里先复用菜单旧自定义链接迁移，把
// type="custom" 转成 type="url"，再走菜单链接归一化，确保裸链接、安全链接
// 和失效链接的处理规则与菜单一致。
//
// @param items 旧版侧边栏直接链接列表。
// @returns 返回可以挂到新版自定义区块中的链接列表。
func normalizeLegacySidebarMenuItems(items []MenuItem) []MenuItem {
	return normalizeMenuItems(migrateLegacyCustomMenuItems(items))
}

// normalizeSidebarCollectionItems 清洗单个侧边栏里的项目列表。
//
// 设计说明：
// 新版侧边栏第一层应全部是区块。为了兼容手动编辑或开发版升级留下的混合结构，
// 这里会把夹在新区块中的旧版直接链接收拢成自定义区块，避免链接地址在
// normalizeSidebarSectionItems 阶段被当作区块字段清空。
//
// @param items settings.json 中单个侧边栏的 items。
// @returns 返回可安全渲染和写回的新结构；纯旧版列表仍返回旧链接列表。
func normalizeSidebarCollectionItems(items []MenuItem) []MenuItem {
	if legacySidebarItems(items) {
		return normalizeLegacySidebarMenuItems(items)
	}
	normalized := make([]MenuItem, 0, len(items))
	legacyLinks := []MenuItem{}
	flushLegacyLinks := func() {
		links := normalizeLegacySidebarMenuItems(legacyLinks)
		if len(links) > 0 {
			normalized = append(normalized, MenuItem{
				Type:  SidebarSectionTypeCustom,
				Items: links,
			})
		}
		legacyLinks = nil
	}
	for _, item := range items {
		if legacySidebarDirectMenuItem(item) {
			legacyLinks = append(legacyLinks, item)
			continue
		}
		flushLegacyLinks()
		normalized = append(normalized, normalizeSidebarSectionItems([]MenuItem{item})...)
	}
	flushLegacyLinks()
	return normalized
}

// resolveSidebarSectionItem 把一个侧边栏区块解析为前台链接区块。
//
// @param item 侧边栏中的一个区块项目。
// @param labels 当前语言的默认文案。
// @returns 返回可渲染区块以及是否应该显示。
func (s *Store) resolveSidebarSectionItem(item MenuItem, labels SidebarLabels) (SidebarLinkSection, bool) {
	sectionType := normalizeSidebarSectionType(item.Type)
	title := strings.TrimSpace(item.Label)
	if title == "" {
		title = sidebarSectionTitle(sectionType, labels)
	}
	section := SidebarLinkSection{Type: sectionType, Title: title}
	switch sectionType {
	case SidebarSectionTypeTopics:
		for _, tag := range s.Tags {
			section.Links = append(section.Links, MenuLink{Label: tag.Title, URL: TagURL(s.Settings, tag), Count: len(tag.Posts)})
		}
	case SidebarSectionTypePages:
		for _, page := range s.Pages {
			section.Links = append(section.Links, MenuLink{Label: page.Title, URL: PageURL(s.Settings, page)})
		}
	case SidebarSectionTypeFeeds:
		section.Links = []MenuLink{
			{Label: labels.RSSLabel, URL: "/feed.xml"},
			{Label: labels.SitemapLabel, URL: "/sitemap.xml"},
		}
	case SidebarSectionTypeRecentPosts:
		for index, post := range s.Posts {
			if index >= recentSidebarPostLimit {
				break
			}
			section.Links = append(section.Links, MenuLink{Label: post.Title, URL: PostURL(s.Settings, post)})
		}
	default:
		for _, child := range item.Items {
			if link, ok := s.resolveMenuItem(child); ok {
				section.Links = append(section.Links, link)
			}
		}
		if title == "" || len(section.Links) == 0 {
			return SidebarLinkSection{}, false
		}
	}
	if title == "" {
		return SidebarLinkSection{}, false
	}
	return section, true
}

// sidebarSectionTitle 返回侧边栏区块的默认标题。
//
// @param sectionType 区块类型。
// @param labels 当前语言的默认文案。
// @returns 返回对应标题。
func sidebarSectionTitle(sectionType string, labels SidebarLabels) string {
	switch normalizeSidebarSectionType(sectionType) {
	case SidebarSectionTypeTopics:
		return labels.TopicsTitle
	case SidebarSectionTypePages:
		return labels.PagesTitle
	case SidebarSectionTypeFeeds:
		return labels.FeedsTitle
	case SidebarSectionTypeRecentPosts:
		return labels.RecentPostsTitle
	default:
		return labels.CustomTitle
	}
}

// ThemeSetting 读取指定主题的自定义设置。
//
// @param themeID 主题包 ID，例如 pure-white。
// @param key 主题设置键名，例如 hero_title。
// @returns 返回设置值以及是否存在。
func (s *Store) ThemeSetting(themeID string, key string) (ThemeSettingValue, bool) {
	if s == nil {
		return ThemeSettingValue{}, false
	}
	themeID = normalizeMenuID(themeID)
	key = normalizeThemeSettingKey(key)
	if themeID == "" || key == "" {
		return ThemeSettingValue{}, false
	}
	values := s.Settings.ThemeSettings.Custom[themeID]
	if values == nil {
		return ThemeSettingValue{}, false
	}
	value, ok := values[key]
	return value, ok
}

// ThemeStringSetting 读取字符串类型的主题设置。
//
// @param themeID 主题包 ID。
// @param key 设置键名。
// @param fallback 未设置或类型不匹配时返回的默认值。
// @returns 返回字符串设置值。
func (s *Store) ThemeStringSetting(themeID string, key string, fallback string) string {
	value, ok := s.ThemeSetting(themeID, key)
	if !ok || value.Type != ThemeSettingTypeString {
		return fallback
	}
	return value.String
}

// ThemeIntegerSetting 读取整数类型的主题设置。
//
// @param themeID 主题包 ID。
// @param key 设置键名。
// @param fallback 未设置或类型不匹配时返回的默认值。
// @returns 返回整数设置值。
func (s *Store) ThemeIntegerSetting(themeID string, key string, fallback int64) int64 {
	value, ok := s.ThemeSetting(themeID, key)
	if !ok || value.Type != ThemeSettingTypeInteger {
		return fallback
	}
	return value.Integer
}

// ThemeFloatSetting 读取浮点类型的主题设置。
//
// @param themeID 主题包 ID。
// @param key 设置键名。
// @param fallback 未设置或类型不匹配时返回的默认值。
// @returns 返回浮点设置值；整数设置会自动转换为浮点数。
func (s *Store) ThemeFloatSetting(themeID string, key string, fallback float64) float64 {
	value, ok := s.ThemeSetting(themeID, key)
	if !ok {
		return fallback
	}
	switch value.Type {
	case ThemeSettingTypeFloat:
		return value.Float
	case ThemeSettingTypeInteger:
		return float64(value.Integer)
	default:
		return fallback
	}
}

// ResolveMenuItem 把后台保存的菜单项解析成前台可渲染链接。
//
// 设计说明：
//   - Store.MenuLinks 会按菜单位置解析整个菜单；管理后台还需要复用同一套解析逻辑，
//     例如渲染未绑定位置时的默认菜单结构。
//   - 无效的内容目标或 URL 会返回 ok=false，由调用方决定是否过滤。
//
// @param item 后台保存的菜单项。
// @returns 第一个返回值是可渲染链接，第二个返回值表示菜单项是否有效。
func (s *Store) ResolveMenuItem(item MenuItem) (MenuLink, bool) {
	return s.resolveMenuItem(item)
}

func (s *Store) resolveMenuItem(item MenuItem) (MenuLink, bool) {
	label := strings.TrimSpace(item.Label)
	switch strings.ToLower(strings.TrimSpace(item.Type)) {
	case "home":
		if label == "" {
			label = "首页"
		}
		return MenuLink{Label: label, URL: "/"}, true
	case "archive":
		if label == "" {
			label = "归档"
		}
		return MenuLink{Label: label, URL: "/archive"}, true
	case "tags":
		if label == "" {
			label = "标签"
		}
		return MenuLink{Label: label, URL: "/tags"}, true
	case "search":
		if label == "" {
			label = "搜索"
		}
		return MenuLink{Label: label, URL: "/search"}, true
	case "admin":
		if label == "" {
			label = "后台"
		}
		return MenuLink{Label: label, URL: "/admin"}, true
	case "page":
		page := s.PagesBySlug[strings.TrimSpace(item.Target)]
		if page == nil {
			return MenuLink{}, false
		}
		if label == "" {
			label = page.Title
		}
		return MenuLink{Label: label, URL: PageURL(s.Settings, page)}, true
	case "post":
		post := s.PostsBySlug[strings.TrimSpace(item.Target)]
		if post == nil {
			return MenuLink{}, false
		}
		if label == "" {
			label = post.Title
		}
		return MenuLink{Label: label, URL: PostURL(s.Settings, post)}, true
	case "tag":
		tag := s.TagsBySlug[strings.TrimSpace(item.Target)]
		if tag == nil {
			return MenuLink{}, false
		}
		if label == "" {
			label = tag.Title
		}
		return MenuLink{Label: label, URL: TagURL(s.Settings, tag)}, true
	case "url":
		url, ok := normalizeMenuURL(item.URL)
		if !ok {
			return MenuLink{}, false
		}
		if label == "" {
			label = url
		}
		return MenuLink{Label: label, URL: url}, true
	default:
		return MenuLink{}, false
	}
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
		Version:         currentSettingsVersion,
		SiteTitle:       defaultSiteTitle(),
		Permalinks:      defaultPermalinks(),
		AutoUpdate:      defaultAutoUpdate(),
		Comments:        defaultCommentSettings(),
		HomePage:        defaultHomePageSettings(),
		MediaProcessing: defaultMediaProcessing(),
		TimeZone:        DefaultTimeZone,
		ThemePack: appearance.Selection{
			Enabled: false,
			PackID:  appearance.DefaultThemePackID,
		},
		ThemeLocale: "en",
	}
}

func defaultPermalinks() PermalinkSettings {
	return PermalinkSettings{
		Post: DefaultPostPermalink,
		Page: DefaultPagePermalink,
		Tag:  DefaultTagPermalink,
	}
}

func defaultSiteTitle() SiteTitle {
	return SiteTitle{
		Main: DefaultSiteTitle,
	}
}

func defaultAutoUpdate() AutoUpdateSettings {
	return AutoUpdateSettings{
		Enabled: false,
	}
}

func defaultHomePageSettings() HomePageSettings {
	return HomePageSettings{
		PageSize: 10,
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
// 1. 先读取旧 settings 版本，用它决定是否执行版本限定的数据迁移。
// 2. 如果旧版 `theme` 字段存在而新版主题包还没写入，则把它迁移到主题包选择。
// 3. 如果旧版 `text_pack` 存在，则迁移成主题语言或插件顺序。
// 4. 无论来源如何，都会补上默认主题包 ID，并清洗插件顺序里的空值和重复值。
func normalizeSettings(settings *Settings) {
	defaults := defaultSettings()
	sourceVersion := strings.TrimSpace(settings.Version)

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
	settings.ThemeSettings = normalizeThemeSettings(settings.ThemeSettings)
	settings.TimeZone = NormalizeTimeZone(settings.TimeZone)
	settings.SiteTitle = normalizeSiteTitle(settings.SiteTitle)
	settings.Permalinks = normalizePermalinks(settings.Permalinks)
	settings.AutoUpdate = normalizeAutoUpdate(settings.AutoUpdate)
	settings.Comments = normalizeCommentSettings(settings.Comments)
	settings.HomePage = normalizeHomePageSettings(settings.HomePage)
	settings.Version = normalizedSettingsVersion(sourceVersion)

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

func normalizeSiteTitle(title SiteTitle) SiteTitle {
	defaults := defaultSiteTitle()
	title.Main = strings.TrimSpace(title.Main)
	title.Subtitle = strings.TrimSpace(title.Subtitle)
	if title.Main == "" {
		title.Main = defaults.Main
	}
	return title
}

func normalizePermalinks(settings PermalinkSettings) PermalinkSettings {
	defaults := defaultPermalinks()
	settings.Post = normalizePermalinkPattern(settings.Post, defaults.Post, postPermalinkAllowedTokens(), []string{"postname", "slug"})
	settings.Page = normalizePermalinkPattern(settings.Page, defaults.Page, pagePermalinkAllowedTokens(), []string{"pagename", "slug"})
	settings.Tag = normalizePermalinkPattern(settings.Tag, defaults.Tag, tagPermalinkAllowedTokens(), []string{"tag", "slug"})
	return settings
}

func normalizeAutoUpdate(settings AutoUpdateSettings) AutoUpdateSettings {
	return AutoUpdateSettings{Enabled: settings.Enabled}
}

func normalizeHomePageSettings(settings HomePageSettings) HomePageSettings {
	defaults := defaultHomePageSettings()
	if settings.PageSize == 0 {
		settings.PageSize = defaults.PageSize
	}
	if settings.PageSize < 1 {
		settings.PageSize = 1
	}
	if settings.PageSize > 100 {
		settings.PageSize = 100
	}
	return settings
}

func normalizePermalinkPattern(pattern, fallback string, allowed map[string]bool, required []string) string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return fallback
	}
	if err := validatePermalinkPattern(pattern, allowed, required); err != nil {
		return fallback
	}
	return pattern
}

func ValidatePermalinks(settings PermalinkSettings) error {
	if err := validatePermalinkSetting("post", settings.Post, postPermalinkAllowedTokens(), []string{"postname", "slug"}); err != nil {
		return err
	}
	if err := validatePermalinkSetting("page", settings.Page, pagePermalinkAllowedTokens(), []string{"pagename", "slug"}); err != nil {
		return err
	}
	if err := validatePermalinkSetting("tag", settings.Tag, tagPermalinkAllowedTokens(), []string{"tag", "slug"}); err != nil {
		return err
	}
	return nil
}

func validatePermalinkSetting(name, pattern string, allowed map[string]bool, required []string) error {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil
	}
	if err := validatePermalinkPattern(pattern, allowed, required); err != nil {
		return fmt.Errorf("%s permalink: %w", name, err)
	}
	return nil
}

func validatePermalinkPattern(pattern string, allowed map[string]bool, required []string) error {
	if !strings.HasPrefix(pattern, "/") {
		return fmt.Errorf("must start with /")
	}
	if strings.ContainsAny(pattern, "?#") {
		return fmt.Errorf("must not contain query strings or fragments")
	}
	if strings.Contains(pattern, "//") {
		return fmt.Errorf("must not contain empty path segments")
	}
	for _, segment := range strings.Split(pattern, "/") {
		if segment == "." || segment == ".." {
			return fmt.Errorf("must not contain . or .. segments")
		}
	}
	if !containsAnyPermalinkToken(pattern, required) {
		return fmt.Errorf("must include one of %s", strings.Join(formatPermalinkTokenNames(required), ", "))
	}
	if unknown := unknownPermalinkTokens(pattern, allowed); len(unknown) > 0 {
		return fmt.Errorf("unknown token %s", strings.Join(formatPermalinkTokenNames(unknown), ", "))
	}
	return nil
}

func containsAnyPermalinkToken(pattern string, names []string) bool {
	for _, name := range names {
		if strings.Contains(strings.ToLower(pattern), "%"+strings.ToLower(name)+"%") {
			return true
		}
	}
	return false
}

func unknownPermalinkTokens(pattern string, allowed map[string]bool) []string {
	var unknown []string
	seen := map[string]bool{}
	for _, match := range permalinkTokenPattern.FindAllStringSubmatch(pattern, -1) {
		if len(match) < 2 {
			continue
		}
		name := strings.ToLower(match[1])
		if allowed[name] || seen[name] {
			continue
		}
		unknown = append(unknown, name)
		seen[name] = true
	}
	return unknown
}

func formatPermalinkTokenNames(names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, "%"+name+"%")
	}
	return out
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

func PostURL(settings Settings, post *Post) string {
	if post == nil {
		return ""
	}
	if settings.Permalinks == (PermalinkSettings{}) && post.URL != "" {
		return post.URL
	}
	permalinks := normalizePermalinks(settings.Permalinks)
	return formatPermalink(permalinks.Post, postPermalinkValues(post.Slug, post.Date))
}

func PostDraftURL(settings Settings, draft PostDraft) string {
	date, _ := parseContentTime(draft.Date, TimeLocation(settings))
	permalinks := normalizePermalinks(settings.Permalinks)
	return formatPermalink(permalinks.Post, postPermalinkValues(draft.Slug, date))
}

func PageURL(settings Settings, page *Page) string {
	if page == nil {
		return ""
	}
	if settings.Permalinks == (PermalinkSettings{}) && page.URL != "" {
		return page.URL
	}
	permalinks := normalizePermalinks(settings.Permalinks)
	return formatPermalink(permalinks.Page, map[string]string{
		"pagename": page.Slug,
		"slug":     page.Slug,
	})
}

func PageDraftURL(settings Settings, draft PageDraft) string {
	permalinks := normalizePermalinks(settings.Permalinks)
	return formatPermalink(permalinks.Page, map[string]string{
		"pagename": draft.Slug,
		"slug":     draft.Slug,
	})
}

func TagURL(settings Settings, tag *Tag) string {
	if tag == nil {
		return ""
	}
	if settings.Permalinks == (PermalinkSettings{}) && tag.URL != "" {
		return tag.URL
	}
	permalinks := normalizePermalinks(settings.Permalinks)
	return formatPermalink(permalinks.Tag, map[string]string{
		"tag":  tag.Slug,
		"slug": tag.Slug,
	})
}

func TagSlugURL(settings Settings, slug string) string {
	permalinks := normalizePermalinks(settings.Permalinks)
	return formatPermalink(permalinks.Tag, map[string]string{
		"tag":  slug,
		"slug": slug,
	})
}

func postPermalinkValues(slug string, date time.Time) map[string]string {
	values := map[string]string{
		"postname": slug,
		"slug":     slug,
		"year":     "0000",
		"monthnum": "00",
		"day":      "00",
	}
	if !date.IsZero() {
		values["year"] = date.Format("2006")
		values["monthnum"] = date.Format("01")
		values["day"] = date.Format("02")
	}
	return values
}

func formatPermalink(pattern string, values map[string]string) string {
	path := permalinkTokenPattern.ReplaceAllStringFunc(pattern, func(token string) string {
		name := strings.ToLower(strings.Trim(token, "%"))
		if value, ok := values[name]; ok {
			return value
		}
		return token
	})
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func samePermalinkPath(a, b string) bool {
	return comparablePermalinkPath(a) == comparablePermalinkPath(b)
}

func comparablePermalinkPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "/"
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	if len(value) > 1 {
		value = strings.TrimRight(value, "/")
	}
	if value == "" {
		return "/"
	}
	return value
}

func postPermalinkAllowedTokens() map[string]bool {
	return map[string]bool{
		"postname": true,
		"slug":     true,
		"year":     true,
		"monthnum": true,
		"day":      true,
	}
}

func pagePermalinkAllowedTokens() map[string]bool {
	return map[string]bool{
		"pagename": true,
		"slug":     true,
	}
}

func tagPermalinkAllowedTokens() map[string]bool {
	return map[string]bool{
		"tag":  true,
		"slug": true,
	}
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

// normalizedSettingsVersion 返回当前应用应写回 settings.json 的配置版本。
//
// 设计说明：
//   - 老版本 settings.json 没有版本字段，保存后写入当前配置版本，后续启动就不会
//     重复执行只面向旧结构的迁移。
//   - 如果将来新版程序写入了更高版本，旧程序不应把版本号降级，避免破坏后续迁移判断。
//
// @param value settings.json 中读到的版本号。
// @returns 返回应写回 settings.json 的版本号。
func normalizedSettingsVersion(value string) string {
	value = strings.TrimSpace(value)
	if compare, ok := compareSettingsVersions(value, currentSettingsVersion); ok && compare > 0 {
		return value
	}
	return currentSettingsVersion
}

// settingsVersionAtOrBefore 判断 settings.json 是否不晚于某个迁移版本。
//
// @param version settings.json 中保存的版本；空值表示旧版未记录版本。
// @param target 迁移上限版本，格式为 vX.Y.Z。
// @returns 版本缺失、无法解析或小于等于 target 时返回 true。
func settingsVersionAtOrBefore(version, target string) bool {
	version = strings.TrimSpace(version)
	if version == "" {
		return true
	}
	compare, ok := compareSettingsVersions(version, target)
	if !ok {
		return true
	}
	return compare <= 0
}

// compareSettingsVersions 比较两个 Postizer 版本号。
//
// @param left 左侧版本号，支持 vX.Y.Z 或 X.Y.Z。
// @param right 右侧版本号，支持 vX.Y.Z 或 X.Y.Z。
// @returns 返回 -1/0/1 表示 left 小于/等于/大于 right；解析失败时 ok=false。
func compareSettingsVersions(left, right string) (result int, ok bool) {
	leftParts, ok := parseSettingsVersion(left)
	if !ok {
		return 0, false
	}
	rightParts, ok := parseSettingsVersion(right)
	if !ok {
		return 0, false
	}
	for index := range leftParts {
		if leftParts[index] < rightParts[index] {
			return -1, true
		}
		if leftParts[index] > rightParts[index] {
			return 1, true
		}
	}
	return 0, true
}

// parseSettingsVersion 把 vX.Y.Z 版本号解析为三段整数。
//
// @param value 待解析版本号。
// @returns 返回三段版本号；格式无效时 ok=false。
func parseSettingsVersion(value string) ([3]int, bool) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return [3]int{}, false
	}
	var parsed [3]int
	for index, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 {
			return [3]int{}, false
		}
		parsed[index] = number
	}
	return parsed, true
}

// normalizeThemeSettings 清洗主题菜单、菜单位置、侧边栏和主题自定义设置。
//
// @param settings settings.json 中读取到的主题设置。
// @returns 返回可供运行时使用并可安全写回 settings.json 的主题设置。
func normalizeThemeSettings(settings ThemeSettings) ThemeSettings {
	settings.Menus = migrateLegacyCustomMenuLinks(settings.Menus)
	menus := normalizeMenuCollection(settings.Menus, "menu")
	menuIDs := map[string]bool{}
	for _, menu := range menus {
		menuIDs[menu.ID] = true
	}

	sidebars := normalizeSidebarCollection(settings.Sidebars, "sidebar")
	sidebarIDs := map[string]bool{}
	for _, sidebar := range sidebars {
		sidebarIDs[sidebar.ID] = true
	}
	sidebar := normalizeSidebarSelection(settings.Sidebar, sidebarIDs)

	custom := normalizeThemeCustomSettings(settings.Custom)
	custom = migrateLegacyPureWhiteHeroSettings(settings.MenuLocations, custom)

	locations := map[string]string{}
	for location, menuID := range settings.MenuLocations {
		location = normalizeMenuID(location)
		menuID = normalizeMenuID(menuID)
		if location == "" || legacyPureWhiteHeroLocation(location) {
			continue
		}
		if menuID == "" {
			locations[location] = ""
			continue
		}
		if menuID == defaultThemeMenuID {
			locations[location] = menuID
			continue
		}
		if !menuIDs[menuID] {
			continue
		}
		locations[location] = menuID
	}

	return ThemeSettings{Menus: menus, MenuLocations: locations, Sidebars: sidebars, Sidebar: sidebar, Custom: custom}
}

// migrateLegacyCustomMenuLinks 迁移旧菜单中的自定义链接类型。
//
// 旧版自定义链接写作 type="custom"；当前菜单编辑器把“自定义链接”写作
// type="url"，而 type="custom" 已被侧边栏区块复用。这里仅迁移主题菜单列表，
// 不处理侧边栏区块，避免把新版自定义区域误改为普通链接。
//
// @param menus settings.json 中的主题菜单列表。
// @returns 返回迁移后的菜单副本。
func migrateLegacyCustomMenuLinks(menus []Menu) []Menu {
	if len(menus) == 0 {
		return menus
	}
	migrated := make([]Menu, len(menus))
	for index, menu := range menus {
		migrated[index] = menu
		migrated[index].Items = migrateLegacyCustomMenuItems(menu.Items)
	}
	return migrated
}

// migrateLegacyCustomMenuItems 把旧版 type="custom" 菜单项改成 type="url"。
//
// @param items 菜单项列表。
// @returns 返回迁移后的菜单项副本。
func migrateLegacyCustomMenuItems(items []MenuItem) []MenuItem {
	if len(items) == 0 {
		return items
	}
	migrated := make([]MenuItem, len(items))
	for index, item := range items {
		migrated[index] = item
		if strings.EqualFold(strings.TrimSpace(item.Type), "custom") {
			migrated[index].Type = "url"
		}
		if len(item.Items) > 0 {
			migrated[index].Items = migrateLegacyCustomMenuItems(item.Items)
		}
	}
	return migrated
}

// normalizeSidebarSelection 清洗主题当前投放的侧边栏 ID。
//
// @param value settings.json 中保存的侧边栏选择；nil 表示默认侧边栏。
// @param sidebarIDs 当前存在的自定义侧边栏 ID 集合。
// @returns 返回清洗后的选择；nil 表示默认，空字符串表示显式不使用侧边栏。
func normalizeSidebarSelection(value *string, sidebarIDs map[string]bool) *string {
	if value == nil {
		return nil
	}
	sidebarID := normalizeMenuID(*value)
	if sidebarID == "" {
		return stringPtr("")
	}
	if !sidebarIDs[sidebarID] {
		return nil
	}
	return stringPtr(sidebarID)
}

// stringPtr 返回字符串指针，供可选配置字段保存显式空值。
//
// @param value 字符串值。
// @returns 返回指向 value 副本的指针。
func stringPtr(value string) *string {
	return &value
}

func normalizeMenuCollection(values []Menu, fallbackPrefix string) []Menu {
	normalized := make([]Menu, 0, len(values))
	usedIDs := map[string]bool{}
	for index, menu := range values {
		name := strings.TrimSpace(menu.Name)
		id := normalizeMenuID(menu.ID)
		if id == "" {
			id = normalizeMenuID(name)
		}
		if id == "" {
			id = fmt.Sprintf("%s-%d", fallbackPrefix, index+1)
		}
		baseID := id
		for suffix := 2; usedIDs[id]; suffix++ {
			id = fmt.Sprintf("%s-%d", baseID, suffix)
		}
		usedIDs[id] = true
		if name == "" {
			name = id
		}
		normalized = append(normalized, Menu{
			ID:    id,
			Name:  name,
			Items: normalizeMenuItems(menu.Items),
		})
	}
	return normalized
}

// normalizeSidebarCollection 清洗侧边栏列表。
//
// 设计说明：
// 侧边栏和自定义菜单一样，第一层是多个可命名的列表；区别在于侧边栏列表里的
// items 是区块。旧版 items 如果是直接链接，会保持为旧结构，渲染和后台初始数据
// 再把它包装为“自定义区域”，避免保存前就改写用户配置。
//
// @param values settings.json 中的侧边栏列表。
// @param fallbackPrefix 缺少 ID 时使用的前缀。
// @returns 返回归一化后的侧边栏列表。
func normalizeSidebarCollection(values []Menu, fallbackPrefix string) []Menu {
	normalized := make([]Menu, 0, len(values))
	usedIDs := map[string]bool{}
	for index, sidebar := range values {
		name := strings.TrimSpace(sidebar.Name)
		id := normalizeMenuID(sidebar.ID)
		if id == "" {
			id = normalizeMenuID(name)
		}
		if id == "" {
			id = fmt.Sprintf("%s-%d", fallbackPrefix, index+1)
		}
		baseID := id
		for suffix := 2; usedIDs[id]; suffix++ {
			id = fmt.Sprintf("%s-%d", baseID, suffix)
		}
		usedIDs[id] = true
		if name == "" {
			name = id
		}
		items := normalizeSidebarCollectionItems(sidebar.Items)
		normalized = append(normalized, Menu{ID: id, Name: name, Items: items})
	}
	return normalized
}

// normalizeThemeCustomSettings 清洗主题自定义设置。
//
// 设计说明：
// - 第一层主题 ID 使用和主题包一致的小写短横线格式。
// - 第二层设置名使用 snake_case，方便 settings.json 手动阅读。
// - 设置值只保留 string、integer、float 三类。
func normalizeThemeCustomSettings(settings ThemeCustomSettings) ThemeCustomSettings {
	custom := ThemeCustomSettings{}
	for themeID, values := range settings {
		themeID = normalizeMenuID(themeID)
		if themeID == "" || len(values) == 0 {
			continue
		}
		normalizedValues := map[string]ThemeSettingValue{}
		for key, value := range values {
			key = normalizeThemeSettingKey(key)
			if key == "" {
				continue
			}
			normalizedValue, ok := normalizeThemeSettingValue(value)
			if !ok {
				continue
			}
			normalizedValues[key] = normalizedValue
		}
		if len(normalizedValues) > 0 {
			custom[themeID] = normalizedValues
		}
	}
	if len(custom) == 0 {
		return nil
	}
	return custom
}

// normalizeThemeSettingValue 校验单个主题设置值。
//
// @param value 待清洗的设置值。
// @returns 返回清洗后的设置值，以及该值是否有效。
func normalizeThemeSettingValue(value ThemeSettingValue) (ThemeSettingValue, bool) {
	switch value.Type {
	case ThemeSettingTypeString:
		return StringThemeSetting(value.String), true
	case ThemeSettingTypeInteger:
		return IntegerThemeSetting(value.Integer), true
	case ThemeSettingTypeFloat:
		if math.IsNaN(value.Float) || math.IsInf(value.Float, 0) {
			return ThemeSettingValue{}, false
		}
		return FloatThemeSetting(value.Float), true
	default:
		return ThemeSettingValue{}, false
	}
}

// normalizeThemeSettingKey 把主题设置名归一为 snake_case。
//
// @param value 原始设置名。
// @returns 返回可持久化的设置名。
func normalizeThemeSettingKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = themeSettingKeyPattern.ReplaceAllString(value, "_")
	value = themeSettingKeyDivider.ReplaceAllString(value, "_")
	return strings.Trim(value, "_")
}

// migrateLegacyPureWhiteHeroSettings 迁移早期 Pure White 主题内编码存储。
//
// 之前为了只修改主题文件，封面标题/副标题曾经被十六进制编码进
// menu_locations 的 key 中。现在后端支持主题自定义设置后，读取旧数据时
// 自动迁移到 custom.pure-white.hero_title / hero_subtitle。
func migrateLegacyPureWhiteHeroSettings(locations map[string]string, custom ThemeCustomSettings) ThemeCustomSettings {
	for location := range locations {
		location = normalizeMenuID(location)
		var key, encoded string
		switch {
		case strings.HasPrefix(location, legacyPureWhiteHeroTitlePrefix):
			key = "hero_title"
			encoded = strings.TrimPrefix(location, legacyPureWhiteHeroTitlePrefix)
		case strings.HasPrefix(location, legacyPureWhiteHeroSubtitlePrefix):
			key = "hero_subtitle"
			encoded = strings.TrimPrefix(location, legacyPureWhiteHeroSubtitlePrefix)
		default:
			continue
		}
		value := decodeLegacyHexThemeSetting(encoded)
		if value == "" {
			continue
		}
		custom = setDefaultThemeStringSetting(custom, "pure-white", key, value)
	}
	if len(custom) == 0 {
		return nil
	}
	return custom
}

// setDefaultThemeStringSetting 在目标设置不存在时写入字符串默认值。
//
// 这个函数用于迁移旧数据，显式的新 custom 配置优先级更高。
func setDefaultThemeStringSetting(custom ThemeCustomSettings, themeID string, key string, value string) ThemeCustomSettings {
	themeID = normalizeMenuID(themeID)
	key = normalizeThemeSettingKey(key)
	if themeID == "" || key == "" {
		return custom
	}
	if custom == nil {
		custom = ThemeCustomSettings{}
	}
	values := custom[themeID]
	if values == nil {
		values = map[string]ThemeSettingValue{}
		custom[themeID] = values
	}
	if _, exists := values[key]; exists {
		return custom
	}
	values[key] = StringThemeSetting(value)
	return custom
}

// decodeLegacyHexThemeSetting 解码旧版十六进制主题设置。
//
// @param value 不带前缀的十六进制字符串。
// @returns 返回 UTF-8 文本；格式无效时返回空字符串。
func decodeLegacyHexThemeSetting(value string) string {
	body, err := hex.DecodeString(value)
	if err != nil || !utf8.Valid(body) {
		return ""
	}
	return string(body)
}

// legacyPureWhiteHeroLocation 判断 menu location 是否为 Pure White 旧版私有设置。
//
// @param location 已归一化的 menu location。
// @returns 如果应从 menu_locations 中移除则返回 true。
func legacyPureWhiteHeroLocation(location string) bool {
	return strings.HasPrefix(location, legacyPureWhiteHeroTitlePrefix) ||
		strings.HasPrefix(location, legacyPureWhiteHeroSubtitlePrefix)
}

// normalizeSidebarSectionItems 清洗一个侧边栏里的区块列表。
//
// @param items 侧边栏区块列表。
// @returns 返回只包含受支持区块类型的项目。
func normalizeSidebarSectionItems(items []MenuItem) []MenuItem {
	normalized := make([]MenuItem, 0, len(items))
	for _, item := range items {
		item.Type = normalizeSidebarSectionType(item.Type)
		item.Label = strings.TrimSpace(item.Label)
		item.Target = ""
		item.URL = ""
		if item.Type == SidebarSectionTypeCustom {
			item.Items = normalizeMenuItems(item.Items)
			if item.Label == "" && len(item.Items) == 0 {
				continue
			}
		} else {
			item.Items = nil
		}
		normalized = append(normalized, item)
	}
	return normalized
}

// normalizeSidebarSectionType 清洗侧边栏区块类型。
//
// @param value 原始区块类型。
// @returns 返回受支持的区块类型；未知值按自定义区域处理。
func normalizeSidebarSectionType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case SidebarSectionTypeNone:
		return SidebarSectionTypeNone
	case SidebarSectionTypeTopics:
		return SidebarSectionTypeTopics
	case SidebarSectionTypePages:
		return SidebarSectionTypePages
	case SidebarSectionTypeFeeds, "feed", "subscriptions":
		return SidebarSectionTypeFeeds
	case SidebarSectionTypeRecentPosts, "recent", "recent_posts":
		return SidebarSectionTypeRecentPosts
	case SidebarSectionTypeCustom:
		return SidebarSectionTypeCustom
	default:
		return SidebarSectionTypeCustom
	}
}

// isSidebarSectionType 判断类型是否是新版侧边栏区块类型。
//
// @param value 待检查的类型。
// @returns 属于侧边栏区块类型时返回 true。
func isSidebarSectionType(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case SidebarSectionTypeNone, SidebarSectionTypeCustom, SidebarSectionTypeTopics, SidebarSectionTypePages, SidebarSectionTypeFeeds, SidebarSectionTypeRecentPosts:
		return true
	default:
		return false
	}
}

func normalizeMenuItems(items []MenuItem) []MenuItem {
	normalized := make([]MenuItem, 0, len(items))
	for _, item := range items {
		item.Type = strings.ToLower(strings.TrimSpace(item.Type))
		if item.Type == "tagindex" {
			item.Type = "tags"
		}
		item.Label = strings.TrimSpace(item.Label)
		item.Target = strings.TrimSpace(item.Target)
		item.URL = strings.TrimSpace(item.URL)
		item.Items = nil
		switch item.Type {
		case "":
			if item.Label == "" && item.Target == "" {
				continue
			}
			item.URL = ""
		case "page", "post", "tag":
			if item.Target == "" {
				continue
			}
			item.URL = ""
		case "url":
			url, ok := normalizeMenuURL(item.URL)
			if !ok {
				continue
			}
			item.URL = url
			item.Target = ""
		case "home", "archive", "tags", "search", "admin":
			item.Target = ""
			item.URL = ""
		default:
			continue
		}
		normalized = append(normalized, item)
	}
	return normalized
}

func normalizeMenuID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(value, "-")
	value = regexp.MustCompile(`-+`).ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}

func ValidMenuURL(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") {
		return true
	}
	return strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "mailto:")
}

// normalizeMenuURL 把后台自定义链接输入归一成可直接渲染的安全 URL。
//
// 设计说明：
// - 以 / 开头的站内路径、http(s) 和 mailto 链接保持原样。
// - 裸域名是后台输入的常见形式，例如 example.com/docs；保存时补成 https 链接。
// - 协议相对 URL、脚本协议、空白字符和没有明显主机名的值继续拒绝，避免保存后生成危险链接。
//
// @param value 用户在自定义链接输入框中填写的原始 URL。
// @returns 第一个返回值是归一化后的 URL，第二个返回值表示是否可保存。
func normalizeMenuURL(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if ValidMenuURL(value) {
		return value, true
	}
	if value == "" || strings.HasPrefix(value, "/") || strings.ContainsAny(value, " \t\r\n") {
		return "", false
	}
	candidate := "https://" + value
	parsed, err := url.Parse(candidate)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", false
	}
	host := parsed.Hostname()
	if host == "" {
		return "", false
	}
	if !strings.EqualFold(host, "localhost") && !strings.Contains(host, ".") {
		return "", false
	}
	return candidate, true
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
		renderSource := stripMoreTags(markdown)
		htmlBody, err := renderMarkdown(md, renderSource)
		if err != nil {
			return err
		}
		post := &Post{
			Title:       fm.get("title", slug),
			Slug:        slug,
			Date:        parseDate(fm.get("date", ""), location),
			Updated:     parseDate(fm.get("updated", ""), location),
			Tags:        parseList(fm.get("tags", "")),
			Summary:     contentSummary(fm, markdown),
			Draft:       fm.get("draft", "false") == "true",
			TOC:         fm.get("toc", "true") != "false",
			HTML:        htmlBody,
			Source:      string(markdown),
			FilePath:    path,
			ReadingTime: readingTime(renderSource),
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
		renderSource := stripMoreTags(markdown)
		htmlBody, err := renderMarkdown(md, renderSource)
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
			Summary:  contentSummary(fm, markdown),
			Draft:    fm.get("draft", "false") == "true",
			TOC:      fm.get("toc", "false") == "true",
			HTML:     htmlBody,
			Source:   string(markdown),
			FilePath: path,
		}
		store.AllPages = append(store.AllPages, page)
		store.AllPagesBySlug[page.Slug] = page
		if !page.Draft {
			store.Pages = append(store.Pages, page)
			store.PagesBySlug[page.Slug] = page
		}
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
	protected, mathHTML := protectMathSegments(prepared)
	if err := md.Convert(protected, &out); err != nil {
		return "", err
	}
	html := out.String()
	for token, value := range mathHTML {
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

func protectMathSegments(source []byte) ([]byte, map[string]string) {
	text := string(source)
	replacements := map[string]string{}
	var out strings.Builder
	var chunk strings.Builder
	inFence := false
	fenceMarker := ""

	flushChunk := func() {
		if chunk.Len() == 0 {
			return
		}
		out.WriteString(protectMathInText(chunk.String(), replacements))
		chunk.Reset()
	}

	for _, line := range strings.SplitAfter(text, "\n") {
		trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\n"))
		if marker := markdownFenceMarker(trimmed); marker != "" {
			flushChunk()
			out.WriteString(line)
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
			out.WriteString(line)
			continue
		}
		chunk.WriteString(line)
	}
	flushChunk()

	return []byte(out.String()), replacements
}

func protectMathInText(text string, replacements map[string]string) string {
	var out strings.Builder
	for i := 0; i < len(text); {
		switch {
		case strings.HasPrefix(text[i:], "$$") && !isEscapedAt(text, i):
			if end := findMathEnd(text, i+2, "$$"); end != -1 {
				out.WriteString(addMathReplacement(text[i:end+2], replacements))
				i = end + 2
				continue
			}
		case strings.HasPrefix(text[i:], `\[`):
			if end := findMathEnd(text, i+2, `\]`); end != -1 {
				out.WriteString(addMathReplacement(text[i:end+2], replacements))
				i = end + 2
				continue
			}
		case strings.HasPrefix(text[i:], `\(`):
			if end := findMathEnd(text, i+2, `\)`); end != -1 {
				out.WriteString(addMathReplacement(text[i:end+2], replacements))
				i = end + 2
				continue
			}
		case text[i] == '$' && !isEscapedAt(text, i):
			if end := findInlineDollarEnd(text, i+1); end != -1 {
				out.WriteString(addMathReplacement(text[i:end+1], replacements))
				i = end + 1
				continue
			}
		}
		out.WriteByte(text[i])
		i++
	}
	return out.String()
}

func addMathReplacement(raw string, replacements map[string]string) string {
	token := fmt.Sprintf("POSTIZER_MATH_%06d", len(replacements))
	replacements[token] = stdhtml.EscapeString(raw)
	return token
}

func findMathEnd(text string, start int, delimiter string) int {
	for i := start; i <= len(text)-len(delimiter); i++ {
		if strings.HasPrefix(text[i:], delimiter) && !isEscapedAt(text, i) {
			return i
		}
	}
	return -1
}

func findInlineDollarEnd(text string, start int) int {
	for i := start; i < len(text); i++ {
		if text[i] != '$' || isEscapedAt(text, i) {
			continue
		}
		if strings.HasPrefix(text[i:], "$$") {
			continue
		}
		return i
	}
	return -1
}

func isEscapedAt(text string, index int) bool {
	backslashes := 0
	for i := index - 1; i >= 0 && text[i] == '\\'; i-- {
		backslashes++
	}
	return backslashes%2 == 1
}

func RenderMarkdown(source string) (template.HTML, error) {
	return renderMarkdown(newMarkdown(), []byte(source))
}

func SavePost(root string, draft PostDraft) (PostDraft, error) {
	if strings.TrimSpace(draft.Title) == "" {
		return PostDraft{}, fmt.Errorf("title is required")
	}
	draft.Slug = NormalizeSlug(draft.Title)
	if !ValidSlug(draft.Slug) {
		return PostDraft{}, fmt.Errorf("invalid post slug %q", draft.Slug)
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

	originalSlug := strings.TrimSpace(draft.OriginalSlug)
	draft.Slug, err = uniqueContentSlug(root, "posts", draft.Slug, originalSlug)
	if err != nil {
		return PostDraft{}, err
	}
	if err := writeContentFile(root, "posts", draft.Slug, serializePost(draft)); err != nil {
		return PostDraft{}, err
	}
	if err := removeOldContentFile(root, "posts", originalSlug, draft.Slug); err != nil {
		return PostDraft{}, err
	}
	return draft, nil
}

func SavePage(root string, draft PageDraft) (PageDraft, error) {
	if strings.TrimSpace(draft.Title) == "" {
		return PageDraft{}, fmt.Errorf("title is required")
	}
	draft.Slug = NormalizeSlug(draft.Title)
	if !ValidSlug(draft.Slug) {
		return PageDraft{}, fmt.Errorf("invalid page slug %q", draft.Slug)
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

	originalSlug := strings.TrimSpace(draft.OriginalSlug)
	draft.Slug, err = uniqueContentSlug(root, "pages", draft.Slug, originalSlug)
	if err != nil {
		return PageDraft{}, err
	}
	if err := writeContentFile(root, "pages", draft.Slug, serializePage(draft)); err != nil {
		return PageDraft{}, err
	}
	if err := removeOldContentFile(root, "pages", originalSlug, draft.Slug); err != nil {
		return PageDraft{}, err
	}
	return draft, nil
}

func SaveImportedPost(root string, draft PostDraft, overwrite bool) (PostDraft, error) {
	if strings.TrimSpace(draft.Title) == "" {
		return PostDraft{}, fmt.Errorf("title is required")
	}
	draft.Slug = NormalizeSlug(draft.Slug)
	if draft.Slug == "" {
		draft.Slug = NormalizeSlug(draft.Title)
	}
	if !ValidSlug(draft.Slug) {
		return PostDraft{}, fmt.Errorf("invalid post slug %q", draft.Slug)
	}
	draft, err := normalizeImportedPostDates(root, draft)
	if err != nil {
		return PostDraft{}, err
	}
	if !overwrite {
		draft.Slug, err = uniqueContentSlug(root, "posts", draft.Slug, "")
		if err != nil {
			return PostDraft{}, err
		}
	}
	if err := writeContentFile(root, "posts", draft.Slug, serializePost(draft)); err != nil {
		return PostDraft{}, err
	}
	return draft, nil
}

func SaveImportedPage(root string, draft PageDraft, overwrite bool) (PageDraft, error) {
	if strings.TrimSpace(draft.Title) == "" {
		return PageDraft{}, fmt.Errorf("title is required")
	}
	draft.Slug = NormalizeSlug(draft.Slug)
	if draft.Slug == "" {
		draft.Slug = NormalizeSlug(draft.Title)
	}
	if !ValidSlug(draft.Slug) {
		return PageDraft{}, fmt.Errorf("invalid page slug %q", draft.Slug)
	}
	draft, err := normalizeImportedPageDates(root, draft)
	if err != nil {
		return PageDraft{}, err
	}
	if !overwrite {
		draft.Slug, err = uniqueContentSlug(root, "pages", draft.Slug, "")
		if err != nil {
			return PageDraft{}, err
		}
	}
	if err := writeContentFile(root, "pages", draft.Slug, serializePage(draft)); err != nil {
		return PageDraft{}, err
	}
	return draft, nil
}

func DeletePost(root, slug string) error {
	return deleteContentFile(root, "posts", slug)
}

func DeletePage(root, slug string) error {
	return deleteContentFile(root, "pages", slug)
}

func writeContentFile(root, section, slug, content string) error {
	target, err := contentFilePath(root, section, slug)
	if err != nil {
		return err
	}
	return os.WriteFile(target, []byte(content), 0644)
}

func uniqueContentSlug(root, section, baseSlug, originalSlug string) (string, error) {
	if !ValidSlug(originalSlug) {
		originalSlug = ""
	}
	for suffix := 0; ; suffix++ {
		candidate := baseSlug
		if suffix > 0 {
			candidate = fmt.Sprintf("%s-%d", baseSlug, suffix+1)
		}
		if candidate == originalSlug {
			return candidate, nil
		}
		exists, err := contentFileExists(root, section, candidate)
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}
}

func contentFileExists(root, section, slug string) (bool, error) {
	target, err := contentFilePath(root, section, slug)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(target); err == nil {
		return true, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	return false, nil
}

func deleteContentFile(root, section, slug string) error {
	slug = strings.TrimSpace(slug)
	if !ValidSlug(slug) {
		return fmt.Errorf("invalid %s slug %q", strings.TrimSuffix(section, "s"), slug)
	}
	target, err := contentFilePath(root, section, slug)
	if err != nil {
		return err
	}
	return os.Remove(target)
}

func removeOldContentFile(root, section, originalSlug, targetSlug string) error {
	if originalSlug == "" || originalSlug == targetSlug || !ValidSlug(originalSlug) {
		return nil
	}
	target, err := contentFilePath(root, section, originalSlug)
	if err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func contentFilePath(root, section, slug string) (string, error) {
	contentDir := filepath.Join(root, section)
	if err := os.MkdirAll(contentDir, 0755); err != nil {
		return "", err
	}
	target := filepath.Join(contentDir, slug+".md")
	cleanContentDir, err := filepath.Abs(contentDir)
	if err != nil {
		return "", err
	}
	cleanTarget, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(cleanTarget, cleanContentDir+string(os.PathSeparator)) {
		return "", fmt.Errorf("%s path escapes content directory", strings.TrimSuffix(section, "s"))
	}
	return cleanTarget, nil
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
	fmt.Fprintf(&b, "draft: %t\n", draft.Draft)
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
		goldmark.WithExtensions(
			extension.GFM,
			extension.Footnote,
			extension.Typographer,
			highlighting.NewHighlighting(
				highlighting.WithStyle("monokai"),
				highlighting.WithGuessLanguage(true),
			),
		),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(gmhtml.WithXHTML()),
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

func contentSummary(fm frontMatter, markdown []byte) string {
	if summary := fm.get("summary", ""); summary != "" {
		return summary
	}
	return summaryFromMoreTag(markdown)
}

func summaryFromMoreTag(markdown []byte) string {
	text := normalizeMarkdownNewlines(string(markdown))
	match := moreTagPattern.FindStringIndex(text)
	if match == nil {
		return ""
	}
	return markdownSummaryText(text[:match[0]])
}

func stripMoreTags(markdown []byte) []byte {
	text := normalizeMarkdownNewlines(string(markdown))
	return []byte(moreTagPattern.ReplaceAllString(text, ""))
}

func normalizeMarkdownNewlines(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	return strings.ReplaceAll(value, "\r", "\n")
}

func markdownSummaryText(value string) string {
	value = summaryFencePattern.ReplaceAllString(value, " ")
	value = summaryHTMLComment.ReplaceAllString(value, " ")
	value = summaryImagePattern.ReplaceAllString(value, "$1")
	value = summaryLinkPattern.ReplaceAllString(value, "$1")
	value = summaryHeadingPattern.ReplaceAllString(value, "")
	value = summaryQuotePattern.ReplaceAllString(value, "")
	value = summaryListPattern.ReplaceAllString(value, "")
	value = summaryHTMLTagPattern.ReplaceAllString(value, " ")
	value = summaryEmphasisPattern.ReplaceAllString(value, "")
	value = summaryLineBreakPattern.ReplaceAllString(value, " ")
	value = strings.NewReplacer("`", "", `\`, "").Replace(value)
	return strings.Join(strings.Fields(stdhtml.UnescapeString(value)), " ")
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

func normalizeImportedPostDates(root string, draft PostDraft) (PostDraft, error) {
	settings, err := LoadSettings(root)
	if err != nil {
		return PostDraft{}, err
	}
	location := TimeLocation(settings)
	now := time.Now().In(location).Truncate(time.Minute)
	if strings.TrimSpace(draft.Date) == "" {
		draft.Date = FormatInputDateTime(now)
	} else {
		date, err := parseContentTime(draft.Date, location)
		if err != nil {
			return PostDraft{}, fmt.Errorf("invalid date %q", draft.Date)
		}
		draft.Date = FormatInputDateTime(date)
	}
	if strings.TrimSpace(draft.Updated) == "" {
		draft.Updated = draft.Date
	} else {
		updated, err := parseContentTime(draft.Updated, location)
		if err != nil {
			return PostDraft{}, fmt.Errorf("invalid updated date %q", draft.Updated)
		}
		draft.Updated = FormatInputDateTime(updated)
	}
	return draft, nil
}

func normalizeImportedPageDates(root string, draft PageDraft) (PageDraft, error) {
	settings, err := LoadSettings(root)
	if err != nil {
		return PageDraft{}, err
	}
	location := TimeLocation(settings)
	now := time.Now().In(location).Truncate(time.Minute)
	if strings.TrimSpace(draft.Date) == "" {
		draft.Date = FormatInputDateTime(now)
	} else {
		date, err := parseContentTime(draft.Date, location)
		if err != nil {
			return PageDraft{}, fmt.Errorf("invalid date %q", draft.Date)
		}
		draft.Date = FormatInputDateTime(date)
	}
	if strings.TrimSpace(draft.Updated) == "" {
		draft.Updated = draft.Date
	} else {
		updated, err := parseContentTime(draft.Updated, location)
		if err != nil {
			return PageDraft{}, fmt.Errorf("invalid updated date %q", draft.Updated)
		}
		draft.Updated = FormatInputDateTime(updated)
	}
	return draft, nil
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
	value = regexp.MustCompile(`[^\p{L}\p{N}]+`).ReplaceAllString(value, "-")
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
