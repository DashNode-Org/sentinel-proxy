package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DashNode-Org/sentinel-proxy/config"
	"github.com/stretchr/testify/assert"
)

func TestForwarder_Forward(t *testing.T) {
	// Setup a mock backend server
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("backend response"))
	}))
	defer backendServer.Close()

	// Setup Config and LoadBalancer
	cfg := &config.Config{
		SentinelBackends: []string{backendServer.URL},
	}
	lb := NewLoadBalancer(cfg)

	// Ensure backend is marked healthy
	lb.UpdateBackendHealth(backendServer.URL, true, 100, 10*time.Millisecond)

	// Setup Forwarder
	f := NewRequestForwarder(cfg, lb)

	// Create a request
	req := httptest.NewRequest("POST", "/", nil)
	w := httptest.NewRecorder()

	// Perform Forward
	f.Forward(w, req)

	// Assertions
	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Check Backend Stats
	backends := lb.GetBackends()
	assert.Equal(t, int64(1), backends[0].RequestStats.TotalRequests)
}

func TestForwarder_ForwardArchiver(t *testing.T) {
	// Setup mock backends
	archiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("archiver"))
	}))
	defer archiver.Close()

	pruned := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("pruned"))
	}))
	defer pruned.Close()

	cfg := &config.Config{
		SentinelBackends: []string{archiver.URL, pruned.URL},
	}
	lb := NewLoadBalancer(cfg)

	// Set node types
	// Set node types
	lb.UpdateBackendStateByUrl(archiver.URL, func(b *Backend) {
		b.NodeType = "archiver"
		b.Healthy = true
	})
	lb.UpdateBackendStateByUrl(pruned.URL, func(b *Backend) {
		b.NodeType = "pruned"
		b.Healthy = true
	})

	f := NewRequestForwarder(cfg, lb)

	// Test ForwardArchiver
	req := httptest.NewRequest("POST", "/archiver", nil)
	w := httptest.NewRecorder()
	f.ForwardArchiver(w, req)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	assert.Equal(t, "archiver", w.Body.String())
}

func TestForwarder_ForwardPruned(t *testing.T) {
	// Setup mock backends
	archiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("archiver"))
	}))
	defer archiver.Close()

	pruned := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("pruned"))
	}))
	defer pruned.Close()

	cfg := &config.Config{
		SentinelBackends: []string{archiver.URL, pruned.URL},
	}
	lb := NewLoadBalancer(cfg)

	// Set node types
	// Set node types
	lb.UpdateBackendStateByUrl(archiver.URL, func(b *Backend) {
		b.NodeType = "archiver"
		b.Healthy = true
	})
	lb.UpdateBackendStateByUrl(pruned.URL, func(b *Backend) {
		b.NodeType = "pruned"
		b.Healthy = true
	})

	f := NewRequestForwarder(cfg, lb)

	// Test ForwardPruned
	req := httptest.NewRequest("POST", "/pruned", nil)
	w := httptest.NewRecorder()
	f.ForwardPruned(w, req)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	assert.Equal(t, "pruned", w.Body.String())
}

func TestForwarder_NoHealthyBackend(t *testing.T) {
	cfg := &config.Config{
		SentinelBackends: []string{"http://badhost"},
	}
	lb := NewLoadBalancer(cfg)
	// Mark unhealthy
	lb.UpdateBackendHealth("http://badhost", false, 0, 0)

	f := NewRequestForwarder(cfg, lb)

	req := httptest.NewRequest("POST", "/", nil)
	w := httptest.NewRecorder()

	f.Forward(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Result().StatusCode)
}
