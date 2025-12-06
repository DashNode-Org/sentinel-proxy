package proxy

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/DashNode-Org/sentinel-proxy/config"
	"github.com/stretchr/testify/assert"
)

func TestSelectWeightedSimulation(t *testing.T) {
	// Setup candidates with different priorities
	candidates := []*Backend{
		{URL: "node1", IntegrityStats: &IntegrityStats{Priority: 100}}, // Baseline
		{URL: "node2", IntegrityStats: &IntegrityStats{Priority: 120}}, // Better
		{URL: "node3", IntegrityStats: &IntegrityStats{Priority: 80}},  // Worse
	}

	// Calculate expected weights for verification logic (same logic as implementation)
	// Min Prio = 80
	// W1 = 100 - 80 + 10 = 30
	// W2 = 120 - 80 + 10 = 50
	// W3 = 80 - 80 + 10 = 10
	minPriority := 80.0

	// Total = 90
	// Expected Probabilities:
	// Node1: 30/90 = 33.3%
	// Node2: 50/90 = 55.5%
	// Node3: 10/90 = 11.1%
	totalWeight := 90.0

	lb := &LoadBalancer{} // Mock LB

	// Simulation parameters
	iterations := 100000
	counts := make(map[string]int)

	fmt.Printf("\n--- Starting Weighted Selection Simulation (%d iterations) ---\n", iterations)
	startTime := time.Now()

	for i := 0; i < iterations; i++ {
		selected := lb.selectWeighted(candidates)
		counts[selected.URL]++
	}

	duration := time.Since(startTime)
	fmt.Printf("Simulation completed in %v\n\n", duration)

	fmt.Printf("%-10s | %-10s | %-10s | %-15s | %-10s\n", "Node", "Priority", "Weight", "Count", "percent")
	fmt.Println("----------------------------------------------------------------------")

	for _, b := range candidates {
		count := counts[b.URL]
		percent := float64(count) / float64(iterations) * 100

		weight := math.Max(1, b.IntegrityStats.Priority-minPriority+10)
		expectedPercent := weight / totalWeight * 100

		fmt.Printf("%-10s | %-10.0f | %-10.0f | %-15d | %.2f%% (Exp: %.2f%%)\n",
			b.URL, b.IntegrityStats.Priority, weight, count, percent, expectedPercent)
	}
	fmt.Println("----------------------------------------------------------------------")
}

func TestNewLoadBalancer(t *testing.T) {
	cfg := &config.Config{
		SentinelBackends: []string{"http://node1:8545", "http://node2:8545"},
	}
	lb := NewLoadBalancer(cfg)

	assert.Equal(t, 2, len(lb.GetBackends()))
	assert.Equal(t, "http://node1:8545", lb.GetBackends()[0].URL)
	assert.True(t, lb.GetBackends()[0].Healthy)
}

func TestGetNextBackend_NoHealthy(t *testing.T) {
	cfg := &config.Config{SentinelBackends: []string{"http://node1"}}
	lb := NewLoadBalancer(cfg)

	// Mark unhealthy
	lb.UpdateBackendHealth("http://node1", false, 0, 0)

	b := lb.GetNextBackend()
	assert.Nil(t, b)
}

func TestGetArchiverBackend(t *testing.T) {
	cfg := &config.Config{SentinelBackends: []string{"http://archiver", "http://pruned"}}
	lb := NewLoadBalancer(cfg)

	// Manually set node types since we don't have real RPC
	backends := lb.GetBackends()
	backends[0].NodeType = "archiver"
	backends[1].NodeType = "pruned"

	// Should return archiver
	b := lb.GetArchiverBackend()
	assert.NotNil(t, b)
	assert.Equal(t, "http://archiver", b.URL)
}

func TestGetPrunedBackend(t *testing.T) {
	cfg := &config.Config{SentinelBackends: []string{"http://archiver", "http://pruned"}}
	lb := NewLoadBalancer(cfg)

	backends := lb.GetBackends()
	backends[0].NodeType = "archiver"
	backends[1].NodeType = "pruned"

	b := lb.GetPrunedBackend()
	assert.NotNil(t, b)
	assert.Equal(t, "http://pruned", b.URL)
}

func TestUpdateIntegrityScore_Priority(t *testing.T) {
	cfg := &config.Config{SentinelBackends: []string{"http://node1"}}
	lb := NewLoadBalancer(cfg)

	// Initial priority
	initial := lb.GetBackends()[0].IntegrityStats.Priority

	// Simulate integrity failure
	missing := []int64{1, 2, 3}
	lb.UpdateIntegrityScore("http://node1", 80, missing, nil)

	updated := lb.GetBackends()[0].IntegrityStats.Priority
	assert.Less(t, updated, initial, "Priority should decrease with missing epochs")
}

func TestPriorityCalculation(t *testing.T) {
	b := &Backend{
		Healthy:        true,
		RequestStats:   &RequestStats{AvgLatency: 50 * time.Millisecond},
		IntegrityStats: &IntegrityStats{Score: 100},
	}

	lb := &LoadBalancer{}
	lb.computePriority(b)
	base := b.IntegrityStats.Priority

	// Case 1: High Latency penalty
	b.RequestStats.AvgLatency = 600 * time.Millisecond
	lb.computePriority(b)
	assert.Less(t, b.IntegrityStats.Priority, base)

	// Case 2: Unhealthy penalty
	b.Healthy = false
	lb.computePriority(b)
	assert.Less(t, b.IntegrityStats.Priority, base-20) // Health bonus lost
}
