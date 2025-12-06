package health

import (
	"context"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/DashNode-Org/sentinel-proxy/config"
	"github.com/DashNode-Org/sentinel-proxy/pkg/integrity"
	"github.com/DashNode-Org/sentinel-proxy/pkg/proxy"
	"github.com/DashNode-Org/sentinel-proxy/pkg/rpc"
	"github.com/rs/zerolog/log"
)

type IntegrityChecker struct {
	cfg           *config.Config
	lb            *proxy.LoadBalancer
	clientFactory func(url string, timeout time.Duration) rpc.RPCClient
}

func NewIntegrityChecker(cfg *config.Config, lb *proxy.LoadBalancer) *IntegrityChecker {
	return &IntegrityChecker{
		cfg: cfg,
		lb:  lb,
		clientFactory: func(url string, timeout time.Duration) rpc.RPCClient {
			return rpc.NewClient(url, timeout)
		},
	}
}

// WithClientFactory allows injecting a mock factory for testing
func (c *IntegrityChecker) WithClientFactory(f func(url string, timeout time.Duration) rpc.RPCClient) *IntegrityChecker {
	c.clientFactory = f
	return c
}

func (c *IntegrityChecker) Start() {
	go func() {
		ticker := time.NewTicker(c.cfg.IntegrityCheckInterval)
		for range ticker.C {
			c.CheckIntegrity()
		}
	}()
	c.CheckIntegrity()
}

func (c *IntegrityChecker) CheckIntegrity() {
	backends := c.lb.GetBackends()
	var wg sync.WaitGroup

	for _, b := range backends {
		if !b.Healthy {
			continue
		}
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			c.checkBackendIntegrity(url)
		}(b.URL)
	}
	wg.Wait()
}

func (c *IntegrityChecker) checkBackendIntegrity(url string) {
	client := c.clientFactory(url, c.cfg.RequestTimeout)
	stats, err := client.GetValidatorsStats(context.Background())
	if err != nil {
		log.Error().Err(err).Str("url", url).Msg("Failed to fetch validator stats")
		return
	}

	epochRecords, epochs, oldestSlot := c.processStats(stats)
	totalEpochs := len(epochs)

	if totalEpochs == 0 {
		return
	}

	// Logic to check only last N epochs
	checkCount := c.cfg.IntegrityCheckEpochs
	if totalEpochs > checkCount {
		epochs = epochs[totalEpochs-checkCount:]
	}

	missingEpochs := []int64{}
	inconsistentEpochs := []int64{}

	minEpoch := epochs[0]
	maxEpoch := epochs[len(epochs)-1]
	epochSet := make(map[int64]bool)
	for _, e := range epochs {
		epochSet[e] = true
	}

	// Check for gaps
	for e := minEpoch; e <= maxEpoch; e++ {
		if !epochSet[e] {
			missingEpochs = append(missingEpochs, e)
		}
	}

	// Check integrity
	currentSlot, _ := strconv.Atoi(stats.LastProcessedSlot)
	currentEpoch := int64(currentSlot / c.cfg.SlotsPerEpoch)

	// Calculate overall integrity
	currentTotalIntegrity := 0
	currentTotalIntegrityCount := 0
	currentAvgIntegrity := 100

	for _, epoch := range epochs {
		if epoch >= currentEpoch {
			continue
		}

		// Skip the first epoch if total epochs is less than integrity check epochs
		if totalEpochs <= c.cfg.IntegrityCheckEpochs && epoch == minEpoch {
			break
		}

		records := epochRecords[epoch]
		result := integrity.AnalyzeEpochIntegrity(integrity.EpochAnalysisInput{
			EpochNumber:                epoch,
			Records:                    records,
			ExpectedValidatorsPerEpoch: c.cfg.ExpectedValidators,
			SlotsPerEpoch:              c.cfg.SlotsPerEpoch,
		})

		if result.IntegrityScore < 100 {
			inconsistentEpochs = append(inconsistentEpochs, epoch)
		}

		currentTotalIntegrity += result.IntegrityScore
		currentTotalIntegrityCount++
		currentAvgIntegrity = currentTotalIntegrity / currentTotalIntegrityCount
	}

	c.lb.UpdateBackendStateByUrl(url, func(b *proxy.Backend) {
		if b.IntegrityStats == nil {
			b.IntegrityStats = &proxy.IntegrityStats{}
		}
		if b.EpochStats == nil {
			b.EpochStats = &proxy.EpochStats{}
		}

		b.IntegrityStats.Score = currentAvgIntegrity
		b.IntegrityStats.MissingEpochs = missingEpochs
		b.IntegrityStats.InconsistentEpochs = inconsistentEpochs
		b.EpochStats.TotalEpochs = totalEpochs
		b.NodeType = "pruned"
		if b.EpochStats.TotalEpochs > c.cfg.ArchiverThresholdEpochs {
			b.NodeType = "archiver"
		}

		// Additional stats
		b.EpochStats.LastProcessedSlot, _ = strconv.Atoi(stats.LastProcessedSlot)
		b.EpochStats.CurrentEpoch = int(currentEpoch)
		b.EpochStats.OldestSlot = int(oldestSlot)

		if currentAvgIntegrity == 100 {
			b.IntegrityStats.Status = "perfect"
		} else if currentAvgIntegrity > c.cfg.IntegrityScoreThreshold {
			b.IntegrityStats.Status = "good"
		} else {
			b.IntegrityStats.Status = "bad"
		}
	})

	log.Info().
		Str("url", url).
		Int("score", currentAvgIntegrity).
		Int("missing", len(missingEpochs)).
		Int("inconsistent", len(inconsistentEpochs)).
		Msg("Integrity check completed")
}

func (c *IntegrityChecker) processStats(stats *rpc.GetValidatorsStatsResponse) (map[int64][]integrity.SlotRecord, []int64, int64) {
	epochRecords := make(map[int64][]integrity.SlotRecord)
	slotsPerEpoch := int64(c.cfg.SlotsPerEpoch)
	oldestSlot, _ := strconv.ParseInt(stats.LastProcessedSlot, 10, 64)

	for addr, validator := range stats.Stats {
		for _, item := range validator.History {
			slot, _ := strconv.ParseInt(item.Slot, 10, 64)
			epoch := slot / slotsPerEpoch

			if slot < oldestSlot {
				oldestSlot = slot
			}

			record := integrity.SlotRecord{
				Slot:      item.Slot,
				Status:    item.Status,
				Validator: addr,
			}
			epochRecords[epoch] = append(epochRecords[epoch], record)
		}
	}

	var epochs []int64
	for e := range epochRecords {
		epochs = append(epochs, e)
	}
	sort.Slice(epochs, func(i, j int) bool { return epochs[i] < epochs[j] })

	return epochRecords, epochs, oldestSlot
}
