package tunnel

import (
	"math"
	"sort"
	"time"
)

const (
	MaxHistory       = 20
	RecentHistoryTTL = time.Hour
	CacheTTL         = 24 * time.Hour
)

type Sample struct {
	MeasuredAt      time.Time `json:"measured_at"`
	ClientRTTMicros int64     `json:"client_rtt_micros,omitempty"`
	ServerRTTMillis int64     `json:"server_rtt_millis,omitempty"`
	TimedOut        bool      `json:"timed_out,omitempty"`
}

type EndpointHistory struct {
	Region                 Region    `json:"region"`
	Pool                   string    `json:"pool"`
	Name                   string    `json:"name"`
	Address                string    `json:"address"`
	Samples                []Sample  `json:"samples,omitempty"`
	IsLastTimeLimitReached bool      `json:"is_last_time_limit_reached,omitempty"`
	LastLimitCheckedAt     time.Time `json:"last_limit_checked_at,omitempty"`
}

type Cache struct {
	Version   int                        `json:"version"`
	Endpoints map[string]EndpointHistory `json:"endpoints"`
}

// Metrics summarizes recent history in milliseconds. ClientRTT, ServerRTT, and
// Ping are arithmetic means; Median, P90, and Max summarize paired total RTTs.
type Metrics struct {
	Samples   int
	Successes int
	LossRate  float64
	ClientRTT float64
	ServerRTT float64
	Ping      float64
	Median    float64
	P90       float64
	Max       float64
	Jitter    float64
}

type RankedTarget struct {
	Target  Target
	Metrics Metrics
	Score   float64
}

func NewCache() Cache {
	return Cache{
		Version:   1,
		Endpoints: make(map[string]EndpointHistory),
	}
}

func (c Cache) Clone() Cache {
	clone := Cache{
		Version:   c.Version,
		Endpoints: make(map[string]EndpointHistory, len(c.Endpoints)),
	}
	for key, history := range c.Endpoints {
		history.Samples = append([]Sample(nil), history.Samples...)
		clone.Endpoints[key] = history
	}
	clone.Normalize()
	return clone
}

func (c *Cache) Normalize() {
	if c.Version == 0 {
		c.Version = 1
	}
	if c.Endpoints == nil {
		c.Endpoints = make(map[string]EndpointHistory)
	}
}

func (c *Cache) Record(
	target Target,
	result ProbeResult,
	probeErr error,
	measuredAt time.Time,
) {
	c.Normalize()
	key := target.Key()
	history := c.Endpoints[key]
	history.Region = target.Region
	history.Pool = target.Pool
	history.Name = target.Endpoint.Name
	history.Address = target.Endpoint.Address

	sample := Sample{
		MeasuredAt: measuredAt.UTC(),
		TimedOut:   probeErr != nil,
	}
	if probeErr == nil {
		sample.ClientRTTMicros = result.ClientRTT.Microseconds()
		sample.ServerRTTMillis = result.ServerRTT.Milliseconds()
		history.IsLastTimeLimitReached = result.LimitReached
		history.LastLimitCheckedAt = measuredAt.UTC()
	}
	history.Samples = append(history.Samples, sample)
	if len(history.Samples) > MaxHistory {
		history.Samples = append([]Sample(nil), history.Samples[len(history.Samples)-MaxHistory:]...)
	}
	c.Endpoints[key] = history
}

func (c Cache) History(region Region, address string) (EndpointHistory, bool) {
	history, ok := c.Endpoints[CacheKey(region, address)]
	if !ok {
		return EndpointHistory{}, false
	}
	history.Samples = append([]Sample(nil), history.Samples...)
	return history, true
}

func (c Cache) Metrics(region Region, address string, now time.Time) (Metrics, bool) {
	history, ok := c.Endpoints[CacheKey(region, address)]
	if !ok {
		return Metrics{}, false
	}
	recent := recentSamples(history.Samples, now, RecentHistoryTTL)
	metrics := calculateMetrics(recent, make([]float64, 0, len(recent)))
	return metrics, metrics.Samples > 0
}

func (c Cache) WasRecentlyLimited(region Region, address string, now time.Time) bool {
	history, ok := c.Endpoints[CacheKey(region, address)]
	if !ok || !history.IsLastTimeLimitReached || history.LastLimitCheckedAt.IsZero() {
		return false
	}
	age := now.Sub(history.LastLimitCheckedAt)
	return age >= 0 && age <= CacheTTL
}

// Prioritize returns a copy with endpoints that most recently reported a
// connection limit first. The order within both groups remains unchanged.
func (c Cache) Prioritize(targets []Target, now time.Time) []Target {
	prioritized := make([]Target, 0, len(targets))
	regular := make([]Target, 0, len(targets))
	for _, target := range targets {
		if c.WasRecentlyLimited(target.Region, target.Endpoint.Address, now) {
			prioritized = append(prioritized, target)
			continue
		}
		regular = append(regular, target)
	}
	return append(prioritized, regular...)
}

func (c *Cache) Prune(now time.Time) {
	c.Normalize()
	for key, history := range c.Endpoints {
		samples := history.Samples[:0]
		for _, sample := range history.Samples {
			age := now.Sub(sample.MeasuredAt)
			if sample.MeasuredAt.IsZero() ||
				age < 0 ||
				age > CacheTTL ||
				!sample.Valid() {
				continue
			}
			samples = append(samples, sample)
		}
		sort.Slice(samples, func(i, j int) bool {
			return samples[i].MeasuredAt.Before(samples[j].MeasuredAt)
		})
		if len(samples) > MaxHistory {
			samples = samples[len(samples)-MaxHistory:]
		}
		history.Samples = append([]Sample(nil), samples...)
		if !history.LastLimitCheckedAt.IsZero() {
			limitAge := now.Sub(history.LastLimitCheckedAt)
			if limitAge < 0 || limitAge > CacheTTL {
				history.IsLastTimeLimitReached = false
				history.LastLimitCheckedAt = time.Time{}
			}
		}
		if len(history.Samples) == 0 && history.LastLimitCheckedAt.IsZero() {
			delete(c.Endpoints, key)
			continue
		}
		c.Endpoints[key] = history
	}
}

// Rank uses paired recent samples and an additive failed-probe penalty.
// Scores are selection costs, not measured game ping.
func (c Cache) Rank(
	targets []Target,
	mode SearchMode,
	clientWeight float64,
	now time.Time,
) []RankedTarget {
	if mode == SearchModeGame &&
		(clientWeight < 0 || math.IsNaN(clientWeight) || math.IsInf(clientWeight, 0)) {
		return nil
	}

	ranked := make([]RankedTarget, 0, len(targets))
	for _, target := range targets {
		history, ok := c.Endpoints[target.Key()]
		if !ok {
			continue
		}
		recent := recentSamples(history.Samples, now, RecentHistoryTTL)
		latencies := make([]float64, 0, len(recent))
		metrics := calculateMetrics(recent, latencies)
		if metrics.Successes == 0 {
			continue
		}

		median, p90 := metrics.Median, metrics.P90
		switch mode {
		case SearchModeClientRTT, SearchModeServerRTT, SearchModeGame:
			valid := true
			for _, sample := range recent {
				if sample.TimedOut {
					continue
				}
				var latency float64
				switch mode {
				case SearchModeClientRTT:
					latency = float64(sample.ClientRTTMicros) / 1000
				case SearchModeServerRTT:
					latency = float64(sample.ServerRTTMillis)
				case SearchModeGame:
					latency = float64(sample.ClientRTTMicros)/1000*clientWeight +
						float64(sample.ServerRTTMillis)
				}
				if latency < 0 || math.IsNaN(latency) || math.IsInf(latency, 0) {
					valid = false
					break
				}
				latencies = append(latencies, latency)
			}
			if !valid {
				continue
			}
			median, p90 = latencyQuantiles(latencies)
		}

		score := median + 0.5*(p90-median)
		if mode == SearchModeStability {
			score = p90
		}
		score += float64(ProbeTimeout/time.Millisecond) * metrics.LossRate
		if score < 0 || math.IsNaN(score) || math.IsInf(score, 0) {
			continue
		}
		ranked = append(ranked, RankedTarget{
			Target:  target,
			Metrics: metrics,
			Score:   score,
		})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Score == ranked[j].Score {
			return ranked[i].Target.Endpoint.Name < ranked[j].Target.Endpoint.Name
		}
		return ranked[i].Score < ranked[j].Score
	})
	return ranked
}

func recentSamples(samples []Sample, now time.Time, maxAge time.Duration) []Sample {
	recent := make([]Sample, 0, len(samples))
	for _, sample := range samples {
		if sample.MeasuredAt.IsZero() || !sample.Valid() {
			continue
		}
		age := now.Sub(sample.MeasuredAt)
		if age < 0 || age > maxAge {
			continue
		}
		recent = append(recent, sample)
	}
	sort.Slice(recent, func(i, j int) bool {
		return recent[i].MeasuredAt.Before(recent[j].MeasuredAt)
	})
	return recent
}

// calculateMetrics uses totals as scratch space and does not retain it.
func calculateMetrics(recent []Sample, totals []float64) Metrics {
	metrics := Metrics{Samples: len(recent)}
	if metrics.Samples == 0 {
		return metrics
	}
	totals = totals[:0]
	var previous float64
	var jitterTotal float64
	var jitterSamples int

	for _, sample := range recent {
		if sample.TimedOut {
			continue
		}
		client := float64(sample.ClientRTTMicros) / 1000
		server := float64(sample.ServerRTTMillis)
		total := client + server
		metrics.Successes++
		metrics.ClientRTT += client
		metrics.ServerRTT += server
		totals = append(totals, total)
		if metrics.Successes > 1 {
			jitterTotal += math.Abs(total - previous)
			jitterSamples++
		}
		previous = total
	}

	metrics.LossRate = float64(metrics.Samples-metrics.Successes) / float64(metrics.Samples)
	if metrics.Successes == 0 {
		return metrics
	}
	metrics.ClientRTT /= float64(metrics.Successes)
	metrics.ServerRTT /= float64(metrics.Successes)
	metrics.Ping = metrics.ClientRTT + metrics.ServerRTT
	if jitterSamples > 0 {
		metrics.Jitter = jitterTotal / float64(jitterSamples)
	}

	metrics.Median, metrics.P90 = latencyQuantiles(totals)
	metrics.Max = totals[len(totals)-1]
	return metrics
}

// latencyQuantiles sorts scratch values, leaving the recorded samples intact.
// Callers supply at least one successful sample.
func latencyQuantiles(values []float64) (median, p90 float64) {
	sort.Float64s(values)
	middle := len(values) / 2
	median = values[middle]
	if len(values)%2 == 0 {
		lower := values[middle-1]
		median = lower + (median-lower)/2
	}
	return median, values[int(math.Ceil(float64(len(values))*0.9))-1]
}

func (s Sample) Valid() bool {
	return s.ClientRTTMicros >= 0 && s.ServerRTTMillis >= 0
}
