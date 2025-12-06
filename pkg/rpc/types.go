package rpc

type GetValidatorsStatsResponse struct {
	LastProcessedSlot string                    `json:"lastProcessedSlot"`
	Stats             map[string]ValidatorStats `json:"stats"`
}

type ValidatorStats struct {
	History []ValidatorHistoryItem `json:"history"`
}

type ValidatorHistoryItem struct {
	Slot   string `json:"slot"`
	Status string `json:"status"`
}
