# Sentinel Proxy (Go)

Integrity-verifying proxy and load balancer for Sentinel nodes on the Aztec network.

This service acts as a smart gateway for Aztec RPC requests, ensuring high availability, data integrity, and optimal routing for validators and archivers.

## Features

- **Built for Scale**: Leveraging Go's concurrency model for efficient handling of concurrent requests.
- **Integrity Verification**: Continuously validates backend epochs against expected validator counts to detect pruning or data corruption.
- **Smart Load Balancing**:
    - **Health-Aware**: Automatically quarantines unhealthy or lagging nodes.
    - **Priority-Based**: Routes traffic to nodes with the highest integrity scores and lowest latency.
    - **Specialized Routing**: Dedicated handling for `/archiver` (historical data) and `/pruned` (recent data) requests.
- **Observability**:
    - **Metrics**: Native Prometheus integration (`/metrics`) tracking request rates, errors, and backend health.
    - **Dashboard**: Built-in status dashboard (`/dashboard`) visualizing node health and integrity.
- **Production Ready**: Optimized Docker build (distroless) and full `docker-compose` integration.

## Architecture

The service is decoupled into three main components:

1.  **Proxy / Load Balancer** (`pkg/proxy`):
    - Manages backend node state (healthy, block number, latency).
    - Implements weighted round-robin selection logic.
    - Handles request forwarding and error tracking.
2.  **Health & Integrity** (`pkg/health`):
    - **Readiness Checks**: Periodically verifies node reachability and sync status.
    - **Integrity Checks**: Analyzes validator participation history to detect missing epochs or inconsistent states.
3.  **Server** (`pkg/server`):
    - HTTP server layer handling routing, middleware, and API endpoints.

## Getting Started

### Prerequisites
- Go 1.22+
- Make

### Quick Start

```bash
# Build binary
make build

# Run service
make run

# Run tests
make test
```

### Configuration

Configuration is managed via environment variables (or `.env` file). The following variables are supported:

| Variable | Description | Default |
|----------|-------------|---------|
| **Core** | | |
| `PROXY_PORT` | Port to listen on | `8080` |
| `LOG_LEVEL` | Logging verbosity (`debug`, `info`, `warn`, `error`) | `info` |
| `SENTINEL_BACKENDS` | Comma-separated list of Aztec RPC URLs (e.g. `http://node1:8545,http://node2:8545`) | (Required) |
| `REQUEST_TIMEOUT_MS` | Timeout for proxy requests to backends (ms) | `30000` |
| **Health & Integrity** | | |
| `HEALTH_CHECK_INTERVAL_MS` | Interval for basic readiness health checks (ms) | `30000` |
| `INTEGRITY_CHECK_INTERVAL_MS`| Interval for deep integrity validation checks (ms) | `60000` |
| `INTEGRITY_CHECK_EPOCHS` | Number of recent epochs to analyze for integrity | `10` |
| `INTEGRITY_SCORE_THRESHOLD` | Integrity score (0-100) below which a node is marked "bad" | `95` |
| **Network Params** | | |
| `SLOTS_PER_EPOCH` | Aztec network slots per epoch | `32` |
| `EXPECTED_VALIDATORS` | Expected number of validators per epoch | `24` |
| `ARCHIVER_THRESHOLD_EPOCHS` | Min epochs required to be considered an "Archiver" node | `100` |

## API Endpoints

- `POST /` - Proxies JSON-RPC requests to the best available node.
- `POST /archiver` - Proxies to a archiver node.
- `POST /pruned` - Proxies to a pruned node.
- `GET /health` - Service health status and backend stats.
- `GET /ready` - Kubernetes-style readiness probe.
- `GET /metrics` - Prometheus metrics.
- `GET /dashboard` - Interactive HTML dashboard.

## Development

Run live reload mode during development:
```bash
make watch
```