package tunnel

import (
	"math"
	"reflect"
	"testing"
	"time"
)

func TestCacheRecordCapsHistoryAndTracksLimit(t *testing.T) {
	t.Parallel()

	cache := NewCache()
	target := testTarget("192.0.2.10:29450")
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)

	for i := 0; i < MaxHistory+3; i++ {
		cache.Record(
			target,
			ProbeResult{
				ClientRTT:    time.Duration(i+1) * time.Millisecond,
				ServerRTT:    2 * time.Millisecond,
				LimitReached: i == MaxHistory+2,
			},
			nil,
			now.Add(time.Duration(i)*time.Second),
		)
	}

	history, ok := cache.History(target.Region, target.Endpoint.Address)
	if !ok {
		t.Fatal("recorded history is missing")
	}
	if len(history.Samples) != MaxHistory {
		t.Fatalf("sample count = %d, want %d", len(history.Samples), MaxHistory)
	}
	if history.Samples[0].ClientRTTMicros != 4_000 {
		t.Fatalf("first retained sample = %dµs, want 4000µs", history.Samples[0].ClientRTTMicros)
	}
	if !history.IsLastTimeLimitReached {
		t.Fatal("last limit state was not retained")
	}
}

func TestCachePrioritizesRecentlyLimitedTargets(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	regular := testTarget("192.0.2.10:29450")
	limited := testTarget("192.0.2.11:29450")
	expired := testTarget("192.0.2.12:29450")
	cache := NewCache()
	cache.Record(limited, ProbeResult{LimitReached: true}, nil, now.Add(-time.Hour))
	cache.Record(expired, ProbeResult{LimitReached: true}, nil, now.Add(-CacheTTL-time.Second))

	got := cache.Prioritize([]Target{regular, expired, limited}, now)
	want := []Target{limited, regular, expired}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Prioritize() = %#v, want %#v", got, want)
	}
}

func TestCacheCloneDoesNotShareSamplesOrMap(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	target := testTarget("192.0.2.10:29450")
	cache := NewCache()
	cache.Record(target, ProbeResult{ClientRTT: time.Millisecond}, nil, now)

	clone := cache.Clone()
	history := clone.Endpoints[target.Key()]
	history.Samples[0].ClientRTTMicros = 99
	clone.Endpoints[target.Key()] = history
	delete(clone.Endpoints, target.Key())

	original, ok := cache.History(target.Region, target.Endpoint.Address)
	if !ok {
		t.Fatal("mutating clone removed original map entry")
	}
	if original.Samples[0].ClientRTTMicros == 99 {
		t.Fatal("clone shares sample storage with original")
	}
}

func TestFreshProbeClearsCachedLimit(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	target := testTarget("192.0.2.10:29450")
	cache := NewCache()
	cache.Record(target, ProbeResult{LimitReached: true}, nil, now.Add(-time.Minute))
	cache.Record(target, ProbeResult{LimitReached: false}, nil, now)

	if cache.WasRecentlyLimited(target.Region, target.Endpoint.Address, now) {
		t.Fatal("successful fresh probe did not clear cached limit")
	}
}

func TestMetricsAndScores(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	samples := []Sample{
		{
			MeasuredAt:      now.Add(-3 * time.Minute),
			ClientRTTMicros: 10_000,
			ServerRTTMillis: 5,
		},
		{
			MeasuredAt: now.Add(-2 * time.Minute),
			TimedOut:   true,
		},
		{
			MeasuredAt:      now.Add(-time.Minute),
			ClientRTTMicros: 30_000,
			ServerRTTMillis: 15,
		},
	}
	metrics, ok := calculateMetrics(samples, now, RecentHistoryTTL)
	if !ok {
		t.Fatal("calculateMetrics() returned no metrics")
	}
	if metrics.Samples != 3 || metrics.Successes != 2 {
		t.Fatalf("counts = %d/%d, want 2/3 successes", metrics.Successes, metrics.Samples)
	}
	if metrics.ClientRTT != 20 || metrics.ServerRTT != 10 || metrics.Ping != 30 {
		t.Fatalf("averages = client %.1f server %.1f ping %.1f", metrics.ClientRTT, metrics.ServerRTT, metrics.Ping)
	}
	if metrics.LossRate != 1.0/3.0 {
		t.Fatalf("loss = %v, want 1/3", metrics.LossRate)
	}
	if score, _ := metrics.Score(SearchModeGame, 1.5); score != 40 {
		t.Fatalf("game-weighted score = %.1f, want 40", score)
	}
	if score, _ := metrics.Score(SearchModeStability, 1); score <= metrics.Ping {
		t.Fatalf("stability score %.1f did not penalize loss/jitter", score)
	}
}

func TestCacheRankSortsBySelectedMode(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	fastClient := testTarget("192.0.2.10:29450")
	fastServer := testTarget("192.0.2.11:29450")
	cache := NewCache()
	cache.Record(
		fastClient,
		ProbeResult{ClientRTT: 5 * time.Millisecond, ServerRTT: 20 * time.Millisecond},
		nil,
		now,
	)
	cache.Record(
		fastServer,
		ProbeResult{ClientRTT: 15 * time.Millisecond, ServerRTT: time.Millisecond},
		nil,
		now,
	)

	byClient := cache.Rank(
		[]Target{fastServer, fastClient},
		SearchModeClientRTT,
		1,
		now,
	)
	if len(byClient) != 2 || byClient[0].Target.Key() != fastClient.Key() {
		t.Fatalf("client ranking = %#v", byClient)
	}

	byServer := cache.Rank(
		[]Target{fastClient, fastServer},
		SearchModeServerRTT,
		1,
		now,
	)
	if len(byServer) != 2 || byServer[0].Target.Key() != fastServer.Key() {
		t.Fatalf("server ranking = %#v", byServer)
	}
}

func TestCachePruneExpiresSamplesAndLimit(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	target := testTarget("192.0.2.10:29450")
	cache := NewCache()
	cache.Record(
		target,
		ProbeResult{LimitReached: true},
		nil,
		now.Add(-CacheTTL-time.Second),
	)
	cache.Prune(now)

	if len(cache.Endpoints) != 0 {
		t.Fatalf("expired endpoint remained in cache: %#v", cache.Endpoints)
	}
}

func TestCachePruneCapsAndOrdersLoadedHistory(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	target := testTarget("192.0.2.10:29450")
	history := EndpointHistory{
		Region:  target.Region,
		Pool:    target.Pool,
		Name:    target.Endpoint.Name,
		Address: target.Endpoint.Address,
	}
	for i := MaxHistory + 4; i >= 0; i-- {
		history.Samples = append(history.Samples, Sample{
			MeasuredAt:      now.Add(-time.Duration(i) * time.Minute),
			ClientRTTMicros: int64(i),
		})
	}
	cache := NewCache()
	cache.Endpoints[target.Key()] = history
	cache.Prune(now)

	got := cache.Endpoints[target.Key()].Samples
	if len(got) != MaxHistory {
		t.Fatalf("sample count = %d, want %d", len(got), MaxHistory)
	}
	for i := 1; i < len(got); i++ {
		if got[i].MeasuredAt.Before(got[i-1].MeasuredAt) {
			t.Fatalf("samples are not ordered: %#v", got)
		}
	}
}

func TestCachePruneDropsFutureAndNegativeSamples(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	target := testTarget("192.0.2.10:29450")
	cache := NewCache()
	cache.Endpoints[target.Key()] = EndpointHistory{
		Region:                 target.Region,
		Pool:                   target.Pool,
		Name:                   target.Endpoint.Name,
		Address:                target.Endpoint.Address,
		IsLastTimeLimitReached: true,
		LastLimitCheckedAt:     now.Add(time.Minute),
		Samples: []Sample{
			{MeasuredAt: now.Add(time.Minute), ClientRTTMicros: 1},
			{MeasuredAt: now.Add(-time.Minute), ClientRTTMicros: -1},
			{MeasuredAt: now.Add(-time.Minute), ClientRTTMicros: 1},
		},
	}

	cache.Prune(now)
	history, ok := cache.History(target.Region, target.Endpoint.Address)
	if !ok {
		t.Fatal("valid history was removed")
	}
	if len(history.Samples) != 1 || history.Samples[0].ClientRTTMicros != 1 {
		t.Fatalf("pruned samples = %#v", history.Samples)
	}
	if history.IsLastTimeLimitReached || !history.LastLimitCheckedAt.IsZero() {
		t.Fatalf("future limit state was retained: %#v", history)
	}
}

func TestMetricsScoreRejectsNonFiniteValues(t *testing.T) {
	t.Parallel()

	metrics := Metrics{
		Successes: 1,
		ClientRTT: 10,
		ServerRTT: 5,
		Ping:      15,
		P90:       15,
	}
	if _, ok := metrics.Score(SearchModeGame, math.Inf(1)); ok {
		t.Fatal("Score() accepted an infinite client weight")
	}
	metrics.ClientRTT = -1
	if _, ok := metrics.Score(SearchModePing, 1); ok {
		t.Fatal("Score() accepted negative metrics")
	}
}

func testTarget(address string) Target {
	return Target{
		Region: RegionRU,
		Pool:   "MSK2",
		Endpoint: Endpoint{
			Name:    address,
			Address: address,
		},
	}
}
