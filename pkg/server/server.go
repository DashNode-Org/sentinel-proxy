package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/DashNode-Org/sentinel-proxy/config"
	"github.com/DashNode-Org/sentinel-proxy/pkg/proxy"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
)

type Server struct {
	cfg       *config.Config
	lb        *proxy.LoadBalancer
	forwarder *proxy.Forwarder
	router    *chi.Mux
	startTime time.Time
	httpSrv   *http.Server
}

func NewServer(cfg *config.Config, lb *proxy.LoadBalancer, forwarder *proxy.Forwarder) *Server {
	return &Server{
		cfg:       cfg,
		lb:        lb,
		forwarder: forwarder,
		router:    chi.NewRouter(),
		startTime: time.Now(),
	}
}

func (s *Server) Start() error {
	s.setupMiddleware()
	s.setupRoutes()

	s.httpSrv = &http.Server{
		Addr:    ":" + strconv.Itoa(s.cfg.ProxyPort),
		Handler: s.router,
	}

	log.Info().Msgf("Starting server on port %d", s.cfg.ProxyPort)
	return s.httpSrv.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpSrv != nil {
		return s.httpSrv.Shutdown(ctx)
	}
	return nil
}

// GetHandler returns the http.Handler for testing or custom usage
func (s *Server) GetHandler() http.Handler {
	s.setupMiddleware()
	s.setupRoutes()
	return s.router
}

func (s *Server) setupMiddleware() {
	s.router.Use(middleware.RequestID)
	s.router.Use(middleware.RealIP)
	s.router.Use(middleware.Logger)
	s.router.Use(middleware.Recoverer)
	s.router.Use(middleware.Timeout(s.cfg.RequestTimeout + 5*time.Second))
}

func (s *Server) setupRoutes() {
	// Prometheus Metrics Endpoint
	s.router.Handle("/metrics", promhttp.Handler())

	// Health Check
	s.router.Get("/health", s.handleHealth)

	// Readiness Check
	s.router.Get("/ready", s.handleReady)

	// Archiver Handler
	s.router.Post("/archiver", func(w http.ResponseWriter, r *http.Request) {
		s.forwarder.ForwardArchiver(w, r)
	})

	// Pruned Handler
	s.router.Post("/pruned", func(w http.ResponseWriter, r *http.Request) {
		s.forwarder.ForwardPruned(w, r)
	})

	// Default Proxy Handler (Any healthy)
	s.router.Post("/", func(w http.ResponseWriter, r *http.Request) {
		s.forwarder.Forward(w, r)
	})

	// Dashboard
	workDir, _ := os.Getwd()
	filesDir := http.Dir(filepath.Join(workDir, "public"))
	FileServer(s.router, "/dashboard", filesDir)
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if len(s.lb.GetBackends()) > 0 {
		w.Write([]byte("READY"))
	} else {
		http.Error(w, "Not Ready", http.StatusServiceUnavailable)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	backends := s.lb.GetBackends()
	healthyCount := 0
	for _, b := range backends {
		if b.Healthy {
			healthyCount++
		}
	}

	status := "unhealthy"
	if healthyCount > 0 {
		status = "healthy"
	}

	totalRequests := sum(backends, func(b *proxy.Backend) int64 {
		if b.RequestStats != nil {
			return b.RequestStats.TotalRequests
		}
		return 0
	})

	totalErrors := sum(backends, func(b *proxy.Backend) int64 {
		if b.RequestStats != nil {
			return b.RequestStats.TotalErrors
		}
		return 0
	})

	response := map[string]interface{}{
		"status":   status,
		"uptime":   time.Since(s.startTime).Seconds(),
		"backends": backends,
		"metrics": map[string]interface{}{
			"totalRequests": totalRequests,
			"totalErrors":   totalErrors,
			"errorRate":     safeDiv(totalErrors, totalRequests),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// FileServer conveniently sets up a http.FileServer handler to serve
// static files from a http.FileSystem.
func FileServer(r chi.Router, path string, root http.FileSystem) {
	if strings.ContainsAny(path, "{}*") {
		panic("FileServer does not permit any URL parameters.")
	}

	if path != "/" && path[len(path)-1] != '/' {
		r.Get(path, http.RedirectHandler(path+"/", http.StatusMovedPermanently).ServeHTTP)
		path += "/"
	}
	path += "*"

	r.Get(path, func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.RouteContext(r.Context())
		pathPrefix := strings.TrimSuffix(rctx.RoutePattern(), "/*")
		fs := http.StripPrefix(pathPrefix, http.FileServer(root))
		fs.ServeHTTP(w, r)
	})
}

// Helper functions for metrics aggregation
func sum(backends []*proxy.Backend, extractor func(*proxy.Backend) int64) int64 {
	var total int64
	for _, b := range backends {
		total += extractor(b)
	}
	return total
}

func safeDiv(numerator, denominator int64) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}
