package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

type Client struct {
	url    string
	client *http.Client
}

type JSONRPCRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      int           `json:"id"`
}

type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
	ID      int             `json:"id"`
}

type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func NewClient(url string, timeout time.Duration) *Client {
	return &Client{
		url: url,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) Call(ctx context.Context, method string, params ...interface{}) (json.RawMessage, error) {
	if params == nil {
		params = []interface{}{}
	}
	reqBody := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      1,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var rpcResp JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("rpc error: %s (code %d)", rpcResp.Error.Message, rpcResp.Error.Code)
	}

	return rpcResp.Result, nil
}

func (c *Client) GetValidatorsStats(ctx context.Context) (*GetValidatorsStatsResponse, error) {
	res, err := c.Call(ctx, "node_getValidatorsStats")
	if err != nil {
		return nil, err
	}

	var stats GetValidatorsStatsResponse
	if err := json.Unmarshal(res, &stats); err != nil {
		return nil, fmt.Errorf("unmarshal stats: %w", err)
	}

	return &stats, nil
}

func (c *Client) IsReady(ctx context.Context) (bool, error) {
	res, err := c.Call(ctx, "node_isReady")
	if err != nil {
		return false, err
	}
	var val bool
	if err := json.Unmarshal(res, &val); err != nil {
		return false, fmt.Errorf("unmarshal isReady: %w", err)
	}
	return val, nil
}

func (c *Client) GetBlockNumber(ctx context.Context) (int, error) {
	res, err := c.Call(ctx, "node_getBlockNumber")
	if err != nil {
		return 0, err
	}

	// Try integer first
	var i int
	if err := json.Unmarshal(res, &i); err == nil {
		return i, nil
	}

	// Fallback to string (e.g. "123")
	var s string
	if err := json.Unmarshal(res, &s); err == nil {
		val, err := strconv.Atoi(s)
		if err != nil {
			return 0, fmt.Errorf("parse blockNum string: %w", err)
		}
		return val, nil
	}

	return 0, fmt.Errorf("unmarshal blockNum failed")
}
