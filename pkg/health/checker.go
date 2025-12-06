package health

import (
	"context"
	"time"

	"github.com/DashNode-Org/sentinel-proxy/config"
	"github.com/DashNode-Org/sentinel-proxy/pkg/proxy"
	"github.com/DashNode-Org/sentinel-proxy/pkg/rpc"
	"github.com/rs/zerolog/log"
)

type Checker struct {
	cfg *config.Config
	lb  *proxy.LoadBalancer
}

func NewChecker(cfg *config.Config, lb *proxy.LoadBalancer) *Checker {
	return &Checker{cfg: cfg, lb: lb}
}

func (c *Checker) Start() {
	go func() {
		ticker := time.NewTicker(c.cfg.HealthCheckInterval)
		for range ticker.C {
			c.CheckAll()
		}
	}()
	c.CheckAll() // Run immediately
}

func (c *Checker) CheckAll() {
	backends := c.lb.GetBackends()
	for _, b := range backends {
		go c.checkBackend(b.URL)
	}
}

func (c *Checker) checkBackend(url string) {
	start := time.Now()
	client := rpc.NewClient(url, c.cfg.RequestTimeout)

	// Check Readiness
	isReady, err := client.IsReady(context.Background())
	if err != nil || !isReady {
		c.lb.UpdateBackendHealth(url, false, 0, time.Since(start))
		return
	}

	// Get Block Number
	blockNum, err := client.GetBlockNumber(context.Background())
	if err != nil {
		c.lb.UpdateBackendHealth(url, false, 0, time.Since(start))
		return
	}

	// Success
	c.lb.UpdateBackendHealth(url, true, blockNum, time.Since(start))
	log.Debug().Str("url", url).Int("block", blockNum).Dur("latency", time.Since(start)).Msg("Health check passed")
}


