package appearance

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const (
	// DefaultThemePackID 是系统内置默认主题包的稳定标识。
	DefaultThemePackID = "newspaper-classic"
	// LegacyDefaultTextPackID 仅用于把旧版文字包配置迁移到主题语言。
	LegacyDefaultTextPackID = "default-en"
	// LegacyChineseTextPackID 仅用于把旧版中文文字包迁移到主题语言。
	LegacyChineseTextPackID = "zh-cn"
)

const (
	// DirThemes 是主题包在资源包根目录中的固定子目录名。
	DirThemes = "themes"
	// DirPlugins 是插件包在资源包根目录中的固定子目录名。
	DirPlugins = "plugins"
	// DirTexts 保留给旧版文字包兼容读取，不再作为正式 UI 分类。
	DirTexts = "texts"
)

const (
	// maxPackZipFiles 限制一次安装最多可解压的普通文件数，避免异常 zip 生成海量小文件。
	maxPackZipFiles = 512
	// maxPackZipSingleFileBytes 限制资源包内单个文件的解压后大小。
	maxPackZipSingleFileBytes int64 = 16 << 20
	// maxPackZipTotalBytes 限制资源包整体解压后的总大小，抵御压缩炸弹。
	maxPackZipTotalBytes int64 = 64 << 20
)

// PackType 表示资源包/插件包类型。
type PackType string

const (
	// ThemePack 用于定义模板、样式以及主题内置多语言翻译。
	ThemePack PackType = "theme"
	// PluginPack 用于定义可排序的附属覆盖包。
	PluginPack PackType = "plugin"
	// LegacyTextPack 仅用于兼容读取旧版单语言文字包。
	LegacyTextPack PackType = "text"
)

// PackSource 标识资源包来自官方目录还是用户目录。
type PackSource string

const (
	// SourceOfficial 表示该包随着项目一起发布。
	SourceOfficial PackSource = "official"
	// SourceUser 表示该包由用户上传安装。
	SourceUser PackSource = "user"
)

// Selection 记录某一类资源包的选中项与启用状态。
//
// 当前只在主题包选择中继续沿用该结构，以兼容已有 settings.json。
type Selection struct {
	Enabled bool   `json:"enabled"`
	PackID  string `json:"pack_id"`
}

// Manifest 描述一个主题包或插件包的元信息。
//
// 约定：
// 1. `styles`、`templates_dir`、`translations_dir`、`messages_file` 都相对包根目录。
// 2. 主题包推荐通过 `translations_dir` 提供多语言翻译，并使用 `default_locale` 指定默认语言。
// 3. 插件包也可以通过 `translations_dir` 提供多语言覆盖。
// 4. 旧版 `text` 包使用 `messages_file + lang` 表示“单语言翻译包”。
type Manifest struct {
	ID              string   `json:"id"`
	Type            PackType `json:"type"`
	Name            string   `json:"name"`
	SortName        string   `json:"sort_name"`
	Version         string   `json:"version"`
	Description     string   `json:"description"`
	Lang            string   `json:"lang"`
	DefaultLocale   string   `json:"default_locale"`
	Tags            []string `json:"tags"`
	Styles          []string `json:"styles"`
	TemplatesDir    string   `json:"templates_dir"`
	TranslationsDir string   `json:"translations_dir"`
	MessagesFile    string   `json:"messages_file"`
}

// Pack 是运行时可用的主题包或插件包对象。
type Pack struct {
	Manifest
	Source       PackSource
	RootDir      string
	RelativeDir  string
	URLBase      string
	StyleURLs    []string
	TemplateDir  string
	BadgeKeys    []string
	Locales      []string
	Translations map[string]map[string]string
	Active       bool
	Order        int
}

// LocaleOption 用于在设置页里渲染当前主题支持的语言选项。
type LocaleOption struct {
	Code  string
	Label string
}

// Catalog 是一次扫描后的主题/插件目录快照和当前激活结果。
type Catalog struct {
	Themes          []Pack
	Plugins         []Pack
	ActiveTheme     Pack
	ActivePlugins   []Pack
	InactivePlugins []Pack
	ThemeSelection  Selection
	ThemeLocale     string
	ThemeLocales    []LocaleOption
	PluginOrder     []string
	Messages        map[string]string
	Lang            string
	ThemeStyles     []string
}

// InstalledPack 表示一次用户上传安装后的结果摘要。
type InstalledPack struct {
	ID     string   `json:"id"`
	Type   PackType `json:"type"`
	Name   string   `json:"name"`
	Source string   `json:"source"`
}

// LoadCatalog 扫描主题包与插件包，构建运行时外观目录。
//
// 参数：
// - officialRoot: 官方资源目录
// - userRoot: 用户资源目录
// - themeSelection: 当前主题选择
// - themeLocale: 当前主题语言
// - pluginOrder: 当前启用插件包顺序，索引越小优先级越高
func LoadCatalog(officialRoot, userRoot string, themeSelection Selection, themeLocale string, pluginOrder []string) (Catalog, error) {
	themeSelection = normalizeSelection(themeSelection, DefaultThemePackID)

	themes, err := scanThemePacks(officialRoot, userRoot)
	if err != nil {
		return Catalog{}, err
	}
	plugins, err := scanPluginPacks(officialRoot, userRoot)
	if err != nil {
		return Catalog{}, err
	}

	themeMap := map[string]Pack{}
	for _, pack := range themes {
		if _, exists := themeMap[pack.ID]; exists {
			return Catalog{}, fmt.Errorf("duplicate theme pack id %q", pack.ID)
		}
		themeMap[pack.ID] = pack
	}
	pluginMap := map[string]Pack{}
	for _, pack := range plugins {
		if _, exists := pluginMap[pack.ID]; exists {
			return Catalog{}, fmt.Errorf("duplicate plugin pack id %q", pack.ID)
		}
		pluginMap[pack.ID] = pack
	}

	defaultTheme, ok := themeMap[DefaultThemePackID]
	if !ok {
		return Catalog{}, fmt.Errorf("default theme pack %q is missing", DefaultThemePackID)
	}

	activeTheme := defaultTheme
	if themeSelection.Enabled && themeSelection.PackID != "" {
		if selected, ok := themeMap[themeSelection.PackID]; ok {
			activeTheme = selected
		}
	}

	activePlugins, inactivePlugins, normalizedPluginOrder := resolvePluginOrder(plugins, pluginMap, pluginOrder)
	defaultLocale := effectiveDefaultLocale(activeTheme)
	themeLocale = normalizeThemeLocale(themeLocale, activeTheme.Locales, defaultLocale)
	themeLocales := buildLocaleOptions(activeTheme.Locales, themeLocale)

	messages := cloneMessages(activeTheme.Translations[defaultLocale])
	if themeLocale != defaultLocale {
		mergeMessages(messages, activeTheme.Translations[themeLocale])
	}
	for index := len(activePlugins) - 1; index >= 0; index-- {
		mergeMessages(messages, activePlugins[index].Translations[themeLocale])
	}

	return Catalog{
		Themes:          themes,
		Plugins:         append(append([]Pack(nil), activePlugins...), inactivePlugins...),
		ActiveTheme:     activeTheme,
		ActivePlugins:   activePlugins,
		InactivePlugins: inactivePlugins,
		ThemeSelection:  normalizeSelection(Selection{Enabled: activeTheme.ID != DefaultThemePackID, PackID: activeTheme.ID}, DefaultThemePackID),
		ThemeLocale:     themeLocale,
		ThemeLocales:    themeLocales,
		PluginOrder:     normalizedPluginOrder,
		Messages:        messages,
		Lang:            themeLocale,
		ThemeStyles:     append([]string(nil), activeTheme.StyleURLs...),
	}, nil
}

// InstallPackZIP 安装用户上传的 zip 资源包或插件包。
//
// 兼容策略：
// 1. 新版主题包 `type=theme`
// 2. 新版插件包 `type=plugin`
// 3. 旧版文字包 `type=text` 仍允许上传，并会被当作插件覆盖层读取
func InstallPackZIP(readerAt io.ReaderAt, size int64, userRoot string) (InstalledPack, error) {
	zr, err := zip.NewReader(readerAt, size)
	if err != nil {
		return InstalledPack{}, fmt.Errorf("open zip: %w", err)
	}

	manifestFile, prefix, err := zipManifest(zr.File)
	if err != nil {
		return InstalledPack{}, err
	}

	manifest, err := readZipManifest(manifestFile)
	if err != nil {
		return InstalledPack{}, err
	}
	if err := validateManifest(manifest); err != nil {
		return InstalledPack{}, err
	}

	targetDir := filepath.Join(userRoot, typeDirName(manifest.Type), manifest.ID)
	if err := os.MkdirAll(userRoot, 0755); err != nil {
		return InstalledPack{}, fmt.Errorf("create user root: %w", err)
	}
	tempRoot, err := os.MkdirTemp(userRoot, "pack-install-*")
	if err != nil {
		return InstalledPack{}, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tempRoot)

	stageDir := filepath.Join(tempRoot, manifest.ID)
	if err := os.MkdirAll(stageDir, 0755); err != nil {
		return InstalledPack{}, fmt.Errorf("create stage dir: %w", err)
	}

	var (
		fileCount int
		totalSize int64
	)
	for _, file := range zr.File {
		relative, ok := zipRelativePath(file.Name, prefix)
		if !ok {
			continue
		}
		if !file.FileInfo().IsDir() {
			fileCount++
			if fileCount > maxPackZipFiles {
				return InstalledPack{}, fmt.Errorf("zip contains too many files: limit is %d", maxPackZipFiles)
			}
			if file.UncompressedSize64 > uint64(maxPackZipSingleFileBytes) {
				return InstalledPack{}, fmt.Errorf("zip file %s is too large: limit is %d bytes", file.Name, maxPackZipSingleFileBytes)
			}
			if totalSize+int64(file.UncompressedSize64) > maxPackZipTotalBytes {
				return InstalledPack{}, fmt.Errorf("zip is too large after extraction: limit is %d bytes", maxPackZipTotalBytes)
			}
		}
		written, err := extractZipEntry(file, filepath.Join(stageDir, filepath.FromSlash(relative)), minInt64(maxPackZipSingleFileBytes, maxPackZipTotalBytes-totalSize))
		if err != nil {
			return InstalledPack{}, err
		}
		totalSize += written
	}

	if err := os.MkdirAll(filepath.Dir(targetDir), 0755); err != nil {
		return InstalledPack{}, fmt.Errorf("create user pack dir: %w", err)
	}
	if err := os.RemoveAll(targetDir); err != nil {
		return InstalledPack{}, fmt.Errorf("replace existing pack: %w", err)
	}
	if err := os.Rename(stageDir, targetDir); err != nil {
		return InstalledPack{}, fmt.Errorf("activate pack: %w", err)
	}

	return InstalledPack{
		ID:     manifest.ID,
		Type:   manifest.Type,
		Name:   manifest.Name,
		Source: string(SourceUser),
	}, nil
}

// DeleteUserPack 删除用户本地安装的资源包目录。
//
// 参数：
// - userRoot: 用户资源包根目录，通常是内容目录下的 `packs`。
// - packType: 要删除的资源包类型，支持 theme、plugin 和兼容旧版的 text。
// - id: manifest 中声明的资源包 ID，只允许小写字母、数字和短横线。
//
// 返回值：
// - 删除成功时返回 nil。
// - 如果类型或 ID 不合法、目录不存在、目标不是目录，返回错误。
//
// 设计说明：
// 1. 删除路径完全由受信任的 userRoot、固定类型目录和校验后的 ID 拼出，避免路径穿越。
// 2. 这里只删除用户上传目录，官方资源包目录不会从这个函数进入。
// 3. 调用方需要先处理当前 settings；如果正在使用这个包，应先把设置恢复到安全默认值。
func DeleteUserPack(userRoot string, packType PackType, id string) error {
	if strings.TrimSpace(userRoot) == "" {
		return errors.New("user pack root is required")
	}
	switch packType {
	case ThemePack, PluginPack, LegacyTextPack:
	default:
		return fmt.Errorf("invalid pack type %q", packType)
	}
	if !validPackID(id) {
		return fmt.Errorf("invalid pack id %q", id)
	}

	targetDir := filepath.Join(userRoot, typeDirName(packType), id)
	info, err := os.Stat(targetDir)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("pack %q does not exist", id)
	}
	if err != nil {
		return fmt.Errorf("stat pack %q: %w", id, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("pack %q is not a directory", id)
	}
	if err := os.RemoveAll(targetDir); err != nil {
		return fmt.Errorf("delete pack %q: %w", id, err)
	}
	return nil
}

// LocaleLabel 把语言代码格式化成适合展示的名称。
func LocaleLabel(code string) string {
	switch strings.TrimSpace(code) {
	case "en":
		return "English"
	case "zh-CN":
		return "简体中文"
	case "zh-TW":
		return "繁體中文"
	case "ja":
		return "日本語"
	case "ko":
		return "한국어"
	default:
		if strings.TrimSpace(code) == "" {
			return "Unknown"
		}
		return code
	}
}

func scanThemePacks(officialRoot, userRoot string) ([]Pack, error) {
	packs, err := scanPacks(officialRoot, userRoot, ThemePack)
	if err != nil {
		return nil, err
	}
	sort.Slice(packs, func(i, j int) bool {
		leftRank := themeSortRank(packs[i])
		rightRank := themeSortRank(packs[j])
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		left := themeSortKey(packs[i])
		right := themeSortKey(packs[j])
		if left != right {
			return left < right
		}
		return packs[i].ID < packs[j].ID
	})
	return packs, nil
}

func scanPluginPacks(officialRoot, userRoot string) ([]Pack, error) {
	var packs []Pack

	found, err := scanPacks(officialRoot, userRoot, PluginPack)
	if err != nil {
		return nil, err
	}
	packs = append(packs, found...)

	// 只兼容读取用户目录里的旧版 text 包，避免把项目内置的历史文字包重新暴露到新 UI。
	legacy, err := scanLegacyTextPacks(userRoot)
	if err != nil {
		return nil, err
	}
	packs = append(packs, legacy...)

	sort.Slice(packs, func(i, j int) bool {
		left := themeSortKey(packs[i])
		right := themeSortKey(packs[j])
		if left != right {
			return left < right
		}
		return packs[i].ID < packs[j].ID
	})
	return packs, nil
}

func scanLegacyTextPacks(userRoot string) ([]Pack, error) {
	found, err := scanPackRoot(userRoot, SourceUser, LegacyTextPack)
	if err != nil {
		return nil, err
	}
	return found, nil
}

func scanPacks(officialRoot, userRoot string, packType PackType) ([]Pack, error) {
	var packs []Pack
	for _, sourceConfig := range []struct {
		source PackSource
		root   string
	}{
		{source: SourceOfficial, root: officialRoot},
		{source: SourceUser, root: userRoot},
	} {
		found, err := scanPackRoot(sourceConfig.root, sourceConfig.source, packType)
		if err != nil {
			return nil, err
		}
		packs = append(packs, found...)
	}

	sort.Slice(packs, func(i, j int) bool {
		leftRank := packTagRank(packs[i])
		rightRank := packTagRank(packs[j])
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		left := themeSortKey(packs[i])
		right := themeSortKey(packs[j])
		if left != right {
			return left < right
		}
		return packs[i].ID < packs[j].ID
	})
	return packs, nil
}

func scanPackRoot(root string, source PackSource, packType PackType) ([]Pack, error) {
	dir := filepath.Join(root, typeDirName(packType))
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s packs: %w", packType, err)
	}

	var packs []Pack
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		packDir := filepath.Join(dir, entry.Name())
		manifestPath := filepath.Join(packDir, "manifest.json")
		body, err := os.ReadFile(manifestPath)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read manifest %s: %w", manifestPath, err)
		}

		var manifest Manifest
		if err := json.Unmarshal(body, &manifest); err != nil {
			return nil, fmt.Errorf("parse manifest %s: %w", manifestPath, err)
		}
		if err := validateManifest(manifest); err != nil {
			return nil, fmt.Errorf("%s: %w", manifestPath, err)
		}
		if manifest.Type != packType {
			return nil, fmt.Errorf("%s: manifest type %q does not match folder type %q", manifestPath, manifest.Type, packType)
		}

		translations, locales, defaultLocale, err := loadPackTranslations(packDir, manifest)
		if err != nil {
			return nil, err
		}

		pack := Pack{
			Manifest:     manifest,
			Source:       source,
			RootDir:      packDir,
			RelativeDir:  path.Join(typeDirName(packType), entry.Name()),
			URLBase:      path.Join("/packs", string(source), typeDirName(packType), entry.Name()),
			Locales:      locales,
			Translations: translations,
		}
		pack.DefaultLocale = defaultLocale
		pack.TemplateDir = resolveExistingDir(packDir, manifest.TemplatesDir, "templates")
		pack.StyleURLs = resolveStyleURLs(pack, manifest.Styles)
		pack.BadgeKeys = packBadges(pack)

		packs = append(packs, pack)
	}
	return packs, nil
}

func loadPackTranslations(packDir string, manifest Manifest) (map[string]map[string]string, []string, string, error) {
	translations := map[string]map[string]string{}

	translationsDir := resolveExistingDir(packDir, manifest.TranslationsDir, "translations")
	if translationsDir != "" {
		entries, err := os.ReadDir(translationsDir)
		if err != nil {
			return nil, nil, "", fmt.Errorf("read translations dir %s: %w", translationsDir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".json" {
				continue
			}
			localeCode := strings.TrimSpace(strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())))
			if localeCode == "" {
				continue
			}
			messages, err := readMessagesFile(filepath.Join(translationsDir, entry.Name()))
			if err != nil {
				return nil, nil, "", err
			}
			translations[localeCode] = messages
		}
	}

	// 旧版 text 包兼容：messages.json + lang 视作单语言插件覆盖包。
	if len(translations) == 0 {
		messagesPath := resolveExistingFile(packDir, manifest.MessagesFile, "messages.json")
		if messagesPath != "" {
			localeCode := strings.TrimSpace(manifest.DefaultLocale)
			if localeCode == "" {
				localeCode = strings.TrimSpace(manifest.Lang)
			}
			if localeCode == "" {
				localeCode = "en"
			}
			messages, err := readMessagesFile(messagesPath)
			if err != nil {
				return nil, nil, "", err
			}
			translations[localeCode] = messages
		}
	}

	locales := make([]string, 0, len(translations))
	for localeCode := range translations {
		locales = append(locales, localeCode)
	}
	sort.Strings(locales)

	defaultLocale := strings.TrimSpace(manifest.DefaultLocale)
	if defaultLocale == "" {
		defaultLocale = strings.TrimSpace(manifest.Lang)
	}
	if defaultLocale == "" {
		if len(locales) > 0 {
			defaultLocale = locales[0]
		} else {
			defaultLocale = "en"
			translations[defaultLocale] = map[string]string{}
			locales = []string{defaultLocale}
		}
	}
	if _, ok := translations[defaultLocale]; !ok {
		translations[defaultLocale] = map[string]string{}
		locales = append(locales, defaultLocale)
		sort.Strings(locales)
	}

	return translations, locales, defaultLocale, nil
}

func effectiveDefaultLocale(pack Pack) string {
	if strings.TrimSpace(pack.DefaultLocale) != "" {
		return pack.DefaultLocale
	}
	if strings.TrimSpace(pack.Lang) != "" {
		return pack.Lang
	}
	if len(pack.Locales) > 0 {
		return pack.Locales[0]
	}
	return "en"
}

func normalizeThemeLocale(requested string, supported []string, fallback string) string {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return fallback
	}
	for _, localeCode := range supported {
		if localeCode == requested {
			return requested
		}
	}
	return fallback
}

func buildLocaleOptions(locales []string, selected string) []LocaleOption {
	locales = sortLocales(locales)
	options := make([]LocaleOption, 0, len(locales))
	for _, localeCode := range locales {
		options = append(options, LocaleOption{
			Code:  localeCode,
			Label: LocaleLabel(localeCode),
		})
	}
	return options
}

func sortLocales(locales []string) []string {
	sorted := append([]string(nil), locales...)
	sort.Slice(sorted, func(i, j int) bool {
		leftRank, leftLabel := localeSortParts(sorted[i])
		rightRank, rightLabel := localeSortParts(sorted[j])
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if leftLabel != rightLabel {
			return leftLabel < rightLabel
		}
		return sorted[i] < sorted[j]
	})
	return sorted
}

func resolvePluginOrder(all []Pack, packMap map[string]Pack, requestedOrder []string) ([]Pack, []Pack, []string) {
	var (
		active   []Pack
		inactive []Pack
		order    []string
		seen     = map[string]bool{}
	)

	for index, id := range requestedOrder {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		pack, ok := packMap[id]
		if !ok {
			continue
		}
		pack.Active = true
		pack.Order = index + 1
		active = append(active, pack)
		order = append(order, id)
		seen[id] = true
	}

	for _, pack := range all {
		if seen[pack.ID] {
			continue
		}
		inactive = append(inactive, pack)
	}

	return active, inactive, order
}

func themeSortKey(pack Pack) string {
	key := strings.TrimSpace(pack.SortName)
	if key == "" {
		key = strings.TrimSpace(pack.Name)
	}
	return strings.ToLower(key)
}

func themeSortRank(pack Pack) int {
	return packTagRank(pack)
}

func packBadges(pack Pack) []string {
	if pack.Type != ThemePack {
		return nil
	}
	var badges []string
	for _, tag := range sortedPackTags(pack.Tags) {
		switch strings.ToLower(strings.TrimSpace(tag)) {
		case "default":
			badges = append(badges, "badge.default")
		case "official":
			badges = append(badges, "badge.official")
		case "other":
			badges = append(badges, "badge.other")
		default:
			badges = append(badges, tag)
		}
	}
	return badges
}

func packTagRank(pack Pack) int {
	switch {
	case hasPackTag(pack.Tags, "default"):
		return 0
	case hasPackTag(pack.Tags, "official"):
		return 1
	default:
		return 2
	}
}

func sortedPackTags(tags []string) []string {
	sorted := append([]string(nil), tags...)
	sort.Slice(sorted, func(i, j int) bool {
		leftRank := badgeTagRank(sorted[i])
		rightRank := badgeTagRank(sorted[j])
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return strings.ToLower(strings.TrimSpace(sorted[i])) < strings.ToLower(strings.TrimSpace(sorted[j]))
	})
	return sorted
}

func badgeTagRank(tag string) int {
	switch strings.ToLower(strings.TrimSpace(tag)) {
	case "default":
		return 0
	case "official":
		return 1
	case "other":
		return 2
	default:
		return 3
	}
}

func hasPackTag(tags []string, expected string) bool {
	expected = strings.ToLower(strings.TrimSpace(expected))
	for _, tag := range tags {
		if strings.ToLower(strings.TrimSpace(tag)) == expected {
			return true
		}
	}
	return false
}

func localeSortParts(code string) (int, string) {
	switch strings.TrimSpace(code) {
	case "en":
		return 0, "english"
	case "zh-CN":
		return 1, "simplified chinese"
	case "zh-TW":
		return 2, "traditional chinese"
	default:
		return 3, strings.ToLower(LocaleLabel(code))
	}
}

func normalizeSelection(selection Selection, fallbackID string) Selection {
	if strings.TrimSpace(selection.PackID) == "" {
		selection.PackID = fallbackID
	}
	return selection
}

func validateManifest(manifest Manifest) error {
	switch manifest.Type {
	case ThemePack, PluginPack, LegacyTextPack:
	default:
		return fmt.Errorf("invalid pack type %q", manifest.Type)
	}
	if !validPackID(manifest.ID) {
		return fmt.Errorf("invalid pack id %q", manifest.ID)
	}
	if strings.TrimSpace(manifest.Name) == "" {
		return errors.New("pack name is required")
	}
	return validateManifestPaths(manifest)
}

func validPackID(value string) bool {
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

func validateManifestPaths(manifest Manifest) error {
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "templates_dir", value: manifest.TemplatesDir},
		{name: "translations_dir", value: manifest.TranslationsDir},
		{name: "messages_file", value: manifest.MessagesFile},
	} {
		if _, err := cleanPackPath(field.value); err != nil {
			return fmt.Errorf("%s: %w", field.name, err)
		}
	}
	for index, style := range manifest.Styles {
		if _, err := cleanPackPath(style); err != nil {
			return fmt.Errorf("styles[%d]: %w", index, err)
		}
	}
	return nil
}

// cleanPackPath 把 manifest 中声明的路径归一成 zip/URL 风格的相对路径。
//
// 设计重点：
// 1. manifest 来自用户上传的资源包，路径字段不能信任。
// 2. 所有路径都必须停留在包根目录内，因此拒绝绝对路径和任何 `..` 段。
// 3. Windows 风格反斜杠先转成 `/`，避免同一份资源包在不同平台上表现不一致。
func cleanPackPath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	cleaned := path.Clean(strings.ReplaceAll(value, "\\", "/"))
	if cleaned == "." || cleaned == ".." || path.IsAbs(cleaned) || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") {
		return "", fmt.Errorf("path %q escapes pack root", value)
	}
	return cleaned, nil
}

func typeDirName(packType PackType) string {
	switch packType {
	case ThemePack:
		return DirThemes
	case PluginPack:
		return DirPlugins
	case LegacyTextPack:
		return DirTexts
	default:
		return "unknown"
	}
}

func resolveExistingDir(root, configured, fallback string) string {
	candidates := []string{configured, fallback}
	for _, candidate := range candidates {
		cleaned, err := cleanPackPath(candidate)
		if err != nil || cleaned == "" {
			continue
		}
		full := filepath.Join(root, filepath.FromSlash(cleaned))
		info, err := os.Stat(full)
		if err == nil && info.IsDir() {
			return full
		}
	}
	return ""
}

func resolveExistingFile(root, configured, fallback string) string {
	candidates := []string{configured, fallback}
	for _, candidate := range candidates {
		cleaned, err := cleanPackPath(candidate)
		if err != nil || cleaned == "" {
			continue
		}
		full := filepath.Join(root, filepath.FromSlash(cleaned))
		info, err := os.Stat(full)
		if err == nil && !info.IsDir() {
			return full
		}
	}
	return ""
}

func resolveStyleURLs(pack Pack, styles []string) []string {
	var urls []string
	for _, style := range styles {
		cleaned, err := cleanPackPath(style)
		if err != nil || cleaned == "" {
			continue
		}
		urls = append(urls, path.Join(pack.URLBase, cleaned))
	}
	return urls
}

func readMessagesFile(path string) (map[string]string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read messages %s: %w", path, err)
	}
	var messages map[string]string
	if err := json.Unmarshal(body, &messages); err != nil {
		return nil, fmt.Errorf("parse messages %s: %w", path, err)
	}
	if messages == nil {
		return map[string]string{}, nil
	}
	return messages, nil
}

func cloneMessages(source map[string]string) map[string]string {
	cloned := map[string]string{}
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func mergeMessages(target, source map[string]string) {
	for key, value := range source {
		if strings.TrimSpace(value) == "" {
			continue
		}
		target[key] = value
	}
}

func zipManifest(files []*zip.File) (*zip.File, string, error) {
	var manifestFile *zip.File
	for _, file := range files {
		name := strings.Trim(file.Name, "/")
		if path.Base(name) != "manifest.json" {
			continue
		}
		if manifestFile != nil {
			return nil, "", errors.New("zip must contain exactly one manifest.json")
		}
		manifestFile = file
	}
	if manifestFile == nil {
		return nil, "", errors.New("zip does not contain manifest.json")
	}
	prefix := path.Dir(strings.Trim(manifestFile.Name, "/"))
	if prefix == "." {
		prefix = ""
	}
	return manifestFile, prefix, nil
}

func readZipManifest(file *zip.File) (Manifest, error) {
	reader, err := file.Open()
	if err != nil {
		return Manifest{}, fmt.Errorf("open manifest: %w", err)
	}
	defer reader.Close()

	var manifest Manifest
	if err := json.NewDecoder(reader).Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse manifest: %w", err)
	}
	return manifest, nil
}

func zipRelativePath(name, prefix string) (string, bool) {
	name = strings.Trim(name, "/")
	if prefix != "" {
		prefix = strings.Trim(prefix, "/")
		if name == prefix {
			return "", false
		}
		if !strings.HasPrefix(name, prefix+"/") {
			return "", false
		}
		name = strings.TrimPrefix(name, prefix+"/")
	}
	name = path.Clean(name)
	if name == "." || name == "" {
		return "", false
	}
	if strings.HasPrefix(name, "../") || strings.Contains(name, "/../") || strings.HasPrefix(name, "/") {
		return "", false
	}
	return name, true
}

func extractZipEntry(file *zip.File, destination string, maxBytes int64) (int64, error) {
	if file.FileInfo().IsDir() {
		return 0, os.MkdirAll(destination, 0755)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return 0, fmt.Errorf("create zip parent dir: %w", err)
	}
	reader, err := file.Open()
	if err != nil {
		return 0, fmt.Errorf("open zip file %s: %w", file.Name, err)
	}
	defer reader.Close()

	out, err := os.Create(destination)
	if err != nil {
		return 0, fmt.Errorf("create extracted file %s: %w", destination, err)
	}
	defer out.Close()

	// zip 文件头中的未压缩大小不一定值得完全信任，所以复制阶段再用 LimitReader
	// 做第二层保护：最多允许写出 maxBytes，多出的 1 字节只用于判断越界。
	written, err := io.Copy(out, io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return written, fmt.Errorf("write extracted file %s: %w", destination, err)
	}
	if written > maxBytes {
		return written, fmt.Errorf("zip file %s exceeds extraction limit of %d bytes", file.Name, maxBytes)
	}
	return written, out.Close()
}

func minInt64(left, right int64) int64 {
	if left < right {
		return left
	}
	return right
}
