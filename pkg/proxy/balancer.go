package proxy

import (
	"math"
	"math/rand"
	"strconv"
	"sync"
	"time"

	"github.com/DashNode-Org/sentinel-proxy/config"
	"github.com/DashNode-Org/sentinel-proxy/pkg/metrics"
	"github.com/rs/zerolog/log"
)

const (
	LatencyWindowSize = 100
)

type RequestStats struct {
	AvgLatency     time.Duration   `json:"avgLatency"`
	MaxLatency     time.Duration   `json:"maxLatency"`
	MinLatency     time.Duration   `json:"minLatency"`
	TotalRequests  int64           `json:"totalRequests"`
	TotalErrors    int64           `json:"totalErrors"`
	LatencyHistory []time.Duration `json:"-"` // Hidden from JSON
}

type IntegrityStats struct {
	MissingEpochs      []int64 `json:"missingEpochs"`
	InconsistentEpochs []int64 `json:"inconsistentEpochs"`
	Score              int     `json:"integrityScore"`
	Status             string  `json:"integrityStatus"`
	Priority           float64 `json:"priority"`
}

type EpochStats struct {
	CurrentEpoch      int `json:"currentEpoch"`
	TotalEpochs       int `json:"totalEpochs"`
	OldestSlot        int `json:"oldestSlot"`
	LastProcessedSlot int `json:"lastProcessedSlot"`
}

type Backend struct {
	URL            string          `json:"url"`
	Healthy        bool            `json:"healthy"`
	BlockNumber    int             `json:"blockNumber"`
	LastChecked    time.Time       `json:"lastCheck"`
	NodeType       string          `json:"nodeType"`
	IntegrityStats *IntegrityStats `json:"integrityStats"`
	EpochStats     *EpochStats     `json:"epochStats"`
	RequestStats   *RequestStats   `json:"requestStats"`
}

type LoadBalancer struct {
	cfg      *config.Config
	backends []*Backend
	mu       sync.RWMutex
}

func NewLoadBalancer(cfg *config.Config) *LoadBalancer {
	var backends []*Backend
	for _, url := range cfg.SentinelBackends {
		backends = append(backends, &Backend{
			URL:         url,
			Healthy:     true, // Assume healthy at start
			LastChecked: time.Now(),
			IntegrityStats: &IntegrityStats{
				Score:    100,
				Priority: 100, // Base priority
			},
			RequestStats: &RequestStats{},
			EpochStats:   &EpochStats{},
		})
	}
	return &LoadBalancer{
		cfg:      cfg,
		backends: backends,
	}
}

func (lb *LoadBalancer) GetBackends() []*Backend {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	return lb.backends
}

func (lb *LoadBalancer) GetNextBackend() *Backend {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	backends := lb.filterHealthy()
	if len(backends) == 0 {
		return nil
	}

	return lb.selectWeighted(backends)
}

func (lb *LoadBalancer) GetArchiverBackend() *Backend {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	// Filter for healthy archivers
	var candidates []*Backend
	for _, b := range lb.backends {
		if b.Healthy && b.NodeType == "archiver" {
			candidates = append(candidates, b)
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	return lb.selectWeighted(candidates)
}

func (lb *LoadBalancer) GetPrunedBackend() *Backend {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	// Filter for healthy pruned nodes
	var candidates []*Backend
	for _, b := range lb.backends {
		if b.Healthy && b.NodeType == "pruned" {
			candidates = append(candidates, b)
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	return lb.selectWeighted(candidates)
}

func (lb *LoadBalancer) UpdateBackendStateByUrl(url string, updateOp func(*Backend)) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	for _, b := range lb.backends {
		if b.URL == url {
			updateOp(b)
			lb.computePriority(b)
			lb.updateMetrics(b)
			return
		}
	}
}

func (lb *LoadBalancer) UpdateBackendHealth(url string, healthy bool, blockNumber int, latency time.Duration) {
	lb.UpdateBackendStateByUrl(url, func(b *Backend) {
		b.Healthy = healthy
		b.BlockNumber = blockNumber
		b.LastChecked = time.Now()

		// Ensure stats exist
		if b.RequestStats == nil {
			b.RequestStats = &RequestStats{}
		}

		// Record health check latency as a sample
		b.RequestStats.recordLatency(latency)

		if !healthy {
			log.Warn().Str("url", url).Msg("Backend marked unhealthy")
		}
	})
}

func (lb *LoadBalancer) UpdateIntegrityScore(url string, score int, missing []int64, inconsistent []int64) {
	lb.UpdateBackendStateByUrl(url, func(b *Backend) {
		if b.IntegrityStats == nil {
			b.IntegrityStats = &IntegrityStats{}
		}
		b.IntegrityStats.Score = score
		b.IntegrityStats.MissingEpochs = missing
		b.IntegrityStats.InconsistentEpochs = inconsistent
	})
}

func (lb *LoadBalancer) IncSuccessfulRequest(b *Backend, status int, latency time.Duration) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	if b.RequestStats == nil {
		b.RequestStats = &RequestStats{}
	}
	b.RequestStats.TotalRequests++
	b.RequestStats.recordLatency(latency)
	metrics.RecordRequest("proxy", strconv.Itoa(status), b.URL)
}

func (lb *LoadBalancer) IncErrorRequest(b *Backend) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	if b.RequestStats == nil {
		b.RequestStats = &RequestStats{}
	}
	b.RequestStats.TotalRequests++
	b.RequestStats.TotalErrors++
	metrics.RecordRequest("proxy", "502", b.URL)
}

// recordLatency adds a new latency sample and recalculates stats
func (rs *RequestStats) recordLatency(d time.Duration) {
	if rs.LatencyHistory == nil {
		rs.LatencyHistory = make([]time.Duration, 0, LatencyWindowSize)
	}

	if len(rs.LatencyHistory) < LatencyWindowSize {
		rs.LatencyHistory = append(rs.LatencyHistory, d)
	} else {
		// Simple shift behavior for now (or could use ring buffer index)
		// For simplicity/readability, let's append and slice.
		// Optimized: Copy could be faster but N=100 is small.
		rs.LatencyHistory = append(rs.LatencyHistory[1:], d)
	}

	// Recalculate
	var total time.Duration
	var min, max time.Duration
	if len(rs.LatencyHistory) > 0 {
		min = rs.LatencyHistory[0]
		max = rs.LatencyHistory[0]
	}

	for _, l := range rs.LatencyHistory {
		total += l
		if l < min {
			min = l
		}
		if l > max {
			max = l
		}
	}

	rs.AvgLatency = total / time.Duration(len(rs.LatencyHistory))
	rs.MinLatency = min
	rs.MaxLatency = max
}

func (lb *LoadBalancer) updateMetrics(b *Backend) {
	metrics.SetBackendHealth(b.URL, b.Healthy)
	metrics.SetBackendBlockNumber(b.URL, b.BlockNumber)
	if b.IntegrityStats != nil {
		metrics.SetBackendIntegrity(b.URL, b.IntegrityStats.Score)
	}
}

// selectWeighted selects a backend from a list of candidates using weighted logic
// Assumes lock is NOT held (or logic is safe) but caller usually holds lock.
func (lb *LoadBalancer) selectWeighted(candidates []*Backend) *Backend {
	if len(candidates) == 1 {
		return candidates[0]
	}

	if candidates[0].IntegrityStats == nil {
		return candidates[0] // Fallback
	}
	minPriority := candidates[0].IntegrityStats.Priority
	for _, b := range candidates {
		if b.IntegrityStats != nil && b.IntegrityStats.Priority < minPriority {
			minPriority = b.IntegrityStats.Priority
		}
	}

	var totalWeight float64
	weights := make([]float64, len(candidates))
	for i, b := range candidates {
		// Weight calculation: distance from minPriority + base
		prio := 100.0
		if b.IntegrityStats != nil {
			prio = b.IntegrityStats.Priority
		}
		w := math.Max(1, prio-minPriority+10)
		weights[i] = w
		totalWeight += w
	}

	r := rand.Float64() * totalWeight

	var cumulative float64
	for i, w := range weights {
		cumulative += w
		if r < cumulative {
			return candidates[i]
		}
	}

	return candidates[0]
}

func (lb *LoadBalancer) filterHealthy() []*Backend {
	var healthy []*Backend
	for _, b := range lb.backends {
		if b.Healthy {
			healthy = append(healthy, b)
		}
	}
	return healthy
}

func (lb *LoadBalancer) computePriority(b *Backend) {
	// Base priority
	priority := 100.0

	// Use IntegrityStats
	if b.IntegrityStats == nil {
		b.IntegrityStats = &IntegrityStats{Priority: 100.0}
	}

	// Penalties
	priority -= float64(len(b.IntegrityStats.MissingEpochs) * 10)
	priority -= float64(len(b.IntegrityStats.InconsistentEpochs) * 5)

	// User removed Latency field. Using RequestStats.AvgLatency instead if needed.
	if b.RequestStats != nil && b.RequestStats.AvgLatency > 0 {
		ms := float64(b.RequestStats.AvgLatency.Milliseconds())
		if ms < 100 {
			priority += 10
		} else if ms < 500 {
			priority += 5
		}
	}

	// Health bonus
	if b.Healthy {
		priority += 20
	}

	b.IntegrityStats.Priority = priority
}
