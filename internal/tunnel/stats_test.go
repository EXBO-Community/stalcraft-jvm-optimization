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

func TestCacheMetricsPreservesHistoricalSummaries(t *testing.T) {
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
	target := testTarget("192.0.2.10:29450")
	cache := NewCache()
	cache.Endpoints[target.Key()] = EndpointHistory{Samples: samples}
	metrics, ok := cache.Metrics(target.Region, target.Endpoint.Address, now)
	if !ok {
		t.Fatal("Metrics() returned no metrics")
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
	if metrics.Median != 30 || metrics.P90 != 45 || metrics.Max != 45 || metrics.Jitter != 30 {
		t.Fatalf("distribution = median %.1f P90 %.1f max %.1f jitter %.1f, want 30/45/45/30",
			metrics.Median, metrics.P90, metrics.Max, metrics.Jitter)
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

	for _, tt := range []struct {
		mode   SearchMode
		first  Target
		scores [2]float64
	}{
		{SearchModeClientRTT, fastClient, [2]float64{5, 15}},
		{SearchModeServerRTT, fastServer, [2]float64{1, 20}},
		{SearchModePing, fastServer, [2]float64{16, 25}},
		{SearchModeStability, fastServer, [2]float64{16, 25}},
		{SearchModeGame, fastClient, [2]float64{30, 31}},
	} {
		t.Run(string(tt.mode), func(t *testing.T) {
			ranked := cache.Rank([]Target{fastServer, fastClient}, tt.mode, 2, now)
			if len(ranked) != 2 || ranked[0].Target.Key() != tt.first.Key() {
				t.Fatalf("ranking = %#v, want %s first", ranked, tt.first.Endpoint.Address)
			}
			if ranked[0].Score != tt.scores[0] || ranked[1].Score != tt.scores[1] {
				t.Fatalf("scores = %v/%v, want %v/%v",
					ranked[0].Score, ranked[1].Score, tt.scores[0], tt.scores[1])
			}
		})
	}
}

func TestCacheRankPrefersStableSeriesOverLowerNoisyMedian(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	stable := testTarget("192.0.2.10:29450")
	noisy := testTarget("192.0.2.11:29450")
	cache := NewCache()
	for i := range 10 {
		measuredAt := now.Add(-time.Duration(9-i) * time.Second)
		cache.Record(stable, ProbeResult{
			ClientRTT: 30 * time.Millisecond,
			ServerRTT: 30 * time.Millisecond,
		}, nil, measuredAt)
		latency := 20 * time.Millisecond
		if i >= 8 {
			latency = 60 * time.Millisecond
		}
		cache.Record(noisy, ProbeResult{
			ClientRTT: latency,
			ServerRTT: latency,
		}, nil, measuredAt)
	}

	for _, tt := range []struct {
		mode       SearchMode
		noisyScore float64
	}{
		{SearchModePing, 80},
		{SearchModeStability, 120},
	} {
		t.Run(string(tt.mode), func(t *testing.T) {
			ranked := cache.Rank([]Target{noisy, stable}, tt.mode, 1, now)
			if len(ranked) != 2 || ranked[0].Target.Key() != stable.Key() {
				t.Fatalf("ranking = %#v, want stable series first", ranked)
			}
			if ranked[0].Score != 60 || ranked[1].Score != tt.noisyScore {
				t.Fatalf("scores = %v/%v, want 60/%v",
					ranked[0].Score, ranked[1].Score, tt.noisyScore)
			}
		})
	}
}

func TestCacheRankResistsOneOutlierWithoutChangingRawHistory(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	target := testTarget("192.0.2.10:29450")
	cache := NewCache()
	for i := range 10 {
		server := 15 * time.Millisecond
		if i == 9 {
			server = 2_000_000 * time.Millisecond
		}
		cache.Record(target, ProbeResult{
			ClientRTT: 10 * time.Millisecond,
			ServerRTT: server,
		}, nil, now.Add(-time.Duration(9-i)*time.Second))
	}
	before, ok := cache.History(target.Region, target.Endpoint.Address)
	if !ok || before.Samples[9].ServerRTTMillis != 2_000_000 {
		t.Fatalf("raw outlier was not retained: %#v", before)
	}

	for _, tt := range []struct {
		mode  SearchMode
		score float64
	}{
		{SearchModePing, 25},
		{SearchModeClientRTT, 10},
		{SearchModeServerRTT, 15},
		{SearchModeGame, 30},
		{SearchModeStability, 25},
	} {
		t.Run(string(tt.mode), func(t *testing.T) {
			ranked := cache.Rank([]Target{target}, tt.mode, 1.5, now)
			if len(ranked) != 1 || ranked[0].Score != tt.score {
				t.Fatalf("ranking = %#v, want score %v", ranked, tt.score)
			}
			metrics := ranked[0].Metrics
			if metrics.Median != 25 || metrics.P90 != 25 || metrics.Max != 2_000_010 ||
				metrics.ClientRTT != 10 || metrics.ServerRTT != 200_013.5 ||
				metrics.Ping != 200_023.5 {
				t.Fatalf("historical metrics lost the raw outlier: %#v", metrics)
			}
		})
	}
	after, _ := cache.History(target.Region, target.Endpoint.Address)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("ranking changed raw history: before %#v, after %#v", before, after)
	}
}

func TestCacheRankUsesPairedSamplesForEachMode(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	target := testTarget("192.0.2.10:29450")
	cache := NewCache()
	for _, pair := range [][2]time.Duration{
		{0, 100 * time.Millisecond},
		{10 * time.Millisecond, 0},
		{100 * time.Millisecond, 10 * time.Millisecond},
	} {
		cache.Record(target, ProbeResult{
			ClientRTT: pair[0],
			ServerRTT: pair[1],
		}, nil, now)
	}

	for _, tt := range []struct {
		mode  SearchMode
		score float64
	}{
		{SearchModePing, 105},
		{SearchModeClientRTT, 55},
		{SearchModeServerRTT, 55},
		{SearchModeGame, 155},
		{SearchModeStability, 110},
	} {
		t.Run(string(tt.mode), func(t *testing.T) {
			ranked := cache.Rank([]Target{target}, tt.mode, 2, now)
			if len(ranked) != 1 || ranked[0].Score != tt.score {
				t.Fatalf("paired-sample ranking = %#v, want score %v", ranked, tt.score)
			}
			if ranked[0].Metrics.Median != 100 || ranked[0].Metrics.P90 != 110 {
				t.Fatalf("paired total distribution = %#v, want median 100 and P90 110",
					ranked[0].Metrics)
			}
		})
	}
}

func TestCacheRankPenalizesFailuresEvenAboveProbeTimeout(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	target := testTarget("192.0.2.10:29450")
	for _, failures := range []struct {
		name  string
		count int
	}{
		{"none", 0},
		{"one", 1},
		{"four", 4},
		{"nine", 9},
		{"all", 10},
	} {
		t.Run(failures.name, func(t *testing.T) {
			cache := NewCache()
			for i := range 10 {
				var probeErr error
				if i < failures.count {
					probeErr = ErrProbeTimeout
				}
				cache.Record(target, ProbeResult{
					ClientRTT: 1500 * time.Millisecond,
					ServerRTT: 1200 * time.Millisecond,
				}, probeErr, now)
			}
			metrics, ok := cache.Metrics(target.Region, target.Endpoint.Address, now)
			fraction := float64(failures.count) / 10
			if !ok || metrics.Samples != 10 || metrics.Successes != 10-failures.count ||
				metrics.LossRate != fraction {
				t.Fatalf("failed series metrics = %#v, want %d/10 successes and failure fraction %v",
					metrics, 10-failures.count, fraction)
			}
			for _, tt := range []struct {
				mode SearchMode
				base float64
			}{
				{SearchModePing, 2700},
				{SearchModeClientRTT, 1500},
				{SearchModeServerRTT, 1200},
				{SearchModeGame, 4200},
				{SearchModeStability, 2700},
			} {
				t.Run(string(tt.mode), func(t *testing.T) {
					ranked := cache.Rank([]Target{target}, tt.mode, 2, now)
					if failures.count == 10 {
						if len(ranked) != 0 {
							t.Fatalf("series with no successful replies was ranked: %#v", ranked)
						}
						return
					}
					want := tt.base + float64(ProbeTimeout/time.Millisecond)*fraction
					if len(ranked) != 1 || math.Abs(ranked[0].Score-want) > 1e-9 {
						t.Fatalf("ranking = %#v, want score %v", ranked, want)
					}
				})
			}
		})
	}
}

func TestCacheRankQuantileBoundaries(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	target := testTarget("192.0.2.10:29450")
	for _, tt := range []struct {
		name   string
		count  int
		median float64
		p90    float64
		score  float64
	}{
		{"one", 1, 1, 1, 1},
		{"two", 2, 1.5, 2, 1.75},
		{"nine", 9, 5, 9, 7},
		{"ten", 10, 5.5, 9, 7.25},
		{"eleven", 11, 6, 10, 8},
		{"twenty", 20, 10.5, 18, 14.25},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewCache()
			for i := 1; i <= tt.count; i++ {
				cache.Record(target, ProbeResult{
					ClientRTT: time.Duration(i) * time.Millisecond,
				}, nil, now.Add(-time.Duration(tt.count-i)*time.Second))
			}
			ranked := cache.Rank([]Target{target}, SearchModePing, 1, now)
			if len(ranked) != 1 || ranked[0].Score != tt.score {
				t.Fatalf("ranking = %#v, want score %v", ranked, tt.score)
			}
			metrics := ranked[0].Metrics
			if metrics.Samples != tt.count || metrics.Successes != tt.count ||
				metrics.Median != tt.median || metrics.P90 != tt.p90 ||
				metrics.Max != float64(tt.count) {
				t.Fatalf("distribution = %#v, want %d successes, median %v and P90 %v",
					metrics, tt.count, tt.median, tt.p90)
			}
			stability := cache.Rank([]Target{target}, SearchModeStability, 1, now)
			if len(stability) != 1 || stability[0].Score != tt.p90 {
				t.Fatalf("stability ranking = %#v, want nearest-rank P90 %v", stability, tt.p90)
			}
		})
	}
}

func TestCacheRankAndMetricsFilterInvalidOrExpiredSamples(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	target := testTarget("192.0.2.10:29450")
	invalidOnly := testTarget("192.0.2.11:29450")
	empty := testTarget("192.0.2.12:29450")
	missing := testTarget("192.0.2.13:29450")
	samples := []Sample{
		{MeasuredAt: now.Add(-RecentHistoryTTL), ClientRTTMicros: 2000, ServerRTTMillis: 4},
		{MeasuredAt: now},
		{MeasuredAt: now.Add(-time.Second), TimedOut: true},
		{MeasuredAt: now, ClientRTTMicros: -1, ServerRTTMillis: 1},
		{MeasuredAt: now, ClientRTTMicros: 1000, ServerRTTMillis: -1},
		{MeasuredAt: now, ClientRTTMicros: -1, TimedOut: true},
		{MeasuredAt: now.Add(time.Nanosecond), ClientRTTMicros: 1},
		{MeasuredAt: now.Add(-RecentHistoryTTL - time.Nanosecond), ClientRTTMicros: 1},
		{ClientRTTMicros: 1},
	}
	cache := NewCache()
	cache.Endpoints[target.Key()] = EndpointHistory{Samples: samples}
	cache.Endpoints[invalidOnly.Key()] = EndpointHistory{Samples: samples[3:]}
	cache.Endpoints[empty.Key()] = EndpointHistory{}

	metrics, ok := cache.Metrics(target.Region, target.Endpoint.Address, now)
	if !ok || metrics.Samples != 3 || metrics.Successes != 2 || metrics.LossRate != 1.0/3.0 ||
		metrics.ClientRTT != 1 || metrics.ServerRTT != 2 || metrics.Ping != 3 ||
		metrics.Median != 3 || metrics.P90 != 6 || metrics.Jitter != 6 {
		t.Fatalf("filtered metrics = %#v, want 2/3 successes and total median/mean 3, P90/jitter 6", metrics)
	}
	for _, excluded := range []Target{invalidOnly, empty, missing} {
		if _, ok := cache.Metrics(excluded.Region, excluded.Endpoint.Address, now); ok {
			t.Fatalf("%s produced metrics without valid recent samples", excluded.Endpoint.Address)
		}
	}
	ranked := cache.Rank([]Target{invalidOnly, empty, missing, target}, SearchModePing, 1, now)
	want := 4.5 + float64(ProbeTimeout/time.Millisecond)/3
	if len(ranked) != 1 || ranked[0].Target.Key() != target.Key() ||
		math.Abs(ranked[0].Score-want) > 1e-9 || ranked[0].Metrics != metrics {
		t.Fatalf("filtered ranking = %#v, want only valid target with score %v and matching history metrics",
			ranked, want)
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

func TestCacheRankRejectsInvalidWeightsAndOverflow(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	target := testTarget("192.0.2.10:29450")
	cache := NewCache()
	cache.Record(target, ProbeResult{
		ClientRTT: 2 * time.Millisecond,
		ServerRTT: 3 * time.Millisecond,
	}, nil, now)

	for _, tt := range []struct {
		name   string
		weight float64
	}{
		{"negative", -1},
		{"nan", math.NaN()},
		{"positive infinity", math.Inf(1)},
		{"negative infinity", math.Inf(-1)},
		{"weighted latency overflow", math.MaxFloat64},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if ranked := cache.Rank([]Target{target}, SearchModeGame, tt.weight, now); len(ranked) != 0 {
				t.Fatalf("invalid weighted latency was ranked: %#v", ranked)
			}
		})
	}
	ranked := cache.Rank([]Target{target}, SearchModeGame, 0, now)
	if len(ranked) != 1 || ranked[0].Score != 3 {
		t.Fatalf("zero client weight ranking = %#v, want server-only score 3", ranked)
	}
}

func TestRepeatedMeasurementsDoNotAccumulateRTT(t *testing.T) {
	t.Parallel()

	cache := NewCache()
	target := testTarget("192.0.2.10:29450")
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	for round := range MaxHistory + 5 {
		measuredAt := now.Add(time.Duration(round) * time.Second)
		cache.Record(target, ProbeResult{
			ClientRTT: 10 * time.Millisecond,
			ServerRTT: 15 * time.Millisecond,
		}, nil, measuredAt)
		metrics, ok := cache.Metrics(target.Region, target.Endpoint.Address, measuredAt)
		if !ok || metrics.ClientRTT != 10 || metrics.ServerRTT != 15 ||
			metrics.Ping != 25 || metrics.P90 != 25 || metrics.Jitter != 0 ||
			metrics.LossRate != 0 {
			t.Fatalf("round %d metrics = %#v, want unchanged 25ms RTT", round+1, metrics)
		}
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
