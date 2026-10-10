package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/tunnel"
)

func TestTunnelResultsRankEveryAvailableTargetBySelectedMode(t *testing.T) {
	t.Parallel()

	for _, mode := range tunnel.SearchModes() {
		t.Run(string(mode), func(t *testing.T) {
			m, targets := tunnelResultTestModel(8)
			m.tunnelMenu.settings.TunnelSearch.Mode = mode
			ranked := m.tunnelRankedResults()
			if len(ranked) != len(targets) {
				t.Fatalf("ranked %d targets, want all %d", len(ranked), len(targets))
			}
			for index, result := range ranked {
				want := index
				if mode == tunnel.SearchModePing || mode == tunnel.SearchModeServerRTT || mode == tunnel.SearchModeStability {
					want = len(targets) - 1 - index
				}
				if result.Target.Key() != targets[want].Key() {
					t.Fatalf("rank %d = %s, want %s", index+1, result.Target.Key(), targets[want].Key())
				}
			}
		})
	}
}

func TestTunnelRerunsRejectStaleAndDuplicateReplies(t *testing.T) {
	t.Parallel()

	m, targets := tunnelResultTestModel(2)
	m.tunnelMenu.cache = tunnel.NewCache()
	m.tunnelMenu.probes = nil
	var previous tea.Cmd
	var previousSession uint64
	for round := range 6 {
		current := m.startTunnelSearch()
		if m.tunnelMenu.probeRound != round+1 {
			t.Fatalf("rerun %d reports round %d", round+1, m.tunnelMenu.probeRound)
		}
		if previous != nil {
			assertTunnelCommandsCanceled(t, previous)
			m.updateTunnel(tunnelTestReply(previousSession, targets[0], tunnel.ProbeResult{ServerRTT: time.Hour}))
			if m.tunnelMenu.pending != len(targets) || m.tunnelMenu.probes[targets[0].Key()].received {
				t.Fatal("prior round reply changed the current round")
			}
		}
		reply := tunnelTestReply(
			m.tunnelMenu.probeSession,
			targets[0],
			tunnel.ProbeResult{ClientRTT: 6 * time.Millisecond, ServerRTT: 9 * time.Millisecond},
		)
		m.updateTunnel(reply)
		reply.result.ServerRTT = time.Hour
		m.updateTunnel(reply)
		metrics, ok := m.tunnelMenu.cache.Metrics(targets[0].Region, targets[0].Endpoint.Address, time.Now())
		if !ok || metrics.Samples != min((round+1)*tunnelSearchProbeCount, tunnel.MaxHistory) || metrics.ServerRTT != 9 || m.tunnelMenu.pending != 1 {
			t.Fatalf("round %d accepted a stale or duplicate sample: metrics=%#v pending=%d", round+1, metrics, m.tunnelMenu.pending)
		}
		fresh, _ := m.tunnelMenu.roundCache.Metrics(targets[0].Region, targets[0].Endpoint.Address, time.Now())
		if fresh.Samples != tunnelSearchProbeCount || fresh.ServerRTT != 9 {
			t.Fatalf("round %d reused old ranking samples: %#v", round+1, fresh)
		}
		previous = current
		previousSession = m.tunnelMenu.probeSession
	}
	m.stopTunnelProbes()
	assertTunnelCommandsCanceled(t, previous)
	m.updateTunnel(tunnelTestReply(previousSession, targets[0], tunnel.ProbeResult{ServerRTT: time.Hour}))
	metrics, _ := m.tunnelMenu.cache.Metrics(targets[0].Region, targets[0].Endpoint.Address, time.Now())
	if metrics.Samples != tunnel.MaxHistory || metrics.ServerRTT != 9 || m.tunnelMenu.pending != 0 || m.tunnelMenu.probeCancel != nil {
		t.Fatalf("stopped round changed state: metrics=%#v pending=%d", metrics, m.tunnelMenu.pending)
	}
}

func TestTunnelRoundReplacementCancelsPreviousProbes(t *testing.T) {
	t.Parallel()

	m, targets := tunnelResultTestModel(2)
	first := m.startTunnelRound(targets)
	m.startTunnelRound(targets)
	assertTunnelCommandsCanceled(t, first)
	m.stopTunnelProbes()
}

func TestTunnelResultWindowKeepsEverySelectedCandidateVisible(t *testing.T) {
	t.Parallel()

	m, targets := tunnelResultTestModel(8)
	m.tunnelMenu.settings.TunnelSearch.Mode = tunnel.SearchModeClientRTT
	items := m.tunnelResultItems()
	for _, maxRows := range []int{12, 7, 1} {
		for index, target := range targets {
			m.selected = tunnelResultActionRows + index
			window := m.renderTunnelResultWindow(items, 80, maxRows)
			if lipgloss.Height(window) > maxRows {
				t.Fatalf("row budget %d produced %d rows", maxRows, lipgloss.Height(window))
			}
			plain := ansi.Strip(window)
			selectedVisible := false
			for _, line := range strings.Split(plain, "\n") {
				if strings.HasPrefix(line, ">") && strings.Contains(line, target.Endpoint.Name) {
					selectedVisible = true
				}
			}
			if !selectedVisible {
				t.Fatalf("selected %s is hidden with row budget %d:\n%s", target.Endpoint.Name, maxRows, plain)
			}
			if maxRows == 12 {
				visible := 0
				for _, candidate := range targets {
					if strings.Contains(plain, candidate.Endpoint.Name) {
						visible++
					}
				}
				if visible != 5 {
					t.Fatalf("visible candidates = %d, want five", visible)
				}
			}
		}
	}
}

func assertTunnelCommandsCanceled(t *testing.T, command tea.Cmd) {
	t.Helper()
	batch, ok := command().(tea.BatchMsg)
	if !ok {
		t.Fatal("expected a multi-target probe batch")
	}
	for _, probe := range batch {
		msg, ok := probe().(tunnelProbeMsg)
		if !ok || !errors.Is(msg.err, context.Canceled) {
			t.Fatalf("superseded probe result = %#v, want cancellation", msg)
		}
	}
}

func tunnelResultTestModel(count int) (menuModel, []tunnel.Target) {
	m := menuModel{
		screen: screenTunnelResults,
		tunnelMenu: tunnelMenuState{
			settings:   tunnel.DefaultSettings(),
			cache:      tunnel.NewCache(),
			roundCache: tunnel.NewCache(),
			region:     tunnel.RegionRU,
			probes:     make(map[string]tunnelProbeView, count),
		},
	}
	pool := tunnel.Pool{Name: "test"}
	now := time.Now()
	for index := range count {
		target := tunnel.Target{
			Region: tunnel.RegionRU,
			Pool:   pool.Name,
			Endpoint: tunnel.Endpoint{
				Name:    fmt.Sprintf("node%02d", index+1),
				Address: fmt.Sprintf("192.0.2.%d:29450", index+1),
			},
		}
		pool.Tunnels = append(pool.Tunnels, target.Endpoint)
		m.tunnelMenu.targets = append(m.tunnelMenu.targets, target)
		result := tunnel.ProbeResult{
			ClientRTT: time.Duration(index+1) * time.Millisecond,
			ServerRTT: time.Duration(2*(count-index)) * time.Millisecond,
		}
		m.tunnelMenu.cache.Record(target, result, nil, now)
		for range tunnelSearchProbeCount {
			m.tunnelMenu.roundCache.Record(target, result, nil, now)
		}
		m.tunnelMenu.probes[target.Key()] = tunnelProbeView{result: result, received: true, freshSuccess: true}
	}
	m.tunnelMenu.catalogs = map[tunnel.Region]tunnel.Catalog{
		tunnel.RegionRU: {Mode: "roxy", Pools: []tunnel.Pool{pool}, ClientToTunnelRTTWeight: 3},
	}
	return m, m.tunnelMenu.targets
}

func tunnelTestReply(session uint64, target tunnel.Target, result tunnel.ProbeResult) tunnelProbeMsg {
	msg := tunnelProbeMsg{session: session, target: target, result: result}
	for range tunnelSearchProbeCount {
		msg.samples = append(msg.samples, tunnelProbeSample{result: result, measuredAt: time.Now()})
	}
	return msg
}

func TestTunnelRankingDoesNotReuseHistoricalOutliers(t *testing.T) {
	t.Parallel()
	m, targets := tunnelResultTestModel(2)
	m.tunnelMenu.cache.Record(targets[1], tunnel.ProbeResult{ServerRTT: time.Hour}, nil, time.Now())
	ranked := m.tunnelRankedResults()
	if ranked[0].Target.Key() != targets[1].Key() || ranked[0].Metrics.Median != 4 {
		t.Fatalf("historical outlier changed fresh ranking: %#v", ranked)
	}
}

func TestTunnelBestWaitsForCompleteComparisonAndEnoughReplies(t *testing.T) {
	t.Parallel()
	m, targets := tunnelResultTestModel(2)
	m.tunnelMenu.pending = 1
	if !m.tunnelResultItems()[0].disabled {
		t.Fatal("automatic selection is enabled before all candidates finish")
	}
	m.tunnelMenu.pending = 0
	m.tunnelMenu.roundCache = tunnel.NewCache()
	m.tunnelMenu.roundCache.Record(targets[0], tunnel.ProbeResult{ClientRTT: time.Millisecond}, nil, time.Now())
	for range tunnelSearchProbeCount {
		m.tunnelMenu.roundCache.Record(targets[1], tunnel.ProbeResult{ClientRTT: 2 * time.Millisecond}, nil, time.Now())
	}
	ranked := m.tunnelRankedResults()
	best := bestTunnelResultIndex(ranked, 0)
	if best < 0 || ranked[best].Target.Key() != targets[1].Key() {
		t.Fatalf("automatic selection chose an under-sampled candidate: %#v", ranked)
	}
	m.tunnelMenu.roundCache = tunnel.NewCache()
	m.tunnelMenu.roundCache.Record(targets[0], tunnel.ProbeResult{ClientRTT: time.Millisecond}, nil, time.Now())
	if !m.tunnelResultItems()[0].disabled {
		t.Fatal("automatic selection is enabled with only one successful reply")
	}
}
