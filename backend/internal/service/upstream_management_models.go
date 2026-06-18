package service

import "time"

type UpstreamPlatformRate struct {
	ID             int64   `json:"id"`
	Platform       string  `json:"platform"`
	BillingMode    string  `json:"billing_mode"`
	RateMultiplier float64 `json:"rate_multiplier"`
	ImageUnitPrice float64 `json:"image_unit_price"`
}

type Upstream struct {
	ID                  int64                  `json:"id"`
	BaseURL             string                 `json:"base_url"`
	NormalizedBaseURL   string                 `json:"normalized_base_url"`
	Name                string                 `json:"name"`
	RateMultiplier      float64                `json:"rate_multiplier"`
	PlatformRates       []UpstreamPlatformRate `json:"platform_rates"`
	InitialBalance      float64                `json:"initial_balance"`
	ConsumedBalance     float64                `json:"consumed_balance"`
	CurrentBalance      float64                `json:"current_balance"`
	AccountCount        int64                  `json:"account_count"`
	BalanceAlertEnabled bool                   `json:"balance_alert_enabled"`
	AlertBalance        *float64               `json:"alert_balance,omitempty"`
	AlertEmailSentAt    *time.Time             `json:"alert_email_sent_at,omitempty"`
	AlertLastBalance    *float64               `json:"alert_last_balance,omitempty"`
	Notes               string                 `json:"notes"`
	CreatedAt           time.Time              `json:"created_at"`
	UpdatedAt           time.Time              `json:"updated_at"`
}

type UpstreamInput struct {
	BaseURL             string                 `json:"base_url"`
	Name                string                 `json:"name"`
	RateMultiplier      float64                `json:"rate_multiplier"`
	PlatformRates       []UpstreamPlatformRate `json:"platform_rates"`
	InitialBalance      float64                `json:"initial_balance"`
	BalanceAlertEnabled bool                   `json:"balance_alert_enabled"`
	AlertBalance        *float64               `json:"alert_balance"`
	Notes               string                 `json:"notes"`
}

type UpstreamStatsSummary struct {
	UpstreamCount         int64   `json:"upstream_count"`
	TotalCurrentBalance   float64 `json:"total_current_balance"`
	TotalInitialBalance   float64 `json:"total_initial_balance"`
	TotalConsumedBalance  float64 `json:"total_consumed_balance"`
	TotalInputTokens      int64   `json:"total_input_tokens"`
	TotalOutputTokens     int64   `json:"total_output_tokens"`
	TotalCacheWriteTokens int64   `json:"total_cache_write_tokens"`
	TotalCacheReadTokens  int64   `json:"total_cache_read_tokens"`
	TotalTokens           int64   `json:"total_tokens"`
}

type UpstreamCostPoint struct {
	Bucket           string  `json:"bucket"`
	UpstreamID       *int64  `json:"upstream_id,omitempty"`
	UpstreamName     string  `json:"upstream_name,omitempty"`
	ConsumedBalance  float64 `json:"consumed_balance"`
	InputTokens      int64   `json:"input_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
	CacheWriteTokens int64   `json:"cache_write_tokens"`
	CacheReadTokens  int64   `json:"cache_read_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
}

type UpstreamStatsResponse struct {
	Summary     UpstreamStatsSummary `json:"summary"`
	CostBars    []UpstreamCostPoint  `json:"cost_bars"`
	TokenTrend  []UpstreamCostPoint  `json:"token_trend"`
	StartDate   string               `json:"start_date"`
	EndDate     string               `json:"end_date"`
	Granularity string               `json:"granularity"`
	UpdatedAt   string               `json:"updated_at"`
}

type FinanceStatsSummary struct {
	UserRechargeTotal      float64 `json:"user_recharge_total"`
	UpstreamRechargeTotal  float64 `json:"upstream_recharge_total"`
	UserConsumedAmount     float64 `json:"user_consumed_amount"`
	UpstreamConsumedAmount float64 `json:"upstream_consumed_amount"`
	ConsumedProfit         float64 `json:"consumed_profit"`
	ConsumedProfitRate     float64 `json:"consumed_profit_rate"`
}

type FinanceTrendPoint struct {
	Bucket                 string  `json:"bucket"`
	Profit                 float64 `json:"profit"`
	UpstreamCost           float64 `json:"upstream_cost"`
	UserRecharge           float64 `json:"user_recharge"`
	UserConsumedAmount     float64 `json:"user_consumed_amount"`
	UpstreamConsumedAmount float64 `json:"upstream_consumed_amount"`
}

type FinanceStatsResponse struct {
	Summary     FinanceStatsSummary `json:"summary"`
	Trend       []FinanceTrendPoint `json:"trend"`
	StartDate   string              `json:"start_date"`
	EndDate     string              `json:"end_date"`
	Granularity string              `json:"granularity"`
	UpdatedAt   string              `json:"updated_at"`
}
