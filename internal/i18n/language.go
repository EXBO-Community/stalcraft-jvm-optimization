// Package i18n stores the UI language preference and loads TUI translations.
package i18n

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

type Language string

const (
	English Language = "en"
	Russian Language = "ru"
)

type Key string

const (
	LanguageName Key = "language.name"

	MainInstallLabel       Key = "main.install.label"
	MainInstallDetail      Key = "main.install.detail"
	MainUninstallLabel     Key = "main.uninstall.label"
	MainUninstallDetail    Key = "main.uninstall.detail"
	MainStatusLabel        Key = "main.status.label"
	MainStatusDetail       Key = "main.status.detail"
	MainSelectConfigLabel  Key = "main.select_config.label"
	MainSelectConfigDetail Key = "main.select_config.detail"
	MainRegenerateLabel    Key = "main.regenerate.label"
	MainRegenerateDetail   Key = "main.regenerate.detail"
	MainTunnelLabel        Key = "main.tunnel.label"
	MainTunnelDetail       Key = "main.tunnel.detail"
	MainLanguageLabel      Key = "main.language.label"
	MainLanguageDetail     Key = "main.language.detail"
	MainExitLabel          Key = "main.exit.label"
	MainExitDetail         Key = "main.exit.detail"

	StartupInstallDetail Key = "startup.install.detail"
	StartupIgnoreLabel   Key = "startup.ignore.label"
	StartupIgnoreDetail  Key = "startup.ignore.detail"

	LanguageEnglishDetail Key = "language.english.detail"
	LanguageRussianDetail Key = "language.russian.detail"

	BackLabel          Key = "back.label"
	BackPrevious       Key = "back.previous"
	BackMain           Key = "back.main"
	FolderDetail       Key = "folder.detail"
	ProfileLabel       Key = "profile.label"
	ProfileUnset       Key = "profile.unset"
	FooterHelp         Key = "footer.help"
	FooterHelpShort    Key = "footer.help.short"
	TitleSetup         Key = "title.setup"
	TitleConfigs       Key = "title.configs"
	TitleReleases      Key = "title.releases"
	TitleStatus        Key = "title.status"
	TitleLanguage      Key = "title.language"
	TitleMain          Key = "title.main"
	TitleTunnel        Key = "title.tunnel"
	TitleTunnelPools   Key = "title.tunnel.pools"
	TitleTunnelNodes   Key = "title.tunnel.nodes"
	TitleTunnelSearch  Key = "title.tunnel.search"
	TitleTunnelMode    Key = "title.tunnel.mode"
	TitleTunnelExclude Key = "title.tunnel.exclude"
	TitleTunnelResults Key = "title.tunnel.results"
	NoConfigs          Key = "body.no_configs"
	LanguageBody       Key = "body.language"
	DetectedSystem     Key = "body.detected"
	MemoryLowWarning   Key = "body.memory.low"
	Memory16Note       Key = "body.memory.16"

	TunnelBodyRegions            Key = "tunnel.body.regions"
	TunnelActiveDefault          Key = "tunnel.active.default"
	TunnelActiveOverride         Key = "tunnel.active.override"
	TunnelGameDefaultLabel       Key = "tunnel.default.label"
	TunnelGameDefaultDetail      Key = "tunnel.default.detail"
	TunnelRegionDetail           Key = "tunnel.region.detail"
	TunnelRegionOverrideDetail   Key = "tunnel.region.override_detail"
	TunnelLoading                Key = "tunnel.loading"
	TunnelLoadFailed             Key = "tunnel.load_failed"
	TunnelCatalogSummary         Key = "tunnel.catalog.summary"
	TunnelRetryLabel             Key = "tunnel.retry.label"
	TunnelRetryDetail            Key = "tunnel.retry.detail"
	TunnelSearchLabel            Key = "tunnel.search.label"
	TunnelSearchDetail           Key = "tunnel.search.detail"
	TunnelPoolDetail             Key = "tunnel.pool.detail"
	TunnelPoolExcludedDetail     Key = "tunnel.pool.excluded_detail"
	TunnelEndpointChecking       Key = "tunnel.endpoint.checking"
	TunnelEndpointCheckingLast   Key = "tunnel.endpoint.checking_last_limit"
	TunnelEndpointNoResponse     Key = "tunnel.endpoint.no_response"
	TunnelEndpointNoResponseLast Key = "tunnel.endpoint.no_response_last_limit"
	TunnelEndpointLimited        Key = "tunnel.endpoint.limited"
	TunnelEndpointCached         Key = "tunnel.endpoint.cached"
	TunnelEndpointReady          Key = "tunnel.endpoint.ready"
	TunnelEndpointDetail         Key = "tunnel.endpoint.detail"
	TunnelEndpointHistory        Key = "tunnel.endpoint.history"
	TunnelModeLabel              Key = "tunnel.mode.label"
	TunnelModeDetail             Key = "tunnel.mode.detail"
	TunnelExclusionsLabel        Key = "tunnel.exclusions.label"
	TunnelExclusionsDetail       Key = "tunnel.exclusions.detail"
	TunnelRunSearchLabel         Key = "tunnel.run.label"
	TunnelRunSearchDetail        Key = "tunnel.run.detail"
	TunnelRerunLabel             Key = "tunnel.rerun.label"
	TunnelRerunDetail            Key = "tunnel.rerun.detail"
	TunnelUseBestLabel           Key = "tunnel.use_best.label"
	TunnelUseBestDetail          Key = "tunnel.use_best.detail"
	TunnelClearExclusionsLabel   Key = "tunnel.exclusions.clear.label"
	TunnelClearExclusionsDetail  Key = "tunnel.exclusions.clear.detail"
	TunnelExclusionPoolDetail    Key = "tunnel.exclusions.pool.detail"
	TunnelNoTargets              Key = "tunnel.no_targets"
	TunnelNoResults              Key = "tunnel.no_results"
	TunnelProgress               Key = "tunnel.progress"
	TunnelComplete               Key = "tunnel.complete"
	TunnelResultDetail           Key = "tunnel.result.detail"
	TunnelModePingLabel          Key = "tunnel.mode.ping.label"
	TunnelModePingDetail         Key = "tunnel.mode.ping.detail"
	TunnelModeClientLabel        Key = "tunnel.mode.client.label"
	TunnelModeClientDetail       Key = "tunnel.mode.client.detail"
	TunnelModeServerLabel        Key = "tunnel.mode.server.label"
	TunnelModeServerDetail       Key = "tunnel.mode.server.detail"
	TunnelModeStabilityLabel     Key = "tunnel.mode.stability.label"
	TunnelModeStabilityDetail    Key = "tunnel.mode.stability.detail"
	TunnelModeGameLabel          Key = "tunnel.mode.game.label"
	TunnelModeGameDetail         Key = "tunnel.mode.game.detail"

	NoticeRequestAdmin       Key = "notice.request_admin"
	NoticeRemovingHook       Key = "notice.removing_hook"
	NoticeActiveConfig       Key = "notice.active_config"
	NoticeRegenerating       Key = "notice.regenerating"
	NoticeInstallFailed      Key = "notice.install_failed"
	NoticeInstallDone        Key = "notice.install_done"
	NoticeUninstallDone      Key = "notice.uninstall_done"
	NoticeRegenerated        Key = "notice.regenerated"
	NoticeLanguageSelected   Key = "notice.language_selected"
	NoticeTunnelDefault      Key = "notice.tunnel.default"
	NoticeTunnelSelected     Key = "notice.tunnel.selected"
	NoticeTunnelMode         Key = "notice.tunnel.mode"
	NoticeTunnelSettingsLoad Key = "notice.tunnel.settings_load"
	NoticeTunnelSettings     Key = "notice.tunnel.settings"
	NoticeTunnelCache        Key = "notice.tunnel.cache"

	StatusInstalled    Key = "status.installed"
	StatusNotInstalled Key = "status.not_installed"
	StatusNone         Key = "status.none"

	WarnHookOther    Key = "warn.hook_other"
	WarnServiceCheck Key = "warn.service_check"
	WarnServiceMiss  Key = "warn.service_missing"
)

const (
	DirName        = "langs"
	filePattern    = "active.*.toml"
	fallbackFile   = "active.en.toml"
	releaseKeyPref = "release."
)

type Catalog struct {
	bundle    *goi18n.Bundle
	languages []Language
}

func Current() Language {
	if lang, ok := Saved(); ok {
		return lang
	}
	return Default()
}

func Normalize(value string) (Language, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}

	tag, err := language.Parse(value)
	if err != nil {
		return "", false
	}
	return normalizeTag(tag)
}

func Load() (*Catalog, error) {
	dir, err := findDir()
	if err != nil {
		return nil, err
	}
	return LoadDir(dir)
}

func LoadDir(dir string) (*Catalog, error) {
	paths, err := languageFiles(dir)
	if err != nil {
		return nil, err
	}

	bundle := goi18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	for _, path := range paths {
		if _, err := bundle.LoadMessageFile(path); err != nil {
			return nil, fmt.Errorf("load language file %s: %w", path, err)
		}
	}

	languages := bundleLanguages(bundle)
	if len(languages) == 0 {
		return nil, fmt.Errorf("no languages loaded from %s", dir)
	}
	return &Catalog{
		bundle:    bundle,
		languages: languages,
	}, nil
}

func (c *Catalog) Available() []Language {
	out := make([]Language, len(c.languages))
	copy(out, c.languages)
	return out
}

func (c *Catalog) Has(lang Language) bool {
	lang, ok := Normalize(string(lang))
	if !ok {
		return false
	}
	for _, candidate := range c.languages {
		if candidate == lang {
			return true
		}
	}
	return false
}

func (c *Catalog) Resolve(lang Language) Language {
	if c.Has(lang) {
		return lang
	}
	if c.Has(English) {
		return English
	}
	if len(c.languages) > 0 {
		return c.languages[0]
	}
	return English
}

func (c *Catalog) DisplayName(lang Language) string {
	if text, ok := c.localize(lang, LanguageName, nil); ok {
		return text
	}
	return string(lang)
}

func (c *Catalog) T(lang Language, key Key, args ...any) string {
	if text, ok := c.localize(lang, key, templateData(args)); ok {
		return text
	}
	return string(key)
}

func (c *Catalog) ReleaseDescription(lang Language, version string, fallback string) string {
	key := Key(releaseKeyPref + version)
	if text, ok := c.localize(lang, key, nil); ok {
		return text
	}
	return fallback
}

func (c *Catalog) localize(lang Language, key Key, data map[string]any) (string, bool) {
	if c == nil || c.bundle == nil {
		return "", false
	}
	lang = c.Resolve(lang)

	tags := []string{string(lang)}
	if lang != English {
		tags = append(tags, string(English))
	}
	localizer := goi18n.NewLocalizer(c.bundle, tags...)
	text, err := localizer.Localize(&goi18n.LocalizeConfig{
		MessageID:    string(key),
		TemplateData: data,
	})
	if err != nil {
		return "", false
	}
	return text, true
}

func templateData(args []any) map[string]any {
	if len(args) == 0 {
		return nil
	}

	data := make(map[string]any, len(args))
	for i, arg := range args {
		data[fmt.Sprintf("Arg%d", i)] = arg
	}
	return data
}

func findDir() (string, error) {
	for _, dir := range dirCandidates() {
		if _, err := languageFiles(dir); err == nil {
			return dir, nil
		}
	}
	return "", fmt.Errorf("language files not found; expected %s next to cli.exe", DirName)
}

func dirCandidates() []string {
	candidates := make([]string, 0, 2)
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), DirName))
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, DirName))
	}

	out := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		clean := filepath.Clean(candidate)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out
}

func languageFiles(dir string) ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(dir, filePattern))
	if err != nil {
		return nil, fmt.Errorf("scan language files: %w", err)
	}
	if len(paths) == 0 {
		if _, statErr := os.Stat(dir); errors.Is(statErr, os.ErrNotExist) {
			return nil, fmt.Errorf("language dir not found: %s", dir)
		}
		return nil, fmt.Errorf("no %s files in %s", filePattern, dir)
	}

	sort.Strings(paths)
	fallback := filepath.Join(dir, fallbackFile)
	fallbackIndex := -1
	for i, path := range paths {
		if filepath.Clean(path) == filepath.Clean(fallback) {
			fallbackIndex = i
			break
		}
	}
	if fallbackIndex < 0 {
		return nil, fmt.Errorf("fallback language file missing: %s", fallback)
	}
	if fallbackIndex > 0 {
		paths = append([]string{paths[fallbackIndex]}, append(paths[:fallbackIndex], paths[fallbackIndex+1:]...)...)
	}
	return paths, nil
}

func bundleLanguages(bundle *goi18n.Bundle) []Language {
	tags := bundle.LanguageTags()
	out := make([]Language, 0, len(tags))
	seen := make(map[Language]struct{}, len(tags))
	for _, tag := range tags {
		lang, ok := normalizeTag(tag)
		if !ok {
			continue
		}
		if _, ok := seen[lang]; ok {
			continue
		}
		seen[lang] = struct{}{}
		out = append(out, lang)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i] == English {
			return true
		}
		if out[j] == English {
			return false
		}
		return out[i] < out[j]
	})
	return out
}

func normalizeTag(tag language.Tag) (Language, bool) {
	base, confidence := tag.Base()
	if confidence == language.No {
		return "", false
	}
	code := base.String()
	if code == "" || code == "und" {
		return "", false
	}
	return Language(code), true
}
