// Package ui renders the interactive menu shown when the wrapper is launched
// without CLI arguments.
package ui

import (
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/buildinfo"
	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/config"
	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/elevate"
	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/i18n"
	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/installer"
	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/profile"
	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/sysinfo"
)

type screen int

const (
	screenMain screen = iota
	screenStartupWarning
	screenConfigs
	screenReleases
	screenStatus
	screenLanguage
)

type noticeKind int

const (
	noticeInfo noticeKind = iota
	noticeSuccess
	noticeWarning
	noticeError
)

const transientNoticeTTL = 5 * time.Second

type menuItem struct {
	label  string
	detail string
	active bool
	run    func(*menuModel) tea.Cmd
}

type menuModel struct {
	sys            sysinfo.Info
	screen         screen
	selected       int
	width          int
	height         int
	catalog        *i18n.Catalog
	lang           i18n.Language
	configPrefix   string
	configEntries  []config.Entry
	statusEntries  []installer.Entry
	startupWarning string
	notice         string
	noticeKind     noticeKind
	noticeID       int
}

type actionResultMsg struct {
	kind noticeKind
	text string
}

type clearNoticeMsg struct {
	id int
}

// Run displays the top-level menu until the user chooses Exit.
func Run() error {
	sys := sysinfo.Detect()
	if err := profile.Ensure(sys); err != nil {
		return err
	}
	catalog, err := i18n.Load()
	if err != nil {
		return err
	}

	prepareTerminal()
	lang := catalog.Resolve(i18n.Current())
	model := newMenuModel(sys, catalog, lang)
	if warning := startupWarning(catalog, lang); warning != "" {
		model.screen = screenStartupWarning
		model.startupWarning = warning
	}

	_, err = tea.NewProgram(model, tea.WithAltScreen()).Run()
	return err
}

func newMenuModel(sys sysinfo.Info, catalog *i18n.Catalog, lang i18n.Language) menuModel {
	return menuModel{
		sys:     sys,
		catalog: catalog,
		lang:    lang,
	}
}

func (m menuModel) Init() tea.Cmd {
	return nil
}

func (m menuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case actionResultMsg:
		return m, m.setNotice(msg.kind, msg.text)
	case clearNoticeMsg:
		if m.noticeID == msg.id {
			m.clearNotice()
		}
		return m, nil
	default:
		return m, nil
	}
}

func (m menuModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "esc", "backspace", "left":
		m.goBack()
		return m, nil
	case "up", "k":
		if m.selected > 0 {
			m.selected--
		}
		return m, nil
	case "down", "j":
		if max := len(m.items()) - 1; m.selected < max {
			m.selected++
		}
		return m, nil
	case "enter", " ":
		items := m.items()
		if len(items) == 0 || m.selected >= len(items) {
			return m, nil
		}
		cmd := items[m.selected].run(&m)
		return m, cmd
	default:
		return m, nil
	}
}

func (m *menuModel) goBack() {
	switch m.screen {
	case screenStartupWarning:
		m.ignoreStartupWarning()
	case screenMain:
		return
	case screenConfigs:
		if m.configPrefix == "" {
			m.openMain()
			return
		}
		m.openConfigDir(parentConfigDir(m.configPrefix))
	case screenLanguage:
		m.openMain()
	default:
		m.openMain()
	}
}

func (m *menuModel) openMain() {
	m.screen = screenMain
	m.selected = 0
	m.configPrefix = ""
	m.configEntries = nil
	m.statusEntries = nil
}

func (m *menuModel) ignoreStartupWarning() {
	m.openMain()
	m.clearNotice()
}

func (m *menuModel) openConfigDir(prefix string) {
	entries, err := config.ListDir(prefix)
	m.screen = screenConfigs
	m.selected = 0
	m.configPrefix = prefix
	m.configEntries = entries
	m.clearNotice()
	if err != nil {
		m.setStickyNotice(noticeError, err.Error())
	}
}

func (m *menuModel) openReleases() {
	m.screen = screenReleases
	m.selected = 0
	m.sys = sysinfo.Detect()
	m.clearNotice()
}

func (m *menuModel) openStatus() {
	m.screen = screenStatus
	m.selected = 0
	m.statusEntries = installer.Status()
	m.clearNotice()
}

func (m *menuModel) openLanguage() {
	m.screen = screenLanguage
	m.selected = 0
	m.clearNotice()
}

func (m menuModel) items() []menuItem {
	switch m.screen {
	case screenStartupWarning:
		return m.startupWarningItems()
	case screenConfigs:
		return m.configItems()
	case screenReleases:
		return m.releaseItems()
	case screenStatus:
		return m.statusItems()
	case screenLanguage:
		return m.languageItems()
	default:
		return m.mainItems()
	}
}

func (m menuModel) startupWarningItems() []menuItem {
	return []menuItem{
		{
			label:  m.t(i18n.MainInstallLabel),
			detail: m.t(i18n.StartupInstallDetail),
			run: func(m *menuModel) tea.Cmd {
				if _, warning := localServiceCheck(m.catalog, m.lang); warning != "" {
					return m.setNotice(noticeError, warning)
				}
				m.openMain()
				m.setStickyNotice(noticeInfo, m.t(i18n.NoticeRequestAdmin))
				return installCmd(m.catalog, m.lang)
			},
		},
		{
			label:  m.t(i18n.StartupIgnoreLabel),
			detail: m.t(i18n.StartupIgnoreDetail),
			run: func(m *menuModel) tea.Cmd {
				m.ignoreStartupWarning()
				return nil
			},
		},
	}
}

func (m menuModel) mainItems() []menuItem {
	return []menuItem{
		{
			label:  m.t(i18n.MainInstallLabel),
			detail: m.t(i18n.MainInstallDetail),
			run: func(m *menuModel) tea.Cmd {
				if _, warning := localServiceCheck(m.catalog, m.lang); warning != "" {
					return m.setNotice(noticeError, warning)
				}
				m.setStickyNotice(noticeInfo, m.t(i18n.NoticeRequestAdmin))
				return installCmd(m.catalog, m.lang)
			},
		},
		{
			label:  m.t(i18n.MainUninstallLabel),
			detail: m.t(i18n.MainUninstallDetail),
			run: func(m *menuModel) tea.Cmd {
				m.setStickyNotice(noticeInfo, m.t(i18n.NoticeRemovingHook))
				return uninstallCmd(m.catalog, m.lang)
			},
		},
		{
			label:  m.t(i18n.MainStatusLabel),
			detail: m.t(i18n.MainStatusDetail),
			run: func(m *menuModel) tea.Cmd {
				m.openStatus()
				return nil
			},
		},
		{
			label:  m.t(i18n.MainSelectConfigLabel),
			detail: m.t(i18n.MainSelectConfigDetail),
			run: func(m *menuModel) tea.Cmd {
				m.openConfigDir("")
				return nil
			},
		},
		{
			label:  m.t(i18n.MainRegenerateLabel),
			detail: m.t(i18n.MainRegenerateDetail),
			run: func(m *menuModel) tea.Cmd {
				m.openReleases()
				return nil
			},
		},
		{
			label:  m.t(i18n.MainLanguageLabel),
			detail: m.t(i18n.MainLanguageDetail),
			run: func(m *menuModel) tea.Cmd {
				m.openLanguage()
				return nil
			},
		},
		{
			label:  m.t(i18n.MainExitLabel),
			detail: m.t(i18n.MainExitDetail),
			run: func(m *menuModel) tea.Cmd {
				return tea.Quit
			},
		},
	}
}

func (m menuModel) languageItems() []menuItem {
	available := m.catalog.Available()
	items := make([]menuItem, 0, len(available)+1)
	for _, lang := range available {
		l := lang
		items = append(items, menuItem{
			label:  m.catalog.DisplayName(l),
			detail: m.languageDetail(l),
			active: l == m.lang,
			run: func(m *menuModel) tea.Cmd {
				if err := i18n.Set(l); err != nil {
					return m.setNotice(noticeError, err.Error())
				}
				m.lang = l
				m.openMain()
				return m.setNotice(
					noticeSuccess,
					m.t(i18n.NoticeLanguageSelected, m.catalog.DisplayName(l)),
				)
			},
		})
	}
	items = append(items, menuItem{
		label:  m.t(i18n.BackLabel),
		detail: m.t(i18n.BackMain),
		run: func(m *menuModel) tea.Cmd {
			m.openMain()
			return nil
		},
	})
	return items
}

func (m menuModel) languageDetail(lang i18n.Language) string {
	switch lang {
	case i18n.Russian:
		return m.t(i18n.LanguageRussianDetail)
	default:
		return m.t(i18n.LanguageEnglishDetail)
	}
}

func (m menuModel) configItems() []menuItem {
	active := config.ActiveName()
	items := make([]menuItem, 0, len(m.configEntries)+1)
	for _, entry := range m.configEntries {
		e := entry
		if e.IsDir {
			items = append(items, menuItem{
				label:  e.Name + "/",
				detail: m.t(i18n.FolderDetail),
				active: active == e.ID || strings.HasPrefix(active, e.ID+"/"),
				run: func(m *menuModel) tea.Cmd {
					m.openConfigDir(e.ID)
					return nil
				},
			})
			continue
		}
		items = append(items, menuItem{
			label:  e.Name,
			detail: e.ID,
			active: e.ID == active,
			run: func(m *menuModel) tea.Cmd {
				if err := config.SetActive(e.ID); err != nil {
					return m.setNotice(noticeError, err.Error())
				}
				m.openMain()
				return m.setNotice(noticeSuccess, m.t(i18n.NoticeActiveConfig, e.ID))
			},
		})
	}
	items = append(items, menuItem{
		label:  m.t(i18n.BackLabel),
		detail: m.t(i18n.BackPrevious),
		run: func(m *menuModel) tea.Cmd {
			m.goBack()
			return nil
		},
	})
	return items
}

func (m menuModel) releaseItems() []menuItem {
	releases := profile.Releases()
	items := make([]menuItem, 0, len(releases)+1)
	for _, release := range releases {
		r := release
		items = append(items, menuItem{
			label:  r.Label,
			detail: m.catalog.ReleaseDescription(m.lang, r.Version, r.Description),
			active: config.ActiveName() == r.DefaultID(),
			run: func(m *menuModel) tea.Cmd {
				m.openMain()
				m.setStickyNotice(noticeInfo, m.t(i18n.NoticeRegenerating, r.Version))
				return regenerateCmd(m.catalog, m.lang, r, m.sys)
			},
		})
	}
	items = append(items, menuItem{
		label:  m.t(i18n.BackLabel),
		detail: m.t(i18n.BackMain),
		run: func(m *menuModel) tea.Cmd {
			m.openMain()
			return nil
		},
	})
	return items
}

func (m menuModel) statusItems() []menuItem {
	return []menuItem{
		{
			label:  m.t(i18n.BackLabel),
			detail: m.t(i18n.BackMain),
			run: func(m *menuModel) tea.Cmd {
				m.openMain()
				return nil
			},
		},
	}
}

func (m menuModel) View() string {
	var b strings.Builder
	contentWidth := m.contentWidth()

	b.WriteString(m.topBar(contentWidth))
	b.WriteString("\n\n")
	b.WriteString(m.screenTitle())
	b.WriteString("\n\n")

	if body := m.screenBody(); body != "" {
		b.WriteString(body)
		b.WriteString("\n\n")
	}

	b.WriteString(renderItems(m.items(), m.selected, contentWidth))
	if detail := m.selectedDetail(); detail != "" {
		b.WriteString("\n\n")
		b.WriteString(detailBoxStyle.Width(contentWidth).Render(detail))
	}
	if m.notice != "" {
		b.WriteString("\n\n")
		b.WriteString(m.renderNotice())
	}
	b.WriteString("\n\n")
	b.WriteString(m.footer(contentWidth))

	panel := panelStyle.Width(m.panelWidth()).Render(b.String())
	return m.renderCanvas(panel)
}

func (m menuModel) panelWidth() int {
	if m.width <= 0 {
		return defaultPanelWidth
	}
	available := m.width - 4
	return clampInt(available, minPanelWidth, maxPanelWidth)
}

func (m menuModel) itemWidth() int {
	return maxInt(36, m.panelWidth()-6)
}

func (m menuModel) contentWidth() int {
	return m.itemWidth()
}

func (m menuModel) renderCanvas(panel string) string {
	width := maxInt(m.width, m.panelWidth()+4)
	height := maxInt(m.height, lipgloss.Height(panel)+4)
	return appStyle.Width(width).Height(height).Render(panel)
}

func (m menuModel) selectedDetail() string {
	items := m.items()
	if len(items) == 0 || m.selected >= len(items) {
		return ""
	}
	return items[m.selected].detail
}

func (m menuModel) t(key i18n.Key, args ...any) string {
	return m.catalog.T(m.lang, key, args...)
}

func (m menuModel) topBar(width int) string {
	left := brandTitle()
	right := m.profileTitle(width / 2)
	if lipgloss.Width(left)+1+lipgloss.Width(right) > width {
		left = brandTitle()
	}
	if lipgloss.Width(left)+1+lipgloss.Width(right) > width {
		return renderSolidRow(width, colorSurface, left)
	}
	return renderSplitRow(width, colorSurface, left, right)
}

func brandTitle() string {
	return strings.Join([]string{
		brandEXBOStyle.Render("EXBO"),
		brandCommunityStyle.Render("Community"),
	}, "")
}

func (m menuModel) footer(width int) string {
	help := helpStyle.Render(m.t(i18n.FooterHelp))
	version := versionStyle.Render(buildLabel())
	if lipgloss.Width(help)+1+lipgloss.Width(version) > width {
		help = helpStyle.Render(m.t(i18n.FooterHelpShort))
	}
	if lipgloss.Width(help)+1+lipgloss.Width(version) > width {
		return renderSplitRow(width, colorSurface, "", version)
	}
	return renderSplitRow(width, colorSurface, help, version)
}

func buildLabel() string {
	if buildinfo.Commit == "" || buildinfo.Commit == "unknown" {
		return buildinfo.Version
	}
	return buildinfo.Version + " - " + buildinfo.Commit
}

func (m menuModel) profileTitle(maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}

	active := config.ActiveName()
	if active == "" {
		active = m.t(i18n.ProfileUnset)
	}

	label := m.t(i18n.ProfileLabel)
	if maxWidth <= lipgloss.Width(label)+5 {
		return profileLabelStyle.Render(strings.TrimSpace(label))
	}

	badgeMaxWidth := maxWidth - lipgloss.Width(label)
	active = truncateMiddle(active, badgeMaxWidth)

	return strings.Join([]string{
		profileLabelStyle.Render(label),
		profileValueStyle.Render(active),
	}, "")
}

func (m menuModel) screenTitle() string {
	switch m.screen {
	case screenStartupWarning:
		return sectionStyle.Render(m.t(i18n.TitleSetup))
	case screenConfigs:
		if m.configPrefix == "" {
			return sectionStyle.Render(m.t(i18n.TitleConfigs))
		}
		return sectionStyle.Render(m.t(i18n.TitleConfigs)) + " " + mutedStyle.Render(m.configPrefix)
	case screenReleases:
		return sectionStyle.Render(m.t(i18n.TitleReleases))
	case screenStatus:
		return sectionStyle.Render(m.t(i18n.TitleStatus))
	case screenLanguage:
		return sectionStyle.Render(m.t(i18n.TitleLanguage))
	default:
		return sectionStyle.Render(m.t(i18n.TitleMain))
	}
}

func (m menuModel) screenBody() string {
	switch m.screen {
	case screenStartupWarning:
		return warningBoxStyle.Width(m.contentWidth()).Render(m.startupWarning)
	case screenConfigs:
		if len(m.configEntries) == 0 {
			return mutedStyle.Render(m.t(i18n.NoConfigs))
		}
	case screenReleases:
		return m.releaseBody()
	case screenStatus:
		return m.statusBody()
	case screenLanguage:
		return mutedStyle.Render(m.t(i18n.LanguageBody))
	}
	return ""
}

func (m menuModel) releaseBody() string {
	lines := []string{m.t(i18n.DetectedSystem, m.sys.Describe())}
	switch {
	case m.sys.TotalGB() < 8:
		lines = append(lines, warningStyle.Render(m.t(i18n.MemoryLowWarning)))
	case m.sys.TotalGB() <= 16:
		lines = append(lines, mutedStyle.Render(m.t(i18n.Memory16Note)))
	}
	return strings.Join(lines, "\n")
}

func (m menuModel) statusBody() string {
	lines := statusLines(m.catalog, m.lang, m.statusEntries)
	for i := range lines {
		lines[i] = mutedStyle.Render(lines[i])
	}
	return strings.Join(lines, "\n")
}

func (m menuModel) renderNotice() string {
	switch m.noticeKind {
	case noticeSuccess:
		return successStyle.Render(m.notice)
	case noticeWarning:
		return warningStyle.Render(m.notice)
	case noticeError:
		return errorStyle.Render(m.notice)
	default:
		return infoStyle.Render(m.notice)
	}
}

func (m *menuModel) setNotice(kind noticeKind, text string) tea.Cmd {
	if text == "" {
		m.clearNotice()
		return nil
	}

	m.noticeID++
	id := m.noticeID
	m.noticeKind = kind
	m.notice = text

	if !noticeAutoClears(kind) {
		return nil
	}
	return clearNoticeAfter(id)
}

func (m *menuModel) setStickyNotice(kind noticeKind, text string) {
	m.noticeID++
	m.noticeKind = kind
	m.notice = text
}

func (m *menuModel) clearNotice() {
	m.noticeID++
	m.notice = ""
}

func noticeAutoClears(kind noticeKind) bool {
	return kind == noticeSuccess || kind == noticeError
}

func clearNoticeAfter(id int) tea.Cmd {
	return tea.Tick(transientNoticeTTL, func(time.Time) tea.Msg {
		return clearNoticeMsg{id: id}
	})
}

func renderItems(items []menuItem, selected int, width int) string {
	if len(items) == 0 {
		return ""
	}

	lines := make([]string, 0, len(items))
	for i, item := range items {
		lines = append(lines, renderItem(item, i == selected, width))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func renderItem(item menuItem, selected bool, width int) string {
	bg := colorSurface
	markerStyle := itemMarkerStyle
	labelStyle := itemLabelStyle
	activeStyle := itemActiveStyle
	if selected {
		bg = colorSelected
		markerStyle = selectedMarkerStyle
		labelStyle = selectedLabelStyle
		activeStyle = selectedActiveStyle
	}

	cursor := " "
	if selected {
		cursor = ">"
	}
	active := " "
	if item.active {
		active = "*"
	}

	label := labelStyle.Render(item.label)
	if item.active {
		label = activeStyle.Render(item.label)
	}
	return renderSolidRow(
		width,
		bg,
		markerStyle.Render(cursor),
		rowTextStyle(bg, colorMuted).Render(" "),
		markerStyle.Render(active),
		rowTextStyle(bg, colorMuted).Render(" "),
		label,
	)
}

func installCmd(catalog *i18n.Catalog, lang i18n.Language) tea.Cmd {
	return func() tea.Msg {
		code, err := elevate.Run("install")
		switch {
		case err != nil:
			return actionResultMsg{kind: noticeError, text: err.Error()}
		case code != 0:
			return actionResultMsg{kind: noticeError, text: catalog.T(lang, i18n.NoticeInstallFailed, code)}
		default:
			return actionResultMsg{kind: noticeSuccess, text: catalog.T(lang, i18n.NoticeInstallDone)}
		}
	}
}

func uninstallCmd(catalog *i18n.Catalog, lang i18n.Language) tea.Cmd {
	return func() tea.Msg {
		if err := installer.Uninstall(); err != nil {
			return actionResultMsg{kind: noticeError, text: err.Error()}
		}
		return actionResultMsg{kind: noticeSuccess, text: catalog.T(lang, i18n.NoticeUninstallDone)}
	}
}

func regenerateCmd(catalog *i18n.Catalog, lang i18n.Language, release profile.Release, sys sysinfo.Info) tea.Cmd {
	return func() tea.Msg {
		generated, err := profile.Regenerate(release.Version, sys)
		if err != nil {
			return actionResultMsg{kind: noticeError, text: err.Error()}
		}
		return actionResultMsg{
			kind: noticeSuccess,
			text: catalog.T(
				lang,
				i18n.NoticeRegenerated,
				release.Version,
				len(generated),
				release.DefaultID(),
			),
		}
	}
}

func statusLines(catalog *i18n.Catalog, lang i18n.Language, entries []installer.Entry) []string {
	lines := make([]string, 0, len(entries)+1)
	anyInstalled := false
	for _, e := range entries {
		if e.Installed {
			lines = append(lines, catalog.T(lang, i18n.StatusInstalled, e.Target, e.Debugger))
			anyInstalled = true
			continue
		}
		lines = append(lines, catalog.T(lang, i18n.StatusNotInstalled, e.Target))
	}
	if !anyInstalled {
		lines = append(lines, catalog.T(lang, i18n.StatusNone))
	}
	return lines
}

func startupWarning(catalog *i18n.Catalog, lang i18n.Language) string {
	expectedService, warning := localServiceCheck(catalog, lang)
	if warning != "" {
		return warning
	}

	if other := mismatchedDebugger(expectedService, installer.Status()); other != "" {
		return catalog.T(lang, i18n.WarnHookOther, shortPath(other))
	}
	return ""
}

func localServiceCheck(catalog *i18n.Catalog, lang i18n.Language) (string, string) {
	expectedService, exists, err := installer.LocalServiceExists()
	if err != nil {
		return "", catalog.T(lang, i18n.WarnServiceCheck, err.Error())
	}
	if !exists {
		return "", catalog.T(lang, i18n.WarnServiceMiss)
	}
	return expectedService, ""
}

func mismatchedDebugger(expected string, entries []installer.Entry) string {
	for _, e := range entries {
		if !e.Installed {
			continue
		}

		debugger := debuggerExecutable(e.Debugger)
		if debugger == "" {
			continue
		}
		if !samePath(expected, debugger) {
			return debugger
		}
	}
	return ""
}

func debuggerExecutable(debugger string) string {
	s := strings.TrimSpace(debugger)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, `"`) {
		rest := s[1:]
		if idx := strings.IndexByte(rest, '"'); idx >= 0 {
			return rest[:idx]
		}
		return strings.Trim(s, `"`)
	}

	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return strings.Trim(fields[0], `"`)
}

func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func shortPath(path string) string {
	clean := filepath.Clean(path)
	parent := filepath.Base(filepath.Dir(clean))
	name := filepath.Base(clean)
	if parent == "." || parent == "" || name == "." || name == "" {
		return clean
	}
	return filepath.Join("...", parent, name)
}

func parentConfigDir(prefix string) string {
	idx := strings.LastIndex(prefix, "/")
	if idx < 0 {
		return ""
	}
	return prefix[:idx]
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func renderSolidRow(width int, bg lipgloss.TerminalColor, parts ...string) string {
	content := strings.Join(parts, "")
	if padding := width - lipgloss.Width(content); padding > 0 {
		content += lipgloss.NewStyle().
			Background(bg).
			Render(strings.Repeat(" ", padding))
	}
	return content
}

func renderSplitRow(width int, bg lipgloss.TerminalColor, left, right string) string {
	spacing := width - lipgloss.Width(left) - lipgloss.Width(right)
	if spacing < 1 {
		return renderSolidRow(width, bg, left)
	}
	return strings.Join([]string{
		left,
		lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", spacing)),
		right,
	}, "")
}

func rowTextStyle(bg lipgloss.TerminalColor, fg lipgloss.TerminalColor) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(fg).Background(bg)
}

func truncateMiddle(s string, maxWidth int) string {
	if maxWidth <= 0 || lipgloss.Width(s) <= maxWidth {
		return s
	}

	runes := []rune(s)
	if maxWidth <= 3 {
		return string(runes[:minInt(maxWidth, len(runes))])
	}

	left := (maxWidth - 3) / 2
	right := maxWidth - 3 - left
	return string(runes[:minInt(left, len(runes))]) + "..." + string(runes[maxInt(0, len(runes)-right):])
}

var (
	minPanelWidth     = 58
	defaultPanelWidth = 92
	maxPanelWidth     = 112

	colorCanvas   = lipgloss.Color("#0E1116")
	colorSurface  = lipgloss.Color("#141A22")
	colorRaised   = lipgloss.Color("#1B2630")
	colorSelected = lipgloss.Color("#1A2028")
	colorLine     = lipgloss.Color("#52798A")
	colorCream    = lipgloss.Color("#E8DFC9")
	colorText     = lipgloss.Color("#C9D1D9")
	colorMuted    = lipgloss.Color("#8B98A7")
	colorBlue     = lipgloss.Color("#8CBFD4")
	colorRed      = lipgloss.Color("#C98282")
	colorGreen    = lipgloss.Color("#9BCBB1")
	colorAmber    = lipgloss.Color("#D9B97A")

	appStyle = lipgloss.NewStyle().
			Background(colorCanvas).
			Padding(2, 2, 1, 2)

	panelStyle = lipgloss.NewStyle().
			Foreground(colorText).
			Background(colorSurface).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorLine).
			Padding(1, 2)

	brandEXBOStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorCream).
			Background(colorLine).
			Padding(0, 1)

	brandCommunityStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorCream).
				Background(lipgloss.Color("#7A3D3D")).
				Padding(0, 1)

	versionStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Background(colorSurface)

	profileLabelStyle = lipgloss.NewStyle().
				Foreground(colorMuted).
				Background(colorSurface)

	profileValueStyle = lipgloss.NewStyle().
				Foreground(colorText).
				Background(colorSurface)

	detailBoxStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Background(colorRaised).
			Padding(0, 1)

	warningBoxStyle = lipgloss.NewStyle().
			Foreground(colorAmber).
			Background(colorRaised).
			Padding(0, 1)

	sectionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorBlue)

	itemStyle = lipgloss.NewStyle().
			Foreground(colorText).
			Background(colorSurface).
			Padding(0, 1)

	selectedStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(colorText).
			Background(colorSelected)

	itemMarkerStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Background(colorSurface)

	selectedMarkerStyle = lipgloss.NewStyle().
				Foreground(colorBlue).
				Background(colorSelected).
				Bold(true)

	itemLabelStyle = lipgloss.NewStyle().
			Foreground(colorText).
			Background(colorSurface)

	selectedLabelStyle = lipgloss.NewStyle().
				Foreground(colorText).
				Background(colorSelected)

	itemDetailStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Background(colorSurface)

	selectedDetailStyle = lipgloss.NewStyle().
				Foreground(colorMuted).
				Background(colorSelected)

	itemActiveStyle = lipgloss.NewStyle().
			Foreground(colorGreen).
			Background(colorSurface).
			Bold(true)

	selectedActiveStyle = lipgloss.NewStyle().
				Foreground(colorGreen).
				Background(colorSelected).
				Bold(true)

	mutedStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	activeStyle = lipgloss.NewStyle().
			Foreground(colorGreen).
			Bold(true)

	infoStyle = lipgloss.NewStyle().
			Foreground(colorBlue)

	successStyle = lipgloss.NewStyle().
			Foreground(colorGreen).
			Bold(true)

	warningStyle = lipgloss.NewStyle().
			Foreground(colorAmber).
			Bold(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(colorRed).
			Bold(true)

	helpStyle = lipgloss.NewStyle().
			Foreground(colorMuted)
)
