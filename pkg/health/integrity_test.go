package health

import (
	"context"
	"testing"
	"time"

	"github.com/DashNode-Org/sentinel-proxy/config"
	"github.com/DashNode-Org/sentinel-proxy/pkg/proxy"
	"github.com/DashNode-Org/sentinel-proxy/pkg/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockClient implements rpc.RPCClient
type MockClient struct {
	mock.Mock
}

func (m *MockClient) IsReady(ctx context.Context) (bool, error) {
	args := m.Called(ctx)
	return args.Bool(0), args.Error(1)
}

func (m *MockClient) GetBlockNumber(ctx context.Context) (int, error) {
	args := m.Called(ctx)
	return args.Int(0), args.Error(1)
}

func (m *MockClient) GetValidatorsStats(ctx context.Context) (*rpc.GetValidatorsStatsResponse, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*rpc.GetValidatorsStatsResponse), args.Error(1)
}

func TestIntegrityChecker_PerfectHealth(t *testing.T) {
	// Setup
	cfg := &config.Config{
		SentinelBackends:        []string{"http://node1"},
		SlotsPerEpoch:           2, // Match mock data length
		ExpectedValidators:      2,
		IntegrityScoreThreshold: 95,
		IntegrityCheckEpochs:    10,
	}
	lb := proxy.NewLoadBalancer(cfg)
	ic := NewIntegrityChecker(cfg, lb)

	// Mock RPC Behavior
	mockClient := new(MockClient)
	ic.WithClientFactory(func(url string, timeout time.Duration) rpc.RPCClient {
		return mockClient
	})

	// Construct Mock Response: 1 epoch, perfect integrity
	// Epoch 100, Slot 3200 (if SlotsPerEpoch=32).
	// But valid SlotsPerEpoch=2 implies slots 200, 201?
	// Logic: epoch = slot / slotsPerEpoch.
	// If slot=3200, epoch=1600.
	// We need consistent mock data.

	// Let's use simpler numbers.
	// Epoch 10. Slots 0-1 (if SlotsPerEpoch=2? No 10*2=20).
	// Slots 20, 21. matches Epoch 10.

	mockStats := &rpc.GetValidatorsStatsResponse{
		LastProcessedSlot: "22", // Start of epoch 11
		Stats: map[string]rpc.ValidatorStats{
			"0x1": {
				History: []rpc.ValidatorHistoryItem{
					{Slot: "20", Status: "block-mined"},
					{Slot: "21", Status: "attestation-sent"},
				},
			},
			"0x2": {
				History: []rpc.ValidatorHistoryItem{
					{Slot: "20", Status: "attestation-sent"},
					{Slot: "21", Status: "block-mined"},
				},
			},
		},
	}

	mockClient.On("GetValidatorsStats", mock.Anything).Return(mockStats, nil)

	// Action
	ic.CheckIntegrity()

	// Assert
	// Backend score should be 100
	backends := lb.GetBackends()
	assert.Equal(t, 100, backends[0].IntegrityStats.Score)
	assert.Equal(t, 120.0, backends[0].IntegrityStats.Priority) // Max priority (100 base + 20 health)
}

func TestIntegrityChecker_MissingEpochs(t *testing.T) {
	cfg := &config.Config{
		SentinelBackends:     []string{"http://node1"},
		SlotsPerEpoch:        32,
		IntegrityCheckEpochs: 5,
	}
	lb := proxy.NewLoadBalancer(cfg)
	ic := NewIntegrityChecker(cfg, lb)

	mockClient := new(MockClient)
	ic.WithClientFactory(func(url string, timeout time.Duration) rpc.RPCClient {
		return mockClient
	})

	// Stats with gap: Epoch 100 and 102 (Missing 101).
	// SlotsPerEpoch 32.
	// Epoch 100 -> 3200.
	// Epoch 102 -> 3264.

	mockStats := &rpc.GetValidatorsStatsResponse{
		LastProcessedSlot: "3300",
		Stats: map[string]rpc.ValidatorStats{
			"0x1": {
				History: []rpc.ValidatorHistoryItem{
					{Slot: "3200", Status: "1"},
					{Slot: "3264", Status: "1"},
				},
			},
		},
	}
	mockClient.On("GetValidatorsStats", mock.Anything).Return(mockStats, nil)

	ic.CheckIntegrity()

	backends := lb.GetBackends()
	assert.Equal(t, 1, len(backends[0].IntegrityStats.MissingEpochs))
	// Use EqualValues for loose type check (int vs int64)
	assert.EqualValues(t, 101, backends[0].IntegrityStats.MissingEpochs[0])
	assert.Less(t, backends[0].IntegrityStats.Priority, 120.0) // Should be penalized
}
