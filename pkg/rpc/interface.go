package rpc

import "context"

type RPCClient interface {
	IsReady(ctx context.Context) (bool, error)
	GetBlockNumber(ctx context.Context) (int, error)
	GetValidatorsStats(ctx context.Context) (*GetValidatorsStatsResponse, error)
}
