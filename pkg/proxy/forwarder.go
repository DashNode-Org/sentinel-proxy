package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/DashNode-Org/sentinel-proxy/config"
	"github.com/DashNode-Org/sentinel-proxy/pkg/metrics"
	"github.com/rs/zerolog/log"
)

// Forwarder handles request forwarding to backends
type Forwarder struct {
	cfg *config.Config
	lb  *LoadBalancer
}

func NewRequestForwarder(cfg *config.Config, lb *LoadBalancer) *Forwarder {
	return &Forwarder{
		cfg: cfg,
		lb:  lb,
	}
}

// Forward forwards the request to any healthy backend
func (f *Forwarder) Forward(w http.ResponseWriter, r *http.Request) {
	backend := f.lb.GetNextBackend()
	if backend == nil {
		metrics.RequestTotal.WithLabelValues("proxy", "503", "none").Inc()
		http.Error(w, "No healthy backends available", http.StatusServiceUnavailable)
		return
	}
	f.forward(w, r, backend)
}

// ForwardArchiver forwards the request to an archiver backend
func (f *Forwarder) ForwardArchiver(w http.ResponseWriter, r *http.Request) {
	backend := f.lb.GetArchiverBackend()
	if backend == nil {
		http.Error(w, "No healthy archiver backend available", http.StatusServiceUnavailable)
		return
	}
	r.URL.Path = "/"
	f.forward(w, r, backend)
}

// ForwardPruned forwards the request to a pruned backend
func (f *Forwarder) ForwardPruned(w http.ResponseWriter, r *http.Request) {
	backend := f.lb.GetPrunedBackend()
	if backend == nil {
		http.Error(w, "No healthy pruned backends available", http.StatusServiceUnavailable)
		return
	}
	r.URL.Path = "/"
	f.forward(w, r, backend)
}

// forward contains the actual reverse proxy logic
func (f *Forwarder) forward(w http.ResponseWriter, r *http.Request, b *Backend) {
	targetURL := b.URL
	target, err := url.Parse(targetURL)
	if err != nil {
		log.Error().Err(err).Str("url", targetURL).Msg("Failed to parse target URL")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Prometheus metric
	start := time.Now()
	defer func() {
		metrics.ObserveRequestDuration("proxy", b.URL, time.Since(start).Seconds())
	}()

	proxy := httputil.NewSingleHostReverseProxy(target)

	// Customize the Director to set the Host header correctly
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = target.Host
	}

	// Error handling
	proxy.ErrorHandler = func(w http.ResponseWriter, req *http.Request, err error) {
		f.lb.IncErrorRequest(b)

		log.Error().Err(err).Str("target", targetURL).Msg("Proxy error")
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	// Modify response to track success
	rw := &statusResponseWriter{ResponseWriter: w, status: 200}
	proxy.ServeHTTP(rw, r)

	// Record the request status (count as success from LB connection perspective)
	f.lb.IncSuccessfulRequest(b, rw.status, time.Since(start))
}

type statusResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusResponseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
