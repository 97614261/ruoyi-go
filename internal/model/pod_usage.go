package model

import "ruoyi-go/pkg/types"

// PODUsageStats deliberately excludes provider receipts and account-wide budgets.
type PODUsageStats struct {
	ChargedMicros   int64 `json:"chargedMicros,string"`
	ReservedMicros  int64 `json:"reservedMicros,string"`
	ProviderCalls   int64 `json:"providerCalls"`
	PendingCalls    int64 `json:"pendingCalls"`
	GeneratedImages int64 `json:"generatedImages"`
}

type PODUsageSummary struct {
	PODUsageStats
	RunCount    int64 `json:"runCount"`
	FailedRuns  int64 `json:"failedRuns"`
	PendingRuns int64 `json:"pendingRuns"`
}

type PODUsageRun struct {
	PODUsageStats
	RunID       int64       `json:"runId,string"`
	ProjectID   int64       `json:"projectId,string"`
	ProjectName string      `json:"projectName"`
	State       PODRunState `json:"state"`
	Attempt     int         `json:"attempt"`
	TestOnly    bool        `json:"testOnly"`
	CreateTime  types.Time  `json:"createTime"`
}

type PODUsageView struct {
	Summary           PODUsageSummary `json:"summary"`
	Rows              []PODUsageRun   `json:"rows"`
	Total             int64           `json:"total"`
	BudgetScope       string          `json:"budgetScope"`
	BudgetLimitMicros int64           `json:"budgetLimitMicros,string"`
}
