package appearance

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
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
	// DirBundles 是可包含多套主题包的资源包集合目录名。
	DirBundles = "bundles"
	// DirPlugins 是插件包在资源包根目录中的固定子目录名。
	DirPlugins = "plugins"
	// DirTexts 保留给旧版文字包兼容读取，不再作为正式 UI 分类。
	DirTexts = "texts"
)

const (
	// maxPackZipFiles 限制一次安装最多可解压的普通文件数，避免异常 zip 生成海量小文件。
	maxPackZipFiles = 512
	// maxPackZipSingleFileBytes 限制资源包内单个文件的解压后大小。
	maxPackZipSingleFileBytes int64 = 64 << 20
	// maxPackZipTotalBytes 限制资源包整体解压后的总大小，抵御压缩炸弹。
	maxPackZipTotalBytes int64 = 256 << 20
)

// PackType 表示资源包/插件包类型。
type PackType string

const (
	// ThemePack 用于定义模板、样式以及主题内置多语言翻译。
	ThemePack PackType = "theme"
	// BundlePack 用于把多个主题包、多个插件包或二者的组合，作为一个安装/删除单位分发。
	BundlePack PackType = "bundle"
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

// BundleEntry 描述 bundle manifest 中的一个子资源包路径。
//
// Path 必须指向一个包含子 manifest 的目录；子 manifest 可以是 theme，也可以是 plugin。
// 如果 bundle manifest 省略 packs 字段，扫描器会自动读取 bundle 下的 themes/* 与 plugins/*。
type BundleEntry struct {
	Path string `json:"path"`
}

// MenuLocation 描述主题模板中可以放入自定义菜单的位置。
type MenuLocation struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// RuntimeKind describes how an interactive plugin is executed.
//
// Empty runtime fields keep the existing static resource-pack behavior:
// translations, templates, and styles can be loaded without starting a
// separate process.
type RuntimeKind string

const (
	RuntimeStatic RuntimeKind = ""
	RuntimeGRPC   RuntimeKind = "grpc"
)

// PluginRuntime describes an optional external process used by a plugin.
type PluginRuntime struct {
	Kind      RuntimeKind             `json:"kind"`
	Command   string                  `json:"command"`
	Args      []string                `json:"args"`
	WorkDir   string                  `json:"work_dir"`
	Env       map[string]string       `json:"env"`
	Platforms []PluginRuntimePlatform `json:"platforms"`
}

// PluginRuntimePlatform declares a platform-specific executable for a plugin.
type PluginRuntimePlatform struct {
	GOOS    string            `json:"goos"`
	GOArch  string            `json:"goarch"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	WorkDir string            `json:"work_dir"`
	Env     map[string]string `json:"env"`
}

// PluginPermission is a stable capability flag requested by a plugin.
type PluginPermission string

// PluginRequirements declares the minimum host capabilities a plugin expects.
type PluginRequirements struct {
	Postizer     string          `json:"postizer,omitempty"`
	HostServices []PluginService `json:"host_services,omitempty"`
}

// PluginService declares a service exposed by an external plugin runtime.
type PluginService struct {
	Name    string   `json:"name"`
	Methods []string `json:"methods"`
}

type UIEntry struct {
	ID     string `json:"id"`
	Outlet string `json:"outlet"`
	Path   string `json:"path"`
}

type PluginUI struct {
	Pages   []PluginUIPage   `json:"pages"`
	Actions []PluginUIAction `json:"actions"`
}

type PluginUIPage struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Actions     []string `json:"actions,omitempty"`
	LoadAction  string   `json:"load_action,omitempty"`
}

type PluginUIAction struct {
	ID                   string          `json:"id"`
	Label                string          `json:"label"`
	Description          string          `json:"description,omitempty"`
	Kind                 string          `json:"kind,omitempty"`
	Fields               []PluginUIField `json:"fields,omitempty"`
	RequiresConfirmation bool            `json:"requires_confirmation,omitempty"`
}

type PluginUIField struct {
	Name      string `json:"name"`
	Label     string `json:"label"`
	Type      string `json:"type"`
	Accept    string `json:"accept,omitempty"`
	Required  bool   `json:"required,omitempty"`
	Help      string `json:"help,omitempty"`
	MaxLength int    `json:"max_length,omitempty"`
}

// Manifest 描述一个主题包、插件包或 bundle 资源合集的元信息。
//
// 约定：
// 1. `styles`、`templates_dir`、`translations_dir`、`messages_file` 都相对包根目录。
// 2. 主题包推荐通过 `translations_dir` 提供多语言翻译，并使用 `default_locale` 指定默认语言。
// 3. 插件包也可以通过 `translations_dir` 提供多语言覆盖。
// 4. 旧版 `text` 包使用 `messages_file + lang` 表示“单语言翻译包”。
type Manifest struct {
	ID              string              `json:"id"`
	Type            PackType            `json:"type"`
	Name            string              `json:"name"`
	SortName        string              `json:"sort_name"`
	Version         string              `json:"version"`
	Description     string              `json:"description"`
	SourceURL       string              `json:"source_url"`
	Lang            string              `json:"lang"`
	DefaultLocale   string              `json:"default_locale"`
	Tags            []string            `json:"tags"`
	Styles          []string            `json:"styles"`
	TemplatesDir    string              `json:"templates_dir"`
	TranslationsDir string              `json:"translations_dir"`
	MessagesFile    string              `json:"messages_file"`
	MenuLocations   []MenuLocation      `json:"menu_locations"`
	Packs           []BundleEntry       `json:"packs"`
	Runtime         PluginRuntime       `json:"runtime"`
	Services        []PluginService     `json:"services"`
	Requires        *PluginRequirements `json:"requires,omitempty"`
	UIEntries       []UIEntry           `json:"ui_entries"`
	Permissions     []PluginPermission  `json:"permissions"`
}

// Pack 是运行时可用的主题包、插件包或 bundle 资源合集对象。
type Pack struct {
	Manifest
	Source        PackSource
	RootDir       string
	RelativeDir   string
	URLBase       string
	StyleURLs     []string
	TemplateDir   string
	BadgeKeys     []string
	Locales       []string
	Translations  map[string]map[string]string
	BundleID      string
	BundleName    string
	BundleVersion string
	// BundledThemeIDs 只在 Type=bundle 的父资源包上使用，记录该 bundle 内包含的主题 ID。
	// 删除 bundle 时会根据这些 ID 判断是否需要把当前主题恢复到默认主题。
	BundledThemeIDs []string
	// BundledPluginIDs 只在 Type=bundle 的父资源包上使用，记录该 bundle 内包含的插件 ID。
	// 删除 bundle 时会从插件启用顺序中移除这些 ID，避免 settings.json 残留悬空插件。
	BundledPluginIDs []string
	Active           bool
	Order            int
}

// LocaleOption 用于在设置页里渲染当前主题支持的语言选项。
type LocaleOption struct {
	Code  string
	Label string
}

// Catalog 是一次扫描后的主题/插件目录快照和当前激活结果。
type Catalog struct {
	Themes          []Pack
	Bundles         []Pack
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
	ID        string   `json:"id"`
	Type      PackType `json:"type"`
	Name      string   `json:"name"`
	Source    string   `json:"source"`
	SourceURL string   `json:"source_url,omitempty"`
	Warnings  []string `json:"warnings,omitempty"`
}

// HostCompatibility describes the Postizer capabilities available during pack installation.
type HostCompatibility struct {
	PostizerVersion string
	PluginServices  []PluginService
	HostServices    []PluginService
}

// LoadCatalog 扫描主题包与插件包，构建运行时外观目录。
//
// 参数：
// - officialRoot: 内置 bundle 根目录，通常是 internal/bundles
// - userRoot: 用户内容根目录，bundle 会从 content/bundles 读取
// - themeSelection: 当前主题选择
// - themeLocale: 当前主题语言
// - pluginOrder: 当前启用插件包顺序，索引越小优先级越高
func LoadCatalog(officialRoot, userRoot string, themeSelection Selection, themeLocale string, pluginOrder []string) (Catalog, error) {
	themeSelection = normalizeSelection(themeSelection, DefaultThemePackID)

	themes, err := scanThemePacks(officialRoot, userRoot)
	if err != nil {
		return Catalog{}, err
	}
	bundles, err := scanBundlePacks(officialRoot, userRoot)
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
	themes = themesWithActivePluginLocales(defaultTheme, themes, activePlugins)
	for _, theme := range themes {
		if theme.ID == defaultTheme.ID {
			defaultTheme = theme
		}
		if theme.ID == activeTheme.ID {
			activeTheme = theme
		}
	}
	defaultLocale := effectiveDefaultLocale(activeTheme)
	themeLocale = normalizeThemeLocale(themeLocale, activeTheme.Locales, defaultLocale)
	themeLocales := buildLocaleOptions(activeTheme.Locales, themeLocale)

	// 非默认主题通常只关心少量视觉文案覆盖，例如首页副标题或导航短名。
	// 先加载默认主题的完整文案，再把当前主题文案覆盖上去，可以避免新主题为了
	// 一个 CSS 变体复制整套翻译文件；同时插件仍然保持最高优先级。
	messages := localizedPackMessages(defaultTheme, themeLocale)
	if activeTheme.ID != defaultTheme.ID {
		mergeMessages(messages, localizedPackMessages(activeTheme, themeLocale))
	}
	for index := len(activePlugins) - 1; index >= 0; index-- {
		mergeMessages(messages, activePlugins[index].Translations[themeLocale])
	}

	return Catalog{
		Themes:          themes,
		Bundles:         bundles,
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

// InstallPackZIP 安装用户上传的 zip 资源包。
//
// 参数：
// - readerAt: zip 文件内容。
// - size: zip 文件字节数。
// - userRoot: 用户内容根目录，通常是 content。
//
// 返回值：
// - 安装成功时返回安装摘要，Source 固定为 user。
// - zip 不合法、manifest 不合法、不是 bundle 类型、解压越界或校验失败时返回错误。
//
// 设计说明：
// 新上传统一以 bundle 为单位，写入 content/bundles/<id>。
// bundle 内可以包含多个 theme、多个 plugin，或二者混合。
// 旧版独立 theme/plugin/text 仍可被扫描读取，但不再作为上传安装格式。
func InstallPackZIP(readerAt io.ReaderAt, size int64, userRoot string) (InstalledPack, error) {
	return InstallPackZIPWithCompatibility(readerAt, size, userRoot, HostCompatibility{})
}

func ReadPackZIPManifest(readerAt io.ReaderAt, size int64) (Manifest, error) {
	zr, err := zip.NewReader(readerAt, size)
	if err != nil {
		return Manifest{}, fmt.Errorf("open zip: %w", err)
	}

	manifestFile, _, err := zipManifest(zr.File)
	if err != nil {
		return Manifest{}, err
	}

	manifest, err := readZipManifest(manifestFile)
	if err != nil {
		return Manifest{}, err
	}
	if err := validateManifest(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func InstallPackZIPWithCompatibility(readerAt io.ReaderAt, size int64, userRoot string, compatibility HostCompatibility) (InstalledPack, error) {
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
	if manifest.Type != BundlePack {
		return InstalledPack{}, fmt.Errorf("uploaded resource pack must be %q, got %q", BundlePack, manifest.Type)
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

	if manifest.Type == BundlePack {
		if err := validateBundleInstall(stageDir, manifest); err != nil {
			return InstalledPack{}, err
		}
		if err := pruneBundleRuntimesForCurrentPlatform(stageDir, manifest); err != nil {
			return InstalledPack{}, err
		}
	}
	warnings := compatibilityWarnings(stageDir, manifest, compatibility)

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
		ID:        manifest.ID,
		Type:      manifest.Type,
		Name:      manifest.Name,
		Source:    string(SourceUser),
		SourceURL: strings.TrimSpace(manifest.SourceURL),
		Warnings:  warnings,
	}, nil
}

// DeleteUserPack 删除用户本地安装的资源包目录。
//
// 参数：
// - userRoot: 用户资源包根目录，通常是内容目录下的 `packs`。
// - packType: 要删除的资源包类型，支持 bundle、theme、plugin 和兼容旧版的 text。
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
	case ThemePack, BundlePack, PluginPack, LegacyTextPack:
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

// validateBundleInstall 校验用户上传的 bundle 是否包含可用子资源包。
//
// 参数：
// - bundleDir: 上传 zip 解压后的临时 bundle 根目录。
// - manifest: 顶层 bundle manifest。
//
// 返回值：
// - bundle 至少包含一个 theme 或 plugin，且同类型子包 ID 不重复时返回 nil。
// - 子包缺失、类型不支持、路径非法或同类型 ID 重复时返回错误。
func validateBundleInstall(bundleDir string, manifest Manifest) error {
	children, err := scanBundleChildren(bundleDir, SourceUser, manifest, path.Join(DirBundles, manifest.ID), path.Join("/packs", string(SourceUser), DirBundles, manifest.ID), "")
	if err != nil {
		return fmt.Errorf("validate bundle children: %w", err)
	}
	if len(children) == 0 {
		return errors.New("bundle must contain at least one theme or plugin pack")
	}
	seen := map[PackType]map[string]bool{}
	for _, child := range children {
		if seen[child.Type] == nil {
			seen[child.Type] = map[string]bool{}
		}
		if seen[child.Type][child.ID] {
			return fmt.Errorf("bundle contains duplicate %s pack id %q", child.Type, child.ID)
		}
		if err := validatePackRuntimeForCurrentPlatform(child); err != nil {
			return err
		}
		seen[child.Type][child.ID] = true
	}
	return nil
}

func compatibilityWarnings(bundleDir string, manifest Manifest, compatibility HostCompatibility) []string {
	if compatibility.empty() {
		return nil
	}
	var warnings []string
	warnings = append(warnings, manifestCompatibilityWarnings(manifest, compatibility)...)
	if manifest.Type != BundlePack {
		return dedupeWarnings(warnings)
	}
	children, err := scanBundleChildren(bundleDir, SourceUser, manifest, path.Join(DirBundles, manifest.ID), path.Join("/packs", string(SourceUser), DirBundles, manifest.ID), "")
	if err != nil {
		return append(warnings, "Compatibility check could not inspect bundled packs: "+err.Error())
	}
	for _, child := range children {
		warnings = append(warnings, manifestCompatibilityWarnings(child.Manifest, compatibility)...)
	}
	return dedupeWarnings(warnings)
}

func manifestCompatibilityWarnings(manifest Manifest, compatibility HostCompatibility) []string {
	var warnings []string
	label := string(manifest.Type) + " " + manifest.ID
	if manifest.Requires != nil {
		warnings = append(warnings, postizerVersionWarnings(label, strings.TrimSpace(manifest.Requires.Postizer), compatibility.PostizerVersion)...)
		warnings = append(warnings, requiredServiceWarnings(label, manifest.Requires.HostServices, compatibility.HostServices, "HostService")...)
	} else if manifest.Runtime.Kind == RuntimeGRPC {
		warnings = append(warnings, fmt.Sprintf("%s does not declare requires; host RPC compatibility could not be fully checked.", label))
	}
	if manifest.Runtime.Kind == RuntimeGRPC {
		warnings = append(warnings, pluginServiceWarnings(label, manifest.Services, compatibility.PluginServices)...)
	}
	return warnings
}

func postizerVersionWarnings(label, required, current string) []string {
	if required == "" {
		return nil
	}
	current = strings.TrimSpace(current)
	if current == "" || strings.EqualFold(current, "dev") {
		return []string{fmt.Sprintf("%s requires Postizer %s or newer, but the current version is unknown.", label, required)}
	}
	compare, ok := compareReleaseVersions(current, required)
	if !ok {
		return []string{fmt.Sprintf("%s requires Postizer %s or newer, but the current version %q could not be compared.", label, required, current)}
	}
	if compare < 0 {
		return []string{fmt.Sprintf("%s requires Postizer %s or newer; current version is %s.", label, required, current)}
	}
	return nil
}

func requiredServiceWarnings(label string, required, supported []PluginService, serviceKind string) []string {
	if len(required) == 0 {
		return nil
	}
	supportedMethods := serviceMethodSet(supported)
	var warnings []string
	for _, service := range required {
		serviceName := strings.TrimSpace(service.Name)
		if serviceName == "" {
			warnings = append(warnings, fmt.Sprintf("%s declares an empty required %s name.", label, serviceKind))
			continue
		}
		methods, ok := supportedMethods[serviceName]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("%s requires unsupported %s %s.", label, serviceKind, serviceName))
			continue
		}
		if len(service.Methods) == 0 {
			warnings = append(warnings, fmt.Sprintf("%s requires %s %s but does not declare required methods.", label, serviceKind, serviceName))
			continue
		}
		for _, method := range service.Methods {
			method = strings.TrimSpace(method)
			if method == "" {
				warnings = append(warnings, fmt.Sprintf("%s declares an empty required method for %s %s.", label, serviceKind, serviceName))
				continue
			}
			if !methods[method] {
				warnings = append(warnings, fmt.Sprintf("%s requires unsupported %s method %s/%s.", label, serviceKind, serviceName, method))
			}
		}
	}
	return warnings
}

func pluginServiceWarnings(label string, declared, supported []PluginService) []string {
	if len(declared) == 0 {
		return []string{fmt.Sprintf("%s does not declare plugin service methods; PluginService compatibility could not be checked.", label)}
	}
	warnings := requiredServiceWarnings(label, declared, supported, "PluginService")
	declaredMethods := serviceMethodSet(declared)
	for serviceName, methods := range serviceMethodSet(supported) {
		declaredForService, ok := declaredMethods[serviceName]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("%s does not declare required PluginService %s.", label, serviceName))
			continue
		}
		for method := range methods {
			if !declaredForService[method] {
				warnings = append(warnings, fmt.Sprintf("%s does not declare required PluginService method %s/%s.", label, serviceName, method))
			}
		}
	}
	return warnings
}

func serviceMethodSet(services []PluginService) map[string]map[string]bool {
	set := map[string]map[string]bool{}
	for _, service := range services {
		name := strings.TrimSpace(service.Name)
		if name == "" {
			continue
		}
		if set[name] == nil {
			set[name] = map[string]bool{}
		}
		for _, method := range service.Methods {
			method = strings.TrimSpace(method)
			if method != "" {
				set[name][method] = true
			}
		}
	}
	return set
}

func dedupeWarnings(warnings []string) []string {
	if len(warnings) == 0 {
		return nil
	}
	deduped := make([]string, 0, len(warnings))
	seen := map[string]bool{}
	for _, warning := range warnings {
		warning = strings.TrimSpace(warning)
		if warning == "" || seen[warning] {
			continue
		}
		seen[warning] = true
		deduped = append(deduped, warning)
	}
	return deduped
}

func (compatibility HostCompatibility) empty() bool {
	return strings.TrimSpace(compatibility.PostizerVersion) == "" && len(compatibility.PluginServices) == 0 && len(compatibility.HostServices) == 0
}

func validatePackRuntimeForCurrentPlatform(pack Pack) error {
	if pack.Type != PluginPack || pack.Runtime.Kind != RuntimeGRPC {
		return nil
	}
	selected, hasCurrentPlatform := pluginRuntimeForCurrentPlatform(pack.Runtime)
	if !hasCurrentPlatform {
		return fmt.Errorf(
			"plugin %q does not include a runtime for current platform %s/%s; supported platforms: %s",
			pack.ID,
			runtime.GOOS,
			runtime.GOARCH,
			runtimeSupportedPlatforms(pack.Runtime),
		)
	}
	if err := validateRuntimeCommandExists(pack.RootDir, selected.Command); err != nil {
		return fmt.Errorf("plugin %q runtime for %s/%s: %w", pack.ID, runtime.GOOS, runtime.GOARCH, err)
	}
	return nil
}

func pluginRuntimeForCurrentPlatform(runtimeConfig PluginRuntime) (PluginRuntime, bool) {
	selected := PluginRuntime{
		Kind:    runtimeConfig.Kind,
		Command: strings.TrimSpace(runtimeConfig.Command),
		Args:    append([]string(nil), runtimeConfig.Args...),
		WorkDir: strings.TrimSpace(runtimeConfig.WorkDir),
		Env:     cloneStringMap(runtimeConfig.Env),
	}
	for _, platform := range runtimeConfig.Platforms {
		if platform.GOOS == runtime.GOOS && platform.GOArch == runtime.GOARCH {
			selected.Command = strings.TrimSpace(platform.Command)
			if platform.Args != nil {
				selected.Args = append([]string(nil), platform.Args...)
			}
			if strings.TrimSpace(platform.WorkDir) != "" {
				selected.WorkDir = strings.TrimSpace(platform.WorkDir)
			}
			selected.Env = mergeStringMap(selected.Env, platform.Env)
			selected.Platforms = nil
			return selected, true
		}
	}
	selected.Platforms = nil
	return selected, selected.Command != ""
}

func runtimeSupportedPlatforms(runtimeConfig PluginRuntime) string {
	if len(runtimeConfig.Platforms) == 0 {
		return "none"
	}
	platforms := make([]string, 0, len(runtimeConfig.Platforms))
	for _, platform := range runtimeConfig.Platforms {
		platforms = append(platforms, platform.GOOS+"/"+platform.GOArch)
	}
	sort.Strings(platforms)
	return strings.Join(platforms, ", ")
}

func validateRuntimeCommandExists(root, command string) error {
	command = strings.TrimSpace(command)
	if command == "" || command == "${go}" || !strings.ContainsAny(command, `/\`) || filepath.IsAbs(command) {
		return nil
	}
	cleaned, err := cleanPackPath(command)
	if err != nil {
		return err
	}
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(cleaned)))
	if err != nil {
		return fmt.Errorf("command file %q is not available: %w", command, err)
	}
	if info.IsDir() {
		return fmt.Errorf("command file %q is a directory", command)
	}
	return nil
}

func pruneBundleRuntimesForCurrentPlatform(bundleDir string, manifest Manifest) error {
	children, err := scanBundleChildren(bundleDir, SourceUser, manifest, path.Join(DirBundles, manifest.ID), path.Join("/packs", string(SourceUser), DirBundles, manifest.ID), "")
	if err != nil {
		return fmt.Errorf("scan bundle children for runtime pruning: %w", err)
	}
	for _, child := range children {
		if err := prunePluginRuntimeForCurrentPlatform(child); err != nil {
			return err
		}
	}
	return nil
}

func prunePluginRuntimeForCurrentPlatform(pack Pack) error {
	if pack.Type != PluginPack || pack.Runtime.Kind != RuntimeGRPC || len(pack.Runtime.Platforms) == 0 {
		return nil
	}
	selected, ok := pluginRuntimeForCurrentPlatform(pack.Runtime)
	if !ok {
		return fmt.Errorf("plugin %q does not include a runtime for current platform %s/%s", pack.ID, runtime.GOOS, runtime.GOARCH)
	}
	for _, platform := range pack.Runtime.Platforms {
		command := strings.TrimSpace(platform.Command)
		if command == "" || command == selected.Command {
			continue
		}
		if err := removePackCommandFile(pack.RootDir, command); err != nil {
			return fmt.Errorf("plugin %q remove unused runtime %s/%s: %w", pack.ID, platform.GOOS, platform.GOArch, err)
		}
	}
	manifest := pack.Manifest
	manifest.Runtime = selected
	if err := writeManifestFile(filepath.Join(pack.RootDir, "manifest.json"), manifest); err != nil {
		return fmt.Errorf("write pruned plugin manifest %q: %w", pack.ID, err)
	}
	if err := ensureRuntimeCommandExecutable(pack.RootDir, selected.Command); err != nil {
		return fmt.Errorf("plugin %q mark runtime executable: %w", pack.ID, err)
	}
	removeEmptyDirs(filepath.Join(pack.RootDir, "bin"))
	return nil
}

func ensureRuntimeCommandExecutable(root, command string) error {
	command = strings.TrimSpace(command)
	if command == "" || command == "${go}" || !strings.ContainsAny(command, `/\`) || filepath.IsAbs(command) {
		return nil
	}
	cleaned, err := cleanPackPath(command)
	if err != nil {
		return err
	}
	target := filepath.Join(root, filepath.FromSlash(cleaned))
	info, err := os.Stat(target)
	if errors.Is(err, fs.ErrNotExist) || err != nil || info.IsDir() {
		return err
	}
	return os.Chmod(target, info.Mode()|0755)
}

func removePackCommandFile(root, command string) error {
	command = strings.TrimSpace(command)
	if command == "" || command == "${go}" || !strings.ContainsAny(command, `/\`) || filepath.IsAbs(command) {
		return nil
	}
	cleaned, err := cleanPackPath(command)
	if err != nil {
		return err
	}
	target := filepath.Join(root, filepath.FromSlash(cleaned))
	info, err := os.Stat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		return nil
	}
	return os.Remove(target)
}

func removeEmptyDirs(root string) {
	var dirs []string
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.IsDir() {
			return nil
		}
		dirs = append(dirs, path)
		return nil
	}); err != nil {
		return
	}
	sort.Slice(dirs, func(i, j int) bool {
		return len(dirs[i]) > len(dirs[j])
	})
	for _, dir := range dirs {
		_ = os.Remove(dir)
	}
}

func writeManifestFile(path string, manifest Manifest) error {
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(path, body, 0644)
}

func cloneStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	cloned := map[string]string{}
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func mergeStringMap(base, overlay map[string]string) map[string]string {
	if len(overlay) == 0 {
		return base
	}
	merged := cloneStringMap(base)
	if merged == nil {
		merged = map[string]string{}
	}
	for key, value := range overlay {
		merged[key] = value
	}
	return merged
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
	direct, err := scanPacks(officialRoot, userRoot, ThemePack)
	if err != nil {
		return nil, err
	}
	bundled, err := scanBundleChildPacks(officialRoot, userRoot, ThemePack)
	if err != nil {
		return nil, err
	}
	packs, err := dedupeBundledPacks(append(direct, bundled...), ThemePack)
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

// scanBundlePacks 扫描官方目录和用户目录中的顶层 bundle。
//
// 参数：
// - officialRoot: 官方资源根目录。
// - userRoot: 用户上传资源根目录。
//
// 返回值：
// - []Pack: 顶层 bundle 列表，每个 bundle 会记录自身包含的主题 ID 与插件 ID。
func scanBundlePacks(officialRoot, userRoot string) ([]Pack, error) {
	var packs []Pack
	for _, sourceConfig := range []struct {
		source PackSource
		root   string
	}{
		{source: SourceOfficial, root: officialRoot},
		{source: SourceUser, root: userRoot},
	} {
		found, err := scanBundlePackRoot(sourceConfig.root, sourceConfig.source)
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

// scanBundlePackRoot 扫描单个来源目录中的顶层 bundle。
//
// 参数：
// - root: 某个来源的资源根目录，例如 internal/bundles 或 content。
// - source: 资源来源标识，用于生成 URLBase。
//
// 返回值：
// - []Pack: 当前来源下的 bundle 父包；不存在对应目录时返回空列表。
func scanBundlePackRoot(root string, source PackSource) ([]Pack, error) {
	var packs []Pack
	for _, bundleRoot := range bundleScanRoots(root, source) {
		found, err := scanBundlePackDir(bundleRoot, source)
		if err != nil {
			return nil, err
		}
		packs = append(packs, found...)
	}
	return packs, nil
}

// scanBundlePackDir 扫描一个已经定位好的 bundle 父目录。
//
// 参数：
// - bundleRoot: 包含物理目录、相对路径前缀和 URL 前缀的扫描配置。
// - source: 资源来源标识，用于写入 Pack.Source。
//
// 返回值：
// - []Pack: 当前目录下所有顶层 bundle 父包。
// - 目录不存在时返回空列表；manifest 错误或类型不匹配时返回错误。
func scanBundlePackDir(bundleRoot bundleScanRoot, source PackSource) ([]Pack, error) {
	entries, err := os.ReadDir(bundleRoot.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read bundle packs: %w", err)
	}

	var packs []Pack
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		bundleDir := filepath.Join(bundleRoot.dir, entry.Name())
		manifestPath := filepath.Join(bundleDir, "manifest.json")
		manifest, err := readManifestFile(manifestPath)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if err := validateManifest(manifest); err != nil {
			return nil, fmt.Errorf("%s: %w", manifestPath, err)
		}
		if manifest.Type != BundlePack {
			return nil, fmt.Errorf("%s: manifest type %q does not match folder type %q", manifestPath, manifest.Type, BundlePack)
		}

		bundleRelativeDir := path.Join(bundleRoot.relativePrefix, entry.Name())
		bundleURLBase := path.Join(bundleRoot.urlPrefix, entry.Name())
		pack, err := buildPack(bundleDir, source, manifest, bundleRelativeDir, bundleURLBase)
		if err != nil {
			return nil, err
		}
		children, err := scanBundleChildren(bundleDir, source, manifest, bundleRelativeDir, bundleURLBase, "")
		if err != nil {
			return nil, err
		}
		pack.BundledThemeIDs, pack.BundledPluginIDs = bundleChildIDs(children)
		packs = append(packs, pack)
	}
	return packs, nil
}

// scanBundleChildPacks 扫描所有 bundle 内指定类型的子资源包。
//
// 参数：
// - officialRoot: 官方资源根目录。
// - userRoot: 用户上传资源根目录。
// - childType: 要提取的子包类型，当前用于 ThemePack 或 PluginPack。
//
// 返回值：
// - []Pack: 所有匹配类型的 bundle 子包，子包会携带 BundleID 等归属信息。
func scanBundleChildPacks(officialRoot, userRoot string, childType PackType) ([]Pack, error) {
	var packs []Pack
	for _, sourceConfig := range []struct {
		source PackSource
		root   string
	}{
		{source: SourceOfficial, root: officialRoot},
		{source: SourceUser, root: userRoot},
	} {
		found, err := scanBundleChildRoot(sourceConfig.root, sourceConfig.source, childType)
		if err != nil {
			return nil, err
		}
		packs = append(packs, found...)
	}
	return packs, nil
}

// scanBundleChildRoot 扫描单个来源目录中所有 bundle 的指定类型子包。
//
// 参数：
// - root: 某个来源的资源根目录。
// - source: 资源来源标识。
// - childType: 要提取的子包类型。
//
// 返回值：
// - []Pack: 当前来源下所有 bundle 的匹配子包。
func scanBundleChildRoot(root string, source PackSource, childType PackType) ([]Pack, error) {
	var packs []Pack
	for _, bundleRoot := range bundleScanRoots(root, source) {
		found, err := scanBundleChildDir(bundleRoot, source, childType)
		if err != nil {
			return nil, err
		}
		packs = append(packs, found...)
	}
	return packs, nil
}

// scanBundleChildDir 扫描一个 bundle 父目录中的指定类型子包。
//
// 参数：
// - bundleRoot: 包含物理目录、相对路径前缀和 URL 前缀的扫描配置。
// - source: 资源来源标识。
// - childType: 需要返回的子包类型，通常是 ThemePack 或 PluginPack。
//
// 返回值：
// - []Pack: 当前父目录下所有 bundle 中匹配类型的子资源包。
func scanBundleChildDir(bundleRoot bundleScanRoot, source PackSource, childType PackType) ([]Pack, error) {
	entries, err := os.ReadDir(bundleRoot.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read bundle packs: %w", err)
	}

	var packs []Pack
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		bundleDir := filepath.Join(bundleRoot.dir, entry.Name())
		manifestPath := filepath.Join(bundleDir, "manifest.json")
		manifest, err := readManifestFile(manifestPath)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if err := validateManifest(manifest); err != nil {
			return nil, fmt.Errorf("%s: %w", manifestPath, err)
		}
		if manifest.Type != BundlePack {
			return nil, fmt.Errorf("%s: manifest type %q does not match folder type %q", manifestPath, manifest.Type, BundlePack)
		}

		children, err := scanBundleChildren(
			bundleDir,
			source,
			manifest,
			path.Join(bundleRoot.relativePrefix, entry.Name()),
			path.Join(bundleRoot.urlPrefix, entry.Name()),
			childType,
		)
		if err != nil {
			return nil, err
		}
		packs = append(packs, children...)
	}
	return packs, nil
}

// bundleScanRoot 描述一次 bundle 父目录扫描所需的路径上下文。
//
// 字段：
// - dir: 实际文件系统目录。
// - relativePrefix: 写入 Pack.RelativeDir 的逻辑前缀。
// - urlPrefix: 写入 Pack.URLBase 的公开 URL 前缀。
type bundleScanRoot struct {
	dir            string
	relativePrefix string
	urlPrefix      string
}

// bundleScanRoots 返回某个来源需要扫描的 bundle 父目录。
//
// 参数：
// - root: 来源资源根目录。官方来源现在直接使用 internal/bundles；用户来源使用 content。
// - source: 资源来源标识，用于决定扫描布局和生成 URL 前缀。
//
// 返回值：
//   - 官方来源会先扫描 root 本身，兼容新的 internal/bundles/newspaper 结构；
//     同时保留 root/bundles 作为过渡兼容，方便旧测试和旧布局继续被识别。
//   - 用户来源只扫描 root/bundles，也就是 content/bundles，确保用户上传目录以 bundle 为安装单位隔离。
func bundleScanRoots(root string, source PackSource) []bundleScanRoot {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil
	}

	urlPrefix := path.Join("/packs", string(source), DirBundles)
	if source == SourceOfficial {
		return []bundleScanRoot{
			{dir: root, relativePrefix: DirBundles, urlPrefix: urlPrefix},
			{dir: filepath.Join(root, DirBundles), relativePrefix: DirBundles, urlPrefix: urlPrefix},
		}
	}
	return []bundleScanRoot{
		{dir: filepath.Join(root, DirBundles), relativePrefix: DirBundles, urlPrefix: urlPrefix},
	}
}

// scanBundleChildren 读取一个 bundle 内的子资源包。
//
// 参数：
// - bundleDir: 父 bundle 在文件系统中的目录。
// - source: 父 bundle 来源，决定生成 /packs/official 或 /packs/user URL。
// - bundleManifest: 父 bundle 的 manifest，用于读取 packs 显式路径和写入子包归属信息。
// - bundleRelativeDir: 父 bundle 相对资源根目录的位置。
// - bundleURLBase: 父 bundle 对外静态资源 URL 前缀。
// - childType: 可选过滤类型；为空时返回 theme 与 plugin，传入 ThemePack/PluginPack 时只返回该类型。
//
// 返回值：
// - []Pack: 已构建好的子主题包或子插件包；每个子包都会带上 BundleID/BundleName/BundleVersion。
//
// 设计说明：
// 1. bundle 子包只允许 theme 和 plugin，避免在 bundle 中递归嵌套 bundle 导致安装/删除语义不清。
// 2. 显式 packs 路径优先；未声明时自动扫描 themes/* 和 plugins/*，让简单资源包少写配置。
func scanBundleChildren(bundleDir string, source PackSource, bundleManifest Manifest, bundleRelativeDir, bundleURLBase string, childType PackType) ([]Pack, error) {
	childPaths, err := bundleChildPaths(bundleDir, bundleManifest)
	if err != nil {
		return nil, err
	}

	var packs []Pack
	for _, childPath := range childPaths {
		childDir := filepath.Join(bundleDir, filepath.FromSlash(childPath))
		manifestPath := filepath.Join(childDir, "manifest.json")
		manifest, err := readManifestFile(manifestPath)
		if err != nil {
			return nil, err
		}
		if err := validateManifest(manifest); err != nil {
			return nil, fmt.Errorf("%s: %w", manifestPath, err)
		}
		switch manifest.Type {
		case ThemePack, PluginPack:
		default:
			return nil, fmt.Errorf("%s: bundle child type %q is not supported; want %q or %q", manifestPath, manifest.Type, ThemePack, PluginPack)
		}
		if childType != "" && manifest.Type != childType {
			continue
		}

		pack, err := buildPack(childDir, source, manifest, path.Join(bundleRelativeDir, childPath), path.Join(bundleURLBase, childPath))
		if err != nil {
			return nil, err
		}
		pack.BundleID = bundleManifest.ID
		pack.BundleName = bundleManifest.Name
		pack.BundleVersion = bundleManifest.Version
		packs = append(packs, pack)
	}
	return packs, nil
}

// bundleChildPaths 解析一个 bundle 应该扫描哪些子包目录。
//
// 参数：
// - bundleDir: 父 bundle 根目录。
// - manifest: 父 bundle manifest。
//
// 返回值：
// - 如果 manifest.packs 存在，返回其中声明的相对路径。
// - 如果 manifest.packs 为空，自动返回 themes/* 与 plugins/* 下的子目录。
func bundleChildPaths(bundleDir string, manifest Manifest) ([]string, error) {
	if len(manifest.Packs) > 0 {
		paths := make([]string, 0, len(manifest.Packs))
		for index, pack := range manifest.Packs {
			cleaned, err := cleanPackPath(pack.Path)
			if err != nil {
				return nil, fmt.Errorf("packs[%d].path: %w", index, err)
			}
			if cleaned == "" {
				return nil, fmt.Errorf("packs[%d].path is required", index)
			}
			paths = append(paths, cleaned)
		}
		return paths, nil
	}

	var paths []string
	for _, childDirName := range []string{DirThemes, DirPlugins} {
		childDir := filepath.Join(bundleDir, childDirName)
		entries, err := os.ReadDir(childDir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read bundle %s: %w", childDirName, err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				paths = append(paths, path.Join(childDirName, entry.Name()))
			}
		}
	}
	sort.Strings(paths)
	return paths, nil
}

// bundleChildIDs 从子包列表中拆出主题 ID 和插件 ID。
//
// 参数：
// - children: 已扫描出的 bundle 子包。
//
// 返回值：
// - 第一个返回值是排序后的主题 ID 列表。
// - 第二个返回值是排序后的插件 ID 列表。
func bundleChildIDs(children []Pack) ([]string, []string) {
	themeIDs := make([]string, 0, len(children))
	pluginIDs := make([]string, 0, len(children))
	for _, child := range children {
		switch child.Type {
		case ThemePack:
			themeIDs = append(themeIDs, child.ID)
		case PluginPack:
			pluginIDs = append(pluginIDs, child.ID)
		}
	}
	sort.Strings(themeIDs)
	sort.Strings(pluginIDs)
	return themeIDs, pluginIDs
}

// dedupeBundledPacks 处理同一来源下“旧独立包”和“新 bundle 子包”的迁移重名。
//
// 参数：
// - packs: 直接扫描和 bundle 子包扫描合并后的资源包列表。
// - packType: 当前处理的类型，用于生成更明确的错误信息。
//
// 返回值：
// - 同一来源下独立包和 bundle 子包 ID 相同时，优先保留 bundle 子包。
// - 其他重复 ID 会返回错误，避免运行时选择产生歧义。
func dedupeBundledPacks(packs []Pack, packType PackType) ([]Pack, error) {
	byID := map[string]Pack{}
	order := make([]string, 0, len(packs))
	for _, pack := range packs {
		if existing, ok := byID[pack.ID]; ok {
			if shouldReplaceWithBundledPack(existing, pack) {
				byID[pack.ID] = pack
				continue
			}
			if shouldReplaceWithBundledPack(pack, existing) {
				continue
			}
			return nil, fmt.Errorf("duplicate %s pack id %q", packType, pack.ID)
		}
		byID[pack.ID] = pack
		order = append(order, pack.ID)
	}

	deduped := make([]Pack, 0, len(byID))
	for _, id := range order {
		deduped = append(deduped, byID[id])
	}
	return deduped, nil
}

// shouldReplaceWithBundledPack 判断候选包是否应该替换已有独立包。
//
// 参数：
// - existing: 当前按 ID 缓存的资源包。
// - candidate: 新扫描到的同 ID 资源包。
//
// 返回值：
// - 同一来源下，已有包是独立包且候选包来自 bundle 时返回 true。
func shouldReplaceWithBundledPack(existing, candidate Pack) bool {
	return existing.Source == candidate.Source && existing.BundleID == "" && candidate.BundleID != ""
}

func scanPluginPacks(officialRoot, userRoot string) ([]Pack, error) {
	direct, err := scanPacks(officialRoot, userRoot, PluginPack)
	if err != nil {
		return nil, err
	}
	bundled, err := scanBundleChildPacks(officialRoot, userRoot, PluginPack)
	if err != nil {
		return nil, err
	}
	packs, err := dedupeBundledPacks(append(direct, bundled...), PluginPack)
	if err != nil {
		return nil, err
	}

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
		if err := json.Unmarshal(stripUTF8BOM(body), &manifest); err != nil {
			return nil, fmt.Errorf("parse manifest %s: %w", manifestPath, err)
		}
		if err := validateManifest(manifest); err != nil {
			return nil, fmt.Errorf("%s: %w", manifestPath, err)
		}
		if manifest.Type != packType {
			return nil, fmt.Errorf("%s: manifest type %q does not match folder type %q", manifestPath, manifest.Type, packType)
		}

		pack, err := buildPack(packDir, source, manifest, path.Join(typeDirName(packType), entry.Name()), path.Join("/packs", string(source), typeDirName(packType), entry.Name()))
		if err != nil {
			return nil, err
		}
		packs = append(packs, pack)
	}
	return packs, nil
}

// buildPack 把已经校验过 manifest 的目录转换成运行时 Pack。
//
// 参数：
// - packDir: 资源包在文件系统中的根目录。
// - source: 资源包来源，官方或用户。
// - manifest: manifest.json 解析结果。
// - relativeDir: 相对资源根目录的位置，用于设置页展示和调试。
// - urlBase: 静态资源 URL 前缀。
//
// 返回值：
// - Pack: 包含样式 URL、模板目录、语言列表和翻译表的运行时对象。
func buildPack(packDir string, source PackSource, manifest Manifest, relativeDir, urlBase string) (Pack, error) {
	translations, locales, defaultLocale, err := loadPackTranslations(packDir, manifest)
	if err != nil {
		return Pack{}, err
	}

	pack := Pack{
		Manifest:     manifest,
		Source:       source,
		RootDir:      packDir,
		RelativeDir:  relativeDir,
		URLBase:      urlBase,
		Locales:      locales,
		Translations: translations,
	}
	pack.SourceURL = strings.TrimSpace(manifest.SourceURL)
	pack.DefaultLocale = defaultLocale
	pack.TemplateDir = resolveExistingDir(packDir, manifest.TemplatesDir, "templates")
	pack.StyleURLs = resolveStyleURLs(pack, manifest.Styles)
	pack.BadgeKeys = packBadges(pack)
	return pack, nil
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

// localizedPackMessages 返回某个资源包在指定语言下的完整文案快照。
//
// 参数：
// - pack: 已扫描完成的主题包或插件包。
// - locale: 调用方希望使用的语言代码，例如 en 或 zh-CN。
//
// 返回值：
// - map[string]string: 先包含资源包默认语言文案，再叠加指定语言文案后的新 map。
//
// 设计说明：
// 1. 主题包的默认语言承担“完整文案基线”的角色。
// 2. 指定语言只需要覆盖与默认语言不同的 key，缺失项自然回退到默认语言。
// 3. 返回新 map 可以避免后续 mergeMessages 修改 Pack.Translations 中的原始缓存。
func localizedPackMessages(pack Pack, locale string) map[string]string {
	defaultLocale := effectiveDefaultLocale(pack)
	messages := cloneMessages(pack.Translations[defaultLocale])
	if strings.TrimSpace(locale) != "" && locale != defaultLocale {
		mergeMessages(messages, pack.Translations[locale])
	}
	return messages
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

// themesWithActivePluginLocales 为每个主题计算已启用语言插件可提供的额外语言。
//
// 参数：
// - defaultTheme: 系统默认主题，用于补齐所有主题共同依赖的基础文案 key。
// - themes: 当前扫描到的全部主题包。
// - activePlugins: 当前已加入排序并启用的插件包。
//
// 返回值：
// - []Pack: 每个主题的 Locales 都追加了“完整匹配该主题”的插件语言。
//
// 设计说明：
// 语言选择列表是跟着用户当前点选的主题变化的，而不是只跟着已经应用的主题变化。
// 因此每个主题卡片都需要提前带上自己的可选语言集合，前端切换主题卡片时直接读取
// data-locales 就能显示正确结果。
func themesWithActivePluginLocales(defaultTheme Pack, themes []Pack, activePlugins []Pack) []Pack {
	mergedThemes := make([]Pack, 0, len(themes))
	for _, theme := range themes {
		requiredMessages := requiredThemeMessages(defaultTheme, theme)
		theme.Locales = mergeActiveLocaleCodes(theme.Locales, activePlugins, requiredMessages)
		mergedThemes = append(mergedThemes, theme)
	}
	return mergedThemes
}

// requiredThemeMessages 计算当前主题需要完整翻译的文案全集。
//
// 参数：
// - defaultTheme: 系统默认主题，承担全站基础文案基线。
// - activeTheme: 当前实际启用的主题，可能只覆盖部分默认文案，也可能新增主题专属文案。
//
// 返回值：
// - map[string]string: 当前主题渲染时需要的全部 key；value 只用于判断 key 是否存在。
//
// 设计说明：
// 语言包作为插件提供时，不能只翻译自己关心的一两个 key 就出现在语言选择里。
// 这里沿用运行时消息合并规则：先取默认主题默认语言的完整文案，再叠加当前主题默认语言
// 的覆盖文案。这样 Pure White 这类“基于默认文案 + 少量主题文案”的主题，会要求语言包
// 同时覆盖默认文案和主题新增文案，避免切换后界面半中文半目标语言。
func requiredThemeMessages(defaultTheme, activeTheme Pack) map[string]string {
	messages := localizedPackMessages(defaultTheme, effectiveDefaultLocale(defaultTheme))
	if activeTheme.ID != defaultTheme.ID {
		mergeMessages(messages, localizedPackMessages(activeTheme, effectiveDefaultLocale(activeTheme)))
	}
	return messages
}

// mergeActiveLocaleCodes 合并当前主题与已启用且完整匹配的语言插件提供的语言代码。
//
// 参数：
// - themeLocales: 当前主题自身声明的语言代码列表，例如 en、zh-CN。
// - activePlugins: 当前已经启用并会参与文案覆盖的插件包。
// - requiredMessages: 当前主题需要被完整翻译的文案 key 集合。
//
// 返回值：
// - []string: 去重后的语言代码列表，保留主题语言在前，并追加插件语言。
//
// 设计说明：
// 翻译包作为 plugin 发布时，主题本身可能并不知道 zh-TW 或 ja。
// 但只有当插件某个语言完整覆盖当前主题全部 key 时，才把它加入语言下拉；否则用户会选到
// 一个不完整的语言，页面出现部分回退文案。
func mergeActiveLocaleCodes(themeLocales []string, activePlugins []Pack, requiredMessages map[string]string) []string {
	seen := map[string]bool{}
	merged := make([]string, 0, len(themeLocales))
	appendLocale := func(localeCode string) {
		localeCode = strings.TrimSpace(localeCode)
		if localeCode == "" || seen[localeCode] {
			return
		}
		seen[localeCode] = true
		merged = append(merged, localeCode)
	}
	for _, localeCode := range themeLocales {
		appendLocale(localeCode)
	}
	for _, plugin := range activePlugins {
		for _, localeCode := range plugin.Locales {
			if !pluginLocaleCoversMessages(plugin, localeCode, requiredMessages) {
				continue
			}
			appendLocale(localeCode)
		}
	}
	return merged
}

// pluginLocaleCoversMessages 判断插件某个语言是否完整覆盖当前主题所需文案。
//
// 参数：
// - plugin: 已扫描出的插件包。
// - localeCode: 要检查的语言代码，例如 ja 或 zh-TW。
// - requiredMessages: 当前主题需要完整翻译的 key 集合。
//
// 返回值：
// - bool: 插件在该语言下包含 requiredMessages 的全部 key，且每个值非空时返回 true。
//
// 注意：
// 这里要求“完整覆盖”，而不是依赖默认主题兜底。这样语言选择列表只展示真正可用的语言包。
func pluginLocaleCoversMessages(plugin Pack, localeCode string, requiredMessages map[string]string) bool {
	localeCode = strings.TrimSpace(localeCode)
	if localeCode == "" || len(requiredMessages) == 0 {
		return false
	}
	messages := plugin.Translations[localeCode]
	if len(messages) == 0 {
		return false
	}
	for key := range requiredMessages {
		if strings.TrimSpace(messages[key]) == "" {
			return false
		}
	}
	return true
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
		case "retro":
			badges = append(badges, "badge.retro")
		case "minimal":
			badges = append(badges, "badge.minimal")
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
	case "minimal":
		return 2
	case "other":
		return 3
	case "retro":
		return 4
	default:
		return 5
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
	case ThemePack, BundlePack, PluginPack, LegacyTextPack:
	default:
		return fmt.Errorf("invalid pack type %q", manifest.Type)
	}
	if !validPackID(manifest.ID) {
		return fmt.Errorf("invalid pack id %q", manifest.ID)
	}
	if strings.TrimSpace(manifest.Name) == "" {
		return errors.New("pack name is required")
	}
	if err := validatePluginRuntime(manifest); err != nil {
		return err
	}
	if err := validateManifestSourceURL(manifest); err != nil {
		return err
	}
	if err := validateServiceDeclarations("services", manifest.Services, false); err != nil {
		return err
	}
	if err := validateManifestRequirements(manifest); err != nil {
		return err
	}
	for index, location := range manifest.MenuLocations {
		if !validPackID(location.ID) {
			return fmt.Errorf("menu_locations[%d].id: invalid menu location id %q", index, location.ID)
		}
	}
	return validateManifestPaths(manifest)
}

func validatePluginRuntime(manifest Manifest) error {
	switch manifest.Runtime.Kind {
	case RuntimeStatic:
		return nil
	case RuntimeGRPC:
		if manifest.Type != PluginPack {
			return errors.New("runtime is only supported on plugin packs")
		}
		if strings.TrimSpace(manifest.Runtime.Command) == "" && len(manifest.Runtime.Platforms) == 0 {
			return errors.New("runtime.command or runtime.platforms is required for grpc plugins")
		}
		seen := map[string]bool{}
		for index, platform := range manifest.Runtime.Platforms {
			if !validPlatformToken(platform.GOOS) {
				return fmt.Errorf("runtime.platforms[%d].goos: invalid platform value %q", index, platform.GOOS)
			}
			if !validPlatformToken(platform.GOArch) {
				return fmt.Errorf("runtime.platforms[%d].goarch: invalid platform value %q", index, platform.GOArch)
			}
			if strings.TrimSpace(platform.Command) == "" {
				return fmt.Errorf("runtime.platforms[%d].command is required", index)
			}
			key := platform.GOOS + "/" + platform.GOArch
			if seen[key] {
				return fmt.Errorf("runtime.platforms[%d]: duplicate platform %q", index, key)
			}
			seen[key] = true
		}
		return nil
	default:
		return fmt.Errorf("invalid runtime kind %q", manifest.Runtime.Kind)
	}
}

func validateManifestRequirements(manifest Manifest) error {
	if manifest.Requires == nil {
		return nil
	}
	if requiredVersion := strings.TrimSpace(manifest.Requires.Postizer); requiredVersion != "" {
		if _, ok := parseReleaseVersion(requiredVersion); !ok {
			return fmt.Errorf("requires.postizer must match vX.Y.Z, got %q", requiredVersion)
		}
	}
	return validateServiceDeclarations("requires.host_services", manifest.Requires.HostServices, true)
}

func validateServiceDeclarations(name string, services []PluginService, requireMethods bool) error {
	for index, service := range services {
		if strings.TrimSpace(service.Name) == "" {
			return fmt.Errorf("%s[%d].name is required", name, index)
		}
		if requireMethods && len(service.Methods) == 0 {
			return fmt.Errorf("%s[%d].methods is required", name, index)
		}
		for methodIndex, method := range service.Methods {
			if strings.TrimSpace(method) == "" {
				return fmt.Errorf("%s[%d].methods[%d] is required", name, index, methodIndex)
			}
		}
	}
	return nil
}

// validateManifestSourceURL 校验 bundle 的来源链接字段。
//
// 参数：
// - manifest: 已解析但尚未完全信任的资源包 manifest。
//
// 返回值：
// - source_url 为空时返回 nil，表示不设置来源链接。
// - source_url 非空时只允许出现在 bundle manifest 上，并且必须是 https://github.com/... 链接。
//
// 设计说明：
// 这个字段用于让用户追溯第三方资源包的来源。当前先收窄到 GitHub，避免把后台变成
// 任意外链展示区；官方 bundle 可以省略该字段。
func validateManifestSourceURL(manifest Manifest) error {
	sourceURL := strings.TrimSpace(manifest.SourceURL)
	if sourceURL == "" {
		return nil
	}
	if manifest.Type != BundlePack {
		return errors.New("source_url is only supported on bundle packs")
	}

	parsed, err := url.Parse(sourceURL)
	if err != nil {
		return fmt.Errorf("source_url must be a valid GitHub URL: %w", err)
	}
	if parsed.Scheme != "https" || parsed.User != nil || !isGitHubHost(parsed.Hostname()) || strings.Trim(parsed.Path, "/") == "" {
		return errors.New("source_url must be an https://github.com/... URL")
	}
	return nil
}

// isGitHubHost 判断 URL hostname 是否属于当前允许的 GitHub 页面域名。
//
// 参数：
// - host: url.URL.Hostname() 返回的主机名，不包含端口。
//
// 返回值：
// - host 是 github.com 或 www.github.com 时返回 true；其他域名返回 false。
func isGitHubHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "github.com", "www.github.com":
		return true
	default:
		return false
	}
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

func validPlatformToken(value string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return false
	}
	return true
}

func compareReleaseVersions(left, right string) (int, bool) {
	leftParts, ok := parseReleaseVersion(left)
	if !ok {
		return 0, false
	}
	rightParts, ok := parseReleaseVersion(right)
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

func parseReleaseVersion(value string) ([3]int, bool) {
	var parts [3]int
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	tokens := strings.Split(value, ".")
	if len(tokens) != len(parts) {
		return parts, false
	}
	for index, token := range tokens {
		if token == "" {
			return parts, false
		}
		n := 0
		for _, r := range token {
			if r < '0' || r > '9' {
				return parts, false
			}
			n = n*10 + int(r-'0')
		}
		parts[index] = n
	}
	return parts, true
}

func validateManifestPaths(manifest Manifest) error {
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "templates_dir", value: manifest.TemplatesDir},
		{name: "translations_dir", value: manifest.TranslationsDir},
		{name: "messages_file", value: manifest.MessagesFile},
		{name: "runtime.work_dir", value: manifest.Runtime.WorkDir},
	} {
		if _, err := cleanPackPath(field.value); err != nil {
			return fmt.Errorf("%s: %w", field.name, err)
		}
	}
	if err := validateRuntimeCommandPath("runtime.command", manifest.Runtime.Command); err != nil {
		return err
	}
	for index, platform := range manifest.Runtime.Platforms {
		if err := validateRuntimeCommandPath(fmt.Sprintf("runtime.platforms[%d].command", index), platform.Command); err != nil {
			return err
		}
		if _, err := cleanPackPath(platform.WorkDir); err != nil {
			return fmt.Errorf("runtime.platforms[%d].work_dir: %w", index, err)
		}
	}
	for index, style := range manifest.Styles {
		if _, err := cleanPackPath(style); err != nil {
			return fmt.Errorf("styles[%d]: %w", index, err)
		}
	}
	for index, pack := range manifest.Packs {
		if _, err := cleanPackPath(pack.Path); err != nil {
			return fmt.Errorf("packs[%d].path: %w", index, err)
		}
	}
	for index, entry := range manifest.UIEntries {
		if !validPackID(entry.ID) {
			return fmt.Errorf("ui_entries[%d].id: invalid ui entry id %q", index, entry.ID)
		}
		if strings.TrimSpace(entry.Outlet) == "" {
			return fmt.Errorf("ui_entries[%d].outlet is required", index)
		}
		if strings.TrimSpace(entry.Path) == "" {
			return fmt.Errorf("ui_entries[%d].path is required", index)
		}
		if _, err := cleanPackPath(entry.Path); err != nil {
			return fmt.Errorf("ui_entries[%d].path: %w", index, err)
		}
	}
	return nil
}

func validateRuntimeCommandPath(name, command string) error {
	command = strings.TrimSpace(command)
	if command == "" || command == "${go}" {
		return nil
	}
	if !strings.ContainsAny(command, `/\`) {
		return nil
	}
	if _, err := cleanPackPath(command); err != nil {
		return fmt.Errorf("%s: %w", name, err)
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
	case BundlePack:
		return DirBundles
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
		styleURL := path.Join(pack.URLBase, cleaned)
		if version := strings.TrimSpace(pack.Version); version != "" {
			styleURL += "?v=" + url.QueryEscape(version)
		}
		urls = append(urls, styleURL)
	}
	return urls
}

func readMessagesFile(path string) (map[string]string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read messages %s: %w", path, err)
	}
	var messages map[string]string
	if err := json.Unmarshal(stripUTF8BOM(body), &messages); err != nil {
		return nil, fmt.Errorf("parse messages %s: %w", path, err)
	}
	if messages == nil {
		return map[string]string{}, nil
	}
	return messages, nil
}

func readManifestFile(path string) (Manifest, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest %s: %w", path, err)
	}
	var manifest Manifest
	if err := json.Unmarshal(stripUTF8BOM(body), &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse manifest %s: %w", path, err)
	}
	return manifest, nil
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
		dir := path.Dir(name)
		if dir != "." && strings.Contains(dir, "/") {
			continue
		}
		if manifestFile != nil {
			return nil, "", errors.New("zip must contain exactly one top-level manifest.json")
		}
		manifestFile = file
	}
	if manifestFile == nil {
		return nil, "", errors.New("zip does not contain top-level manifest.json")
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

	body, err := io.ReadAll(reader)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(stripUTF8BOM(body), &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse manifest: %w", err)
	}
	return manifest, nil
}

func stripUTF8BOM(body []byte) []byte {
	return bytes.TrimPrefix(body, []byte{0xEF, 0xBB, 0xBF})
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
