package ui

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/i18n"
	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/tunnel"
)

const (
	tunnelRoundTimeout      = time.Second
	tunnelPriorityHeadStart = 35 * time.Millisecond
	maxTunnelSearchResults  = 5
)

type tunnelMenuState struct {
	settings    tunnel.Settings
	settingsErr error
	cache       tunnel.Cache
	catalogs    map[tunnel.Region]tunnel.Catalog

	region           tunnel.Region
	pool             string
	catalogLoading   bool
	catalogErr       error
	catalogRequest   uint64
	catalogCancel    context.CancelFunc
	modeReturn       screen
	exclusionsReturn screen

	probeSession uint64
	probeRound   int
	pending      int
	targets      []tunnel.Target
	probes       map[string]tunnelProbeView
	probeCancel  context.CancelFunc
}

type tunnelProbeView struct {
	result       tunnel.ProbeResult
	err          error
	received     bool
	checking     bool
	freshSuccess bool
}

type tunnelCatalogMsg struct {
	request uint64
	region  tunnel.Region
	catalog tunnel.Catalog
	err     error
}

type tunnelProbeMsg struct {
	session    uint64
	target     tunnel.Target
	result     tunnel.ProbeResult
	err        error
	measuredAt time.Time
}

func (m *menuModel) openTunnel() tea.Cmd {
	m.stopTunnelProbes()
	m.stopTunnelCatalog()
	catalogs := m.tunnelMenu.catalogs
	if catalogs == nil {
		catalogs = make(map[tunnel.Region]tunnel.Catalog)
	}
	nextProbeSession := m.tunnelMenu.probeSession + 1
	nextCatalogRequest := m.tunnelMenu.catalogRequest + 1

	settings, settingsErr := tunnel.LoadSettings()
	cache, cacheErr := tunnel.LoadCache()
	m.tunnelMenu = tunnelMenuState{
		settings:       settings,
		settingsErr:    settingsErr,
		cache:          cache,
		catalogs:       catalogs,
		probeSession:   nextProbeSession,
		catalogRequest: nextCatalogRequest,
		modeReturn:     screenTunnelSearch,
	}
	m.screen = screenTunnelRegions
	m.selected = 0
	m.clearNotice()

	var notices []string
	if settingsErr != nil {
		notices = append(
			notices,
			m.t(i18n.NoticeTunnelSettingsLoad, settingsErr.Error()),
		)
	}
	if cacheErr != nil {
		notices = append(notices, m.t(i18n.NoticeTunnelCache, cacheErr.Error()))
	}
	if len(notices) > 0 {
		return m.setNotice(noticeError, strings.Join(notices, "\n"))
	}
	return nil
}

func (m *menuModel) showTunnelRegions() {
	m.stopTunnelProbes()
	m.stopTunnelCatalog()
	m.tunnelMenu.catalogRequest++
	m.tunnelMenu.catalogLoading = false
	m.tunnelMenu.catalogErr = nil
	m.screen = screenTunnelRegions
	m.selected = 0
	m.clearNotice()
}

func (m *menuModel) showTunnelPools() {
	m.stopTunnelProbes()
	m.screen = screenTunnelPools
	m.selected = 0
	m.clearNotice()
}

func (m *menuModel) showTunnelSearch() {
	m.stopTunnelProbes()
	m.screen = screenTunnelSearch
	m.selected = 0
	m.clearNotice()
}

func (m *menuModel) returnFromTunnelMode() {
	switch m.tunnelMenu.modeReturn {
	case screenTunnelResults:
		m.screen = screenTunnelResults
	default:
		m.screen = screenTunnelSearch
	}
	m.selected = 0
	m.clearNotice()
}

func (m *menuModel) returnFromTunnelExclusions() {
	switch m.tunnelMenu.exclusionsReturn {
	case screenTunnelResults:
		m.screen = screenTunnelResults
	default:
		m.screen = screenTunnelSearch
	}
	m.selected = 0
	m.clearNotice()
}

func (m *menuModel) stopTunnelProbes() {
	if m.tunnelMenu.probeCancel != nil {
		m.tunnelMenu.probeCancel()
		m.tunnelMenu.probeCancel = nil
	}
	m.tunnelMenu.probeSession++
	m.tunnelMenu.probeRound = 0
	m.tunnelMenu.pending = 0
	m.tunnelMenu.targets = nil
	m.tunnelMenu.probes = nil
}

func (m *menuModel) stopTunnelCatalog() {
	if m.tunnelMenu.catalogCancel == nil {
		return
	}
	m.tunnelMenu.catalogCancel()
	m.tunnelMenu.catalogCancel = nil
}

func (m *menuModel) openTunnelRegion(region tunnel.Region) tea.Cmd {
	m.stopTunnelProbes()
	m.stopTunnelCatalog()
	m.tunnelMenu.catalogRequest++
	m.tunnelMenu.region = region
	m.tunnelMenu.pool = ""
	m.tunnelMenu.catalogErr = nil
	m.screen = screenTunnelPools
	m.selected = 0
	m.clearNotice()

	if _, ok := m.tunnelMenu.catalogs[region]; ok {
		m.tunnelMenu.catalogLoading = false
		return nil
	}
	return m.fetchTunnelCatalog(region)
}

func (m *menuModel) fetchTunnelCatalog(region tunnel.Region) tea.Cmd {
	m.stopTunnelCatalog()
	m.tunnelMenu.catalogRequest++
	request := m.tunnelMenu.catalogRequest
	m.tunnelMenu.catalogLoading = true
	m.tunnelMenu.catalogErr = nil
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	m.tunnelMenu.catalogCancel = cancel

	return func() tea.Msg {
		defer cancel()
		catalog, err := tunnel.Fetch(ctx, region)
		return tunnelCatalogMsg{
			request: request,
			region:  region,
			catalog: catalog,
			err:     err,
		}
	}
}

func (m *menuModel) openTunnelPool(pool tunnel.Pool) tea.Cmd {
	m.stopTunnelProbes()
	m.tunnelMenu.pool = pool.Name
	m.screen = screenTunnelEndpoints
	m.selected = 0
	m.clearNotice()

	targets := make([]tunnel.Target, 0, len(pool.Tunnels))
	for _, endpoint := range pool.Tunnels {
		targets = append(targets, tunnel.Target{
			Region:   m.tunnelMenu.region,
			Pool:     pool.Name,
			Endpoint: endpoint,
		})
	}
	return m.startTunnelRound(targets)
}

func (m *menuModel) openTunnelSearch() {
	m.stopTunnelProbes()
	m.screen = screenTunnelSearch
	m.selected = 0
	m.clearNotice()
}

func (m *menuModel) openTunnelMode(returnTo screen) {
	m.tunnelMenu.modeReturn = returnTo
	m.screen = screenTunnelMode
	m.selected = 0
	m.clearNotice()
}

func (m *menuModel) openTunnelExclusions(returnTo screen) {
	m.tunnelMenu.exclusionsReturn = returnTo
	if returnTo != screenTunnelResults {
		m.stopTunnelProbes()
	}
	m.screen = screenTunnelExclusions
	m.selected = 0
	m.clearNotice()
}

func (m *menuModel) startTunnelSearch() tea.Cmd {
	catalog, ok := m.currentTunnelCatalog()
	if !ok {
		return m.setNotice(noticeError, m.t(i18n.TunnelNoTargets))
	}
	targets := catalog.Targets(
		m.tunnelMenu.region,
		func(pool string) bool {
			return m.tunnelMenu.settings.IsExcluded(m.tunnelMenu.region, pool)
		},
	)
	if len(targets) == 0 {
		return m.setNotice(noticeWarning, m.t(i18n.TunnelNoTargets))
	}

	m.stopTunnelProbes()
	m.screen = screenTunnelResults
	m.selected = 0
	m.clearNotice()
	return m.startTunnelRound(targets)
}

func (m *menuModel) startTunnelRound(targets []tunnel.Target) tea.Cmd {
	if len(targets) == 0 {
		return nil
	}
	m.tunnelMenu.probes = make(map[string]tunnelProbeView, len(targets))

	m.tunnelMenu.probeSession++
	session := m.tunnelMenu.probeSession
	m.tunnelMenu.probeRound++
	m.tunnelMenu.pending = len(targets)
	m.tunnelMenu.targets = append([]tunnel.Target(nil), targets...)
	ctx, cancel := context.WithTimeout(context.Background(), tunnelRoundTimeout)
	m.tunnelMenu.probeCancel = cancel

	now := time.Now()
	prioritized := m.tunnelMenu.cache.Prioritize(targets, now)
	hasPriority := false
	for _, target := range prioritized {
		if m.tunnelMenu.cache.WasRecentlyLimited(
			target.Region,
			target.Endpoint.Address,
			now,
		) {
			hasPriority = true
			break
		}
	}

	commands := make([]tea.Cmd, 0, len(prioritized))
	for _, target := range prioritized {
		view := m.tunnelMenu.probes[target.Key()]
		view.checking = true
		m.tunnelMenu.probes[target.Key()] = view

		delay := time.Duration(0)
		if hasPriority &&
			!m.tunnelMenu.cache.WasRecentlyLimited(
				target.Region,
				target.Endpoint.Address,
				now,
			) {
			delay = tunnelPriorityHeadStart
		}
		commands = append(commands, tunnelProbeCmd(ctx, session, target, delay))
	}
	return tea.Batch(commands...)
}

func tunnelProbeCmd(
	ctx context.Context,
	session uint64,
	target tunnel.Target,
	delay time.Duration,
) tea.Cmd {
	return func() tea.Msg {
		if delay > 0 {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-ctx.Done():
				return tunnelProbeMsg{
					session:    session,
					target:     target,
					err:        ctx.Err(),
					measuredAt: time.Now().UTC(),
				}
			}
		}

		result, err := (tunnel.Prober{Timeout: tunnelRoundTimeout}).Probe(
			ctx,
			target.Endpoint,
		)
		return tunnelProbeMsg{
			session:    session,
			target:     target,
			result:     result,
			err:        err,
			measuredAt: time.Now().UTC(),
		}
	}
}

func (m *menuModel) updateTunnel(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case tunnelCatalogMsg:
		if msg.request != m.tunnelMenu.catalogRequest ||
			msg.region != m.tunnelMenu.region {
			return nil, true
		}
		m.stopTunnelCatalog()
		m.tunnelMenu.catalogLoading = false
		m.tunnelMenu.catalogErr = msg.err
		m.selected = 0
		if msg.err != nil {
			return m.setNotice(
				noticeError,
				m.t(i18n.TunnelLoadFailed, msg.err.Error()),
			), true
		}
		m.tunnelMenu.catalogs[msg.region] = msg.catalog
		m.clearNotice()
		return nil, true

	case tunnelProbeMsg:
		if msg.session != m.tunnelMenu.probeSession {
			return nil, true
		}
		view, ok := m.tunnelMenu.probes[msg.target.Key()]
		if !ok || !view.checking {
			return nil, true
		}

		view.result = msg.result
		view.err = msg.err
		view.received = true
		view.checking = false
		view.freshSuccess = msg.err == nil
		m.tunnelMenu.probes[msg.target.Key()] = view
		m.tunnelMenu.cache.Record(msg.target, msg.result, msg.err, msg.measuredAt)
		if m.tunnelMenu.pending > 0 {
			m.tunnelMenu.pending--
		}
		if m.tunnelMenu.pending != 0 {
			return nil, true
		}
		if m.tunnelMenu.probeCancel != nil {
			m.tunnelMenu.probeCancel()
			m.tunnelMenu.probeCancel = nil
		}

		_, replies := m.tunnelRoundCounts()
		slog.Info(
			"tunnel probe round completed",
			"region", m.tunnelMenu.region,
			"round", m.tunnelMenu.probeRound,
			"targets", len(m.tunnelMenu.targets),
			"replies", replies,
			"no_reply", len(m.tunnelMenu.targets)-replies,
		)
		if err := tunnel.SaveCache(m.tunnelMenu.cache.Clone()); err != nil {
			return m.setNotice(
				noticeError,
				m.t(i18n.NoticeTunnelCache, err.Error()),
			), true
		}
		return nil, true

	default:
		return nil, false
	}
}

func (m menuModel) tunnelItems() []menuItem {
	switch m.screen {
	case screenTunnelRegions:
		return m.tunnelRegionItems()
	case screenTunnelPools:
		return m.tunnelPoolItems()
	case screenTunnelEndpoints:
		return m.tunnelEndpointItems()
	case screenTunnelSearch:
		return m.tunnelSearchItems()
	case screenTunnelMode:
		return m.tunnelModeItems()
	case screenTunnelExclusions:
		return m.tunnelExclusionItems()
	case screenTunnelResults:
		return m.tunnelResultItems()
	default:
		return nil
	}
}

func (m menuModel) tunnelRegionItems() []menuItem {
	items := make([]menuItem, 0, len(tunnel.Regions())+1)
	for _, region := range tunnel.Regions() {
		region := region
		override, active := m.tunnelMenu.settings.Override(region)
		detail := m.t(i18n.TunnelRegionDetail, region.Label())
		if active {
			detail = m.t(
				i18n.TunnelRegionOverrideDetail,
				region.Label(),
				override.Pool,
				override.Name,
			)
		}
		items = append(items, menuItem{
			label:  region.Label(),
			detail: detail,
			active: active,
			run: func(m *menuModel) tea.Cmd {
				return m.openTunnelRegion(region)
			},
		})
	}
	return append(items, m.tunnelBackItem(m.t(i18n.BackMain), func(m *menuModel) {
		m.openMain()
	}))
}

func (m menuModel) tunnelPoolItems() []menuItem {
	_, hasOverride := m.tunnelMenu.settings.Override(m.tunnelMenu.region)
	items := []menuItem{
		{
			label:    m.t(i18n.TunnelGameDefaultLabel),
			detail:   m.t(i18n.TunnelGameDefaultDetail, m.tunnelMenu.region.Label()),
			active:   !hasOverride,
			disabled: m.tunnelSettingsLocked(),
			run: func(m *menuModel) tea.Cmd {
				return m.selectGameDefaultTunnel(m.tunnelMenu.region)
			},
		},
	}
	if m.tunnelMenu.catalogLoading {
		return append(items, m.tunnelBackItem(m.t(i18n.BackPrevious), func(m *menuModel) {
			m.showTunnelRegions()
		}))
	}
	if m.tunnelMenu.catalogErr != nil {
		items = append(items, menuItem{
			label:  m.t(i18n.TunnelRetryLabel),
			detail: m.t(i18n.TunnelRetryDetail),
			run: func(m *menuModel) tea.Cmd {
				return m.fetchTunnelCatalog(m.tunnelMenu.region)
			},
		})
		return append(items, m.tunnelBackItem(m.t(i18n.BackPrevious), func(m *menuModel) {
			m.showTunnelRegions()
		}))
	}

	catalog, ok := m.currentTunnelCatalog()
	if !ok {
		return append(items, m.tunnelBackItem(m.t(i18n.BackPrevious), func(m *menuModel) {
			m.showTunnelRegions()
		}))
	}
	items = append(items, menuItem{
		label:  m.t(i18n.TunnelSearchLabel),
		detail: m.t(i18n.TunnelSearchDetail),
		run: func(m *menuModel) tea.Cmd {
			m.openTunnelSearch()
			return nil
		},
	})
	for _, pool := range catalog.Pools {
		pool := pool
		excluded := m.tunnelMenu.settings.IsExcluded(m.tunnelMenu.region, pool.Name)
		detailKey := i18n.TunnelPoolDetail
		if excluded {
			detailKey = i18n.TunnelPoolExcludedDetail
		}
		override, hasOverride := m.tunnelMenu.settings.Override(m.tunnelMenu.region)
		active := hasOverride && override.Pool == pool.Name
		items = append(items, menuItem{
			label:  pool.Name,
			detail: m.t(detailKey, len(pool.Tunnels)),
			active: active,
			run: func(m *menuModel) tea.Cmd {
				return m.openTunnelPool(pool)
			},
		})
	}
	return append(items, m.tunnelBackItem(m.t(i18n.BackPrevious), func(m *menuModel) {
		m.showTunnelRegions()
	}))
}

func (m menuModel) tunnelEndpointItems() []menuItem {
	tones := m.tunnelEndpointTones()
	items := make([]menuItem, 0, len(m.tunnelMenu.targets)+1)
	for _, target := range m.tunnelMenu.targets {
		target := target
		label, detail, disabled, tone := m.tunnelEndpointPresentation(target)
		if relativeTone, ok := tones[target.Key()]; ok {
			tone = relativeTone
		}
		active := m.tunnelSelectionMatches(target)
		items = append(items, menuItem{
			label:    target.Endpoint.Name + "  " + label,
			detail:   detail,
			active:   active,
			disabled: disabled || m.tunnelSettingsLocked(),
			tone:     tone,
			run: func(m *menuModel) tea.Cmd {
				return m.selectTunnelTarget(target)
			},
		})
	}
	return append(items, m.tunnelBackItem(m.t(i18n.BackPrevious), func(m *menuModel) {
		m.showTunnelPools()
	}))
}

func (m menuModel) tunnelSearchItems() []menuItem {
	excluded := m.tunnelExcludedCount()
	return []menuItem{
		{
			label:  m.t(i18n.TunnelRunSearchLabel),
			detail: m.t(i18n.TunnelRunSearchDetail),
			run: func(m *menuModel) tea.Cmd {
				return m.startTunnelSearch()
			},
		},
		{
			label: m.t(
				i18n.TunnelModeLabel,
				m.tunnelModeLabel(m.tunnelMenu.settings.TunnelSearch.Mode),
			),
			detail:   m.t(i18n.TunnelModeDetail),
			disabled: m.tunnelSettingsLocked(),
			run: func(m *menuModel) tea.Cmd {
				m.openTunnelMode(screenTunnelSearch)
				return nil
			},
		},
		{
			label:    m.t(i18n.TunnelExclusionsLabel, excluded),
			detail:   m.t(i18n.TunnelExclusionsDetail),
			disabled: m.tunnelSettingsLocked(),
			run: func(m *menuModel) tea.Cmd {
				m.openTunnelExclusions(screenTunnelSearch)
				return nil
			},
		},
		m.tunnelBackItem(m.t(i18n.BackPrevious), func(m *menuModel) {
			m.showTunnelPools()
		}),
	}
}

func (m menuModel) tunnelModeItems() []menuItem {
	items := make([]menuItem, 0, len(tunnel.SearchModes())+1)
	for _, mode := range tunnel.SearchModes() {
		mode := mode
		items = append(items, menuItem{
			label:    m.tunnelModeLabel(mode),
			detail:   m.tunnelModeDetail(mode),
			active:   m.tunnelMenu.settings.TunnelSearch.Mode == mode,
			disabled: m.tunnelSettingsLocked(),
			run: func(m *menuModel) tea.Cmd {
				return m.selectTunnelMode(mode)
			},
		})
	}
	return append(items, m.tunnelBackItem(m.t(i18n.BackPrevious), func(m *menuModel) {
		m.returnFromTunnelMode()
	}))
}

func (m menuModel) tunnelExclusionItems() []menuItem {
	catalog, ok := m.currentTunnelCatalog()
	if !ok {
		return []menuItem{
			m.tunnelBackItem(m.t(i18n.BackPrevious), func(m *menuModel) {
				m.showTunnelSearch()
			}),
		}
	}

	items := make([]menuItem, 0, len(catalog.Pools)+2)
	for _, pool := range catalog.Pools {
		pool := pool
		excluded := m.tunnelMenu.settings.IsExcluded(m.tunnelMenu.region, pool.Name)
		items = append(items, menuItem{
			label:    pool.Name,
			detail:   m.t(i18n.TunnelExclusionPoolDetail, len(pool.Tunnels)),
			active:   excluded,
			disabled: m.tunnelSettingsLocked(),
			tone: func() lipgloss.TerminalColor {
				if excluded {
					return colorAmber
				}
				return nil
			}(),
			run: func(m *menuModel) tea.Cmd {
				return m.toggleTunnelExclusion(pool.Name)
			},
		})
	}
	if m.tunnelExcludedCount() > 0 {
		items = append(items, menuItem{
			label:    m.t(i18n.TunnelClearExclusionsLabel),
			detail:   m.t(i18n.TunnelClearExclusionsDetail),
			disabled: m.tunnelSettingsLocked(),
			run: func(m *menuModel) tea.Cmd {
				return m.clearTunnelExclusions()
			},
		})
	}
	return append(items, m.tunnelBackItem(m.t(i18n.BackPrevious), func(m *menuModel) {
		m.returnFromTunnelExclusions()
	}))
}

func (m menuModel) tunnelResultItems() []menuItem {
	ranked := m.tunnelRankedResults()
	unavailable := m.tunnelUnavailableSearchTargets()
	items := make([]menuItem, 0, len(ranked)+len(unavailable)+5)

	useBestDetail := m.t(i18n.TunnelNoResults)
	if len(ranked) > 0 {
		useBestDetail = m.t(
			i18n.TunnelUseBestDetail,
			ranked[0].Target.Pool,
			ranked[0].Target.Endpoint.Name,
		)
	}
	items = append(items, menuItem{
		label:    m.t(i18n.TunnelUseBestLabel),
		detail:   useBestDetail,
		disabled: len(ranked) == 0 || m.tunnelSettingsLocked(),
		run: func(m *menuModel) tea.Cmd {
			current := m.tunnelRankedResults()
			if len(current) == 0 {
				return nil
			}
			return m.selectTunnelTarget(current[0].Target)
		},
	})
	items = append(items, menuItem{
		label: m.t(
			i18n.TunnelModeLabel,
			m.tunnelModeLabel(m.tunnelMenu.settings.TunnelSearch.Mode),
		),
		detail:   m.t(i18n.TunnelModeDetail),
		disabled: m.tunnelSettingsLocked(),
		run: func(m *menuModel) tea.Cmd {
			m.openTunnelMode(screenTunnelResults)
			return nil
		},
	})
	items = append(items, menuItem{
		label:    m.t(i18n.TunnelExclusionsLabel, m.tunnelExcludedCount()),
		detail:   m.t(i18n.TunnelExclusionsDetail),
		disabled: m.tunnelSettingsLocked(),
		run: func(m *menuModel) tea.Cmd {
			m.openTunnelExclusions(screenTunnelResults)
			return nil
		},
	})
	items = append(items, menuItem{
		label:  m.t(i18n.TunnelRerunLabel),
		detail: m.t(i18n.TunnelRerunDetail),
		run: func(m *menuModel) tea.Cmd {
			return m.startTunnelSearch()
		},
	})

	tones := rankedTargetTones(ranked)
	for index, result := range ranked {
		result := result
		label := fmt.Sprintf(
			"%d. %s / %s  %s",
			index+1,
			result.Target.Pool,
			result.Target.Endpoint.Name,
			formatTunnelLatency(result.Score),
		)
		items = append(items, menuItem{
			label:    label,
			detail:   m.tunnelResultDetail(result),
			active:   m.tunnelSelectionMatches(result.Target),
			disabled: m.tunnelSettingsLocked(),
			tone:     tones[result.Target.Key()],
			run: func(m *menuModel) tea.Cmd {
				return m.selectTunnelTarget(result.Target)
			},
		})
	}
	for _, target := range unavailable {
		label, detail, _, tone := m.tunnelEndpointPresentation(target)
		items = append(items, menuItem{
			label:    target.Pool + " / " + target.Endpoint.Name + "  " + label,
			detail:   detail,
			active:   m.tunnelSelectionMatches(target),
			disabled: true,
			tone:     tone,
		})
	}
	return append(items, m.tunnelBackItem(m.t(i18n.BackPrevious), func(m *menuModel) {
		m.showTunnelSearch()
	}))
}

func (m menuModel) tunnelBackItem(
	detail string,
	back func(*menuModel),
) menuItem {
	return menuItem{
		label:  m.t(i18n.BackLabel),
		detail: detail,
		run: func(m *menuModel) tea.Cmd {
			back(m)
			return nil
		},
	}
}

func (m menuModel) tunnelTitle() string {
	var key i18n.Key
	var suffix string
	switch m.screen {
	case screenTunnelRegions:
		key = i18n.TitleTunnel
	case screenTunnelPools:
		key = i18n.TitleTunnelPools
		suffix = m.tunnelMenu.region.Label()
	case screenTunnelEndpoints:
		key = i18n.TitleTunnelNodes
		suffix = strings.TrimSpace(
			m.tunnelMenu.region.Label() + " / " + m.tunnelMenu.pool,
		)
	case screenTunnelSearch:
		key = i18n.TitleTunnelSearch
		suffix = m.tunnelMenu.region.Label()
	case screenTunnelMode:
		key = i18n.TitleTunnelMode
	case screenTunnelExclusions:
		key = i18n.TitleTunnelExclude
		suffix = m.tunnelMenu.region.Label()
	case screenTunnelResults:
		key = i18n.TitleTunnelResults
		suffix = m.tunnelMenu.region.Label()
	default:
		key = i18n.TitleTunnel
	}

	title := sectionStyle.Render(m.t(key))
	if suffix != "" {
		title += surfaceSpace() + mutedStyle.Render(suffix)
	}
	return title
}

func (m menuModel) tunnelBody() string {
	switch m.screen {
	case screenTunnelRegions:
		lines := []string{mutedStyle.Render(m.t(i18n.TunnelBodyRegions))}
		for _, region := range tunnel.Regions() {
			override, ok := m.tunnelMenu.settings.Override(region)
			if !ok {
				continue
			}
			lines = append(
				lines,
				activeStyle.Render(
					m.t(
						i18n.TunnelActiveOverride,
						region.Label(),
						override.Pool,
						override.Name,
					),
				),
			)
		}
		if len(lines) == 1 {
			lines = append(lines, mutedStyle.Render(m.t(i18n.TunnelActiveDefault)))
		}
		return strings.Join(lines, "\n")

	case screenTunnelPools:
		switch {
		case m.tunnelMenu.catalogLoading:
			return infoStyle.Render(m.t(i18n.TunnelLoading))
		case m.tunnelMenu.catalogErr != nil:
			return errorStyle.Render(
				m.t(i18n.TunnelLoadFailed, m.tunnelMenu.catalogErr.Error()),
			)
		default:
			catalog, ok := m.currentTunnelCatalog()
			if !ok {
				return ""
			}
			return mutedStyle.Render(
				m.t(
					i18n.TunnelCatalogSummary,
					m.tunnelMenu.region.Label(),
					len(catalog.Pools),
					len(catalog.Targets(m.tunnelMenu.region, nil)),
				),
			)
		}

	case screenTunnelEndpoints, screenTunnelResults:
		total := len(m.tunnelMenu.targets)
		done, replies := m.tunnelRoundCounts()
		if m.tunnelMenu.pending > 0 {
			return infoStyle.Render(
				m.t(
					i18n.TunnelProgress,
					done,
					total,
					replies,
					done-replies,
					m.tunnelMenu.probeRound,
				),
			)
		}
		if total > 0 {
			return mutedStyle.Render(
				m.t(
					i18n.TunnelComplete,
					replies,
					total,
					total-replies,
					m.tunnelMenu.probeRound,
				),
			)
		}

	case screenTunnelSearch:
		return mutedStyle.Render(
			m.t(
				i18n.TunnelModeLabel,
				m.tunnelModeLabel(m.tunnelMenu.settings.TunnelSearch.Mode),
			),
		)

	case screenTunnelExclusions:
		return mutedStyle.Render(
			m.t(i18n.TunnelExclusionsLabel, m.tunnelExcludedCount()),
		)
	}
	return ""
}

func (m menuModel) tunnelEndpointPresentation(
	target tunnel.Target,
) (label, detail string, disabled bool, tone lipgloss.TerminalColor) {
	view := m.tunnelMenu.probes[target.Key()]
	metrics, hasMetrics := m.tunnelMenu.cache.Metrics(
		target.Region,
		target.Endpoint.Address,
		time.Now(),
	)
	limited := m.tunnelMenu.cache.WasRecentlyLimited(
		target.Region,
		target.Endpoint.Address,
		time.Now(),
	)

	switch {
	case !view.received && limited:
		label = m.t(i18n.TunnelEndpointCheckingLast)
		detail = target.Endpoint.Address + "\n" + m.t(i18n.TunnelEndpointLimited)
		return label, detail, true, colorAmber

	case !view.received && hasMetrics:
		label = m.t(i18n.TunnelEndpointCached, formatTunnelLatency(metrics.Ping))
		detail = target.Endpoint.Address + "\n" + m.tunnelHistoryDetail(metrics)
		return label, detail, true, colorMuted

	case !view.received:
		label = m.t(i18n.TunnelEndpointChecking)
		detail = target.Endpoint.Address
		return label, detail, true, colorMuted

	case view.err != nil:
		if limited {
			label = m.t(i18n.TunnelEndpointNoResponseLast)
			detail = target.Endpoint.Address + "\n" + label
			return label, detail, true, colorAmber
		}
		label = m.t(i18n.TunnelEndpointNoResponse)
		detail = target.Endpoint.Address + "\n" + m.t(i18n.TunnelEndpointNoResponse)
		return label, detail, true, colorMuted

	case view.result.LimitReached:
		label = m.t(i18n.TunnelEndpointLimited)
		detail = target.Endpoint.Address + "\n" + m.t(i18n.TunnelEndpointLimited)
		return label, detail, true, colorRed

	default:
		total := view.result.ClientRTT + view.result.ServerRTT
		label = m.t(i18n.TunnelEndpointReady, formatTunnelDuration(total))
		detail = m.t(
			i18n.TunnelEndpointDetail,
			target.Endpoint.Address,
			formatTunnelDuration(view.result.ClientRTT),
			formatTunnelDuration(view.result.ServerRTT),
			formatTunnelDuration(total),
		)
		if hasMetrics {
			detail += "\n" + m.tunnelHistoryDetail(metrics)
		}
		return label, detail, !view.freshSuccess, colorGreen
	}
}

func (m menuModel) tunnelHistoryDetail(metrics tunnel.Metrics) string {
	return m.t(
		i18n.TunnelEndpointHistory,
		metrics.Successes,
		metrics.Samples,
		formatTunnelLatency(metrics.P90),
		formatTunnelLatency(metrics.Jitter),
		fmt.Sprintf("%.0f%%", metrics.LossRate*100),
	)
}

func (m menuModel) tunnelEndpointTones() map[string]lipgloss.TerminalColor {
	available := m.tunnelAvailableTargets()
	ranked := m.tunnelMenu.cache.Rank(
		available,
		tunnel.SearchModePing,
		1,
		time.Now(),
	)
	return rankedTargetTones(ranked)
}

func (m menuModel) tunnelRankedResults() []tunnel.RankedTarget {
	catalog, ok := m.currentTunnelCatalog()
	if !ok {
		return nil
	}
	ranked := m.tunnelMenu.cache.Rank(
		m.tunnelAvailableTargets(),
		m.tunnelMenu.settings.TunnelSearch.Mode,
		catalog.ClientToTunnelRTTWeight,
		time.Now(),
	)
	if len(ranked) > maxTunnelSearchResults {
		ranked = ranked[:maxTunnelSearchResults]
	}
	return ranked
}

func (m menuModel) tunnelAvailableTargets() []tunnel.Target {
	targets := make([]tunnel.Target, 0, len(m.tunnelMenu.targets))
	for _, target := range m.tunnelMenu.targets {
		if m.tunnelMenu.settings.IsExcluded(target.Region, target.Pool) {
			continue
		}
		view := m.tunnelMenu.probes[target.Key()]
		if !view.received ||
			!view.freshSuccess ||
			view.err != nil ||
			view.result.LimitReached {
			continue
		}
		targets = append(targets, target)
	}
	return targets
}

func (m menuModel) tunnelUnavailableSearchTargets() []tunnel.Target {
	now := time.Now()
	targets := make([]tunnel.Target, 0)
	for _, target := range m.tunnelMenu.targets {
		if m.tunnelMenu.settings.IsExcluded(target.Region, target.Pool) {
			continue
		}
		view := m.tunnelMenu.probes[target.Key()]
		limited := m.tunnelMenu.cache.WasRecentlyLimited(
			target.Region,
			target.Endpoint.Address,
			now,
		)
		if view.received &&
			view.err == nil &&
			!view.result.LimitReached &&
			!limited {
			continue
		}
		if limited || (view.received && view.err == nil && view.result.LimitReached) {
			targets = append(targets, target)
		}
	}
	return targets
}

func rankedTargetTones(
	ranked []tunnel.RankedTarget,
) map[string]lipgloss.TerminalColor {
	tones := make(map[string]lipgloss.TerminalColor, len(ranked))
	if len(ranked) == 0 {
		return tones
	}

	minScore := ranked[0].Score
	maxScore := ranked[0].Score
	for _, result := range ranked[1:] {
		if result.Score < minScore {
			minScore = result.Score
		}
		if result.Score > maxScore {
			maxScore = result.Score
		}
	}
	spread := maxScore - minScore
	if spread < 5 || spread < minScore*0.15 {
		for _, result := range ranked {
			tones[result.Target.Key()] = colorGreen
		}
		return tones
	}

	for _, result := range ranked {
		position := (result.Score - minScore) / spread
		switch {
		case position <= 0.35:
			tones[result.Target.Key()] = colorGreen
		case position <= 0.7:
			tones[result.Target.Key()] = colorAmber
		default:
			tones[result.Target.Key()] = colorRed
		}
	}
	return tones
}

func (m menuModel) tunnelResultDetail(result tunnel.RankedTarget) string {
	return m.t(
		i18n.TunnelResultDetail,
		result.Target.Endpoint.Address,
		formatTunnelLatency(result.Metrics.ClientRTT),
		formatTunnelLatency(result.Metrics.ServerRTT),
		formatTunnelLatency(result.Metrics.Ping),
		formatTunnelLatency(result.Metrics.P90),
		formatTunnelLatency(result.Metrics.Jitter),
		fmt.Sprintf("%.0f%%", result.Metrics.LossRate*100),
		result.Metrics.Successes,
		result.Metrics.Samples,
	)
}

func (m menuModel) tunnelModeLabel(mode tunnel.SearchMode) string {
	switch mode {
	case tunnel.SearchModeClientRTT:
		return m.t(i18n.TunnelModeClientLabel)
	case tunnel.SearchModeServerRTT:
		return m.t(i18n.TunnelModeServerLabel)
	case tunnel.SearchModeStability:
		return m.t(i18n.TunnelModeStabilityLabel)
	case tunnel.SearchModeGame:
		return m.t(i18n.TunnelModeGameLabel)
	default:
		return m.t(i18n.TunnelModePingLabel)
	}
}

func (m menuModel) tunnelModeDetail(mode tunnel.SearchMode) string {
	switch mode {
	case tunnel.SearchModeClientRTT:
		return m.t(i18n.TunnelModeClientDetail)
	case tunnel.SearchModeServerRTT:
		return m.t(i18n.TunnelModeServerDetail)
	case tunnel.SearchModeStability:
		return m.t(i18n.TunnelModeStabilityDetail)
	case tunnel.SearchModeGame:
		return m.t(i18n.TunnelModeGameDetail)
	default:
		return m.t(i18n.TunnelModePingDetail)
	}
}

func (m menuModel) currentTunnelCatalog() (tunnel.Catalog, bool) {
	catalog, ok := m.tunnelMenu.catalogs[m.tunnelMenu.region]
	return catalog, ok
}

func (m menuModel) tunnelExcludedCount() int {
	return len(
		m.tunnelMenu.settings.TunnelSearch.ExcludedPools[m.tunnelMenu.region],
	)
}

func (m menuModel) tunnelRoundCounts() (done, replies int) {
	for _, target := range m.tunnelMenu.targets {
		view, ok := m.tunnelMenu.probes[target.Key()]
		if !ok || !view.received {
			continue
		}
		done++
		if view.err != nil {
			continue
		}
		replies++
	}
	return done, replies
}

func (m menuModel) tunnelSelectionMatches(target tunnel.Target) bool {
	override, ok := m.tunnelMenu.settings.Override(target.Region)
	return ok && override.Address == target.Endpoint.Address
}

func (m *menuModel) selectGameDefaultTunnel(region tunnel.Region) tea.Cmd {
	if m.tunnelSettingsLocked() {
		return m.tunnelSettingsLoadNotice()
	}
	previous := m.tunnelMenu.settings.Clone()
	m.tunnelMenu.settings.ClearOverride(region)
	if err := tunnel.SaveSettings(m.tunnelMenu.settings); err != nil {
		m.tunnelMenu.settings = previous
		return m.setNotice(
			noticeError,
			m.t(i18n.NoticeTunnelSettings, err.Error()),
		)
	}
	return m.setNotice(
		noticeSuccess,
		m.t(i18n.NoticeTunnelDefault, region.Label()),
	)
}

func (m *menuModel) selectTunnelTarget(target tunnel.Target) tea.Cmd {
	if m.tunnelSettingsLocked() {
		return m.tunnelSettingsLoadNotice()
	}
	previous := m.tunnelMenu.settings.Clone()
	m.tunnelMenu.settings.SetOverride(tunnel.Selection{
		Region:  target.Region,
		Pool:    target.Pool,
		Name:    target.Endpoint.Name,
		Address: target.Endpoint.Address,
	})
	if err := tunnel.SaveSettings(m.tunnelMenu.settings); err != nil {
		m.tunnelMenu.settings = previous
		return m.setNotice(
			noticeError,
			m.t(i18n.NoticeTunnelSettings, err.Error()),
		)
	}
	return m.setNotice(
		noticeSuccess,
		m.t(
			i18n.NoticeTunnelSelected,
			target.Region.Label(),
			target.Pool,
			target.Endpoint.Name,
		),
	)
}

func (m *menuModel) selectTunnelMode(mode tunnel.SearchMode) tea.Cmd {
	if m.tunnelSettingsLocked() {
		return m.tunnelSettingsLoadNotice()
	}
	previous := m.tunnelMenu.settings.Clone()
	m.tunnelMenu.settings.TunnelSearch.Mode = mode
	if err := tunnel.SaveSettings(m.tunnelMenu.settings); err != nil {
		m.tunnelMenu.settings = previous
		return m.setNotice(
			noticeError,
			m.t(i18n.NoticeTunnelSettings, err.Error()),
		)
	}
	m.returnFromTunnelMode()
	return m.setNotice(
		noticeSuccess,
		m.t(i18n.NoticeTunnelMode, m.tunnelModeLabel(mode)),
	)
}

func (m *menuModel) toggleTunnelExclusion(pool string) tea.Cmd {
	if m.tunnelSettingsLocked() {
		return m.tunnelSettingsLoadNotice()
	}
	previous := m.tunnelMenu.settings.Clone()
	m.tunnelMenu.settings.ToggleExcluded(m.tunnelMenu.region, pool)
	if err := tunnel.SaveSettings(m.tunnelMenu.settings); err != nil {
		m.tunnelMenu.settings = previous
		return m.setNotice(
			noticeError,
			m.t(i18n.NoticeTunnelSettings, err.Error()),
		)
	}
	return nil
}

func (m *menuModel) clearTunnelExclusions() tea.Cmd {
	if m.tunnelSettingsLocked() {
		return m.tunnelSettingsLoadNotice()
	}
	previous := m.tunnelMenu.settings.Clone()
	delete(
		m.tunnelMenu.settings.TunnelSearch.ExcludedPools,
		m.tunnelMenu.region,
	)
	if err := tunnel.SaveSettings(m.tunnelMenu.settings); err != nil {
		m.tunnelMenu.settings = previous
		return m.setNotice(
			noticeError,
			m.t(i18n.NoticeTunnelSettings, err.Error()),
		)
	}
	return nil
}

func (m menuModel) tunnelSettingsLocked() bool {
	return m.tunnelMenu.settingsErr != nil
}

func (m *menuModel) tunnelSettingsLoadNotice() tea.Cmd {
	return m.setNotice(
		noticeError,
		m.t(i18n.NoticeTunnelSettingsLoad, m.tunnelMenu.settingsErr.Error()),
	)
}

func formatTunnelDuration(value time.Duration) string {
	return formatTunnelLatency(float64(value) / float64(time.Millisecond))
}

func formatTunnelLatency(value float64) string {
	switch {
	case value < 10:
		return fmt.Sprintf("%.1f ms", value)
	case value < 100:
		return fmt.Sprintf("%.1f ms", value)
	default:
		return fmt.Sprintf("%.0f ms", value)
	}
}
