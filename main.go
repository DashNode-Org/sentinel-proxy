package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DashNode-Org/sentinel-proxy/config"
	"github.com/DashNode-Org/sentinel-proxy/pkg/health"
	"github.com/DashNode-Org/sentinel-proxy/pkg/metrics"
	"github.com/DashNode-Org/sentinel-proxy/pkg/proxy"
	"github.com/DashNode-Org/sentinel-proxy/pkg/server"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	// Load .env file if it exists
	_ = godotenv.Load()

	// Load config
	cfg := config.Load()

	// Register metrics
	metrics.Register()

	// Setup logging
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	level, err := zerolog.ParseLevel(cfg.LogLevel)
	if err == nil {
		zerolog.SetGlobalLevel(level)
	}

	// Initialize Load Balancer and Health Checkers
	lb := proxy.NewLoadBalancer(cfg)

	hc := health.NewChecker(cfg, lb)
	go hc.Start()

	ic := health.NewIntegrityChecker(cfg, lb)
	go ic.Start()

	forwarder := proxy.NewRequestForwarder(cfg, lb)

	// Initialize and Start Server
	srv := server.NewServer(cfg, lb, forwarder)

	go func() {
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Server startup failed")
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info().Msg("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("Server forced to shutdown")
	}

	log.Info().Msg("Server exited properly")
}
