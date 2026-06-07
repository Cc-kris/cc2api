package service

type DashboardRevenueOverview struct {
	TotalCreditAmount string `json:"total_credit_amount"`
	UsedAmount        string `json:"used_amount"`
	UnusedAmount      string `json:"unused_amount"`
	NonAdminUserCount int64  `json:"non_admin_user_count"`
	CreditedUserCount int64  `json:"credited_user_count"`
	IsEstimated       bool   `json:"is_estimated"`
	UpdatedAt         string `json:"updated_at"`
}

type DashboardRepurchaseBucket struct {
	Bucket    string  `json:"bucket"`
	Label     string  `json:"label"`
	UserCount int64   `json:"user_count"`
	Ratio     float64 `json:"ratio"`
}

type DashboardRepurchaseDistribution struct {
	Buckets   []DashboardRepurchaseBucket `json:"buckets"`
	UpdatedAt string                      `json:"updated_at"`
}
