package model

import "time"

type CheckStatus string

const (
	CheckPass    CheckStatus = "pass"
	CheckFail    CheckStatus = "fail"
	CheckSkipped CheckStatus = "skipped"
)

type CheckResult struct {
	Name       string         `json:"name"`
	Layer      string         `json:"layer"`
	Status     CheckStatus    `json:"status"`
	DurationMS int64          `json:"duration_ms"`
	Message    string         `json:"message,omitempty"`
	Details    map[string]any `json:"details,omitempty"`
}

type Diagnosis struct {
	Target      string           `json:"target"`
	Host        string           `json:"host"`
	Port        int              `json:"port"`
	Scheme      string           `json:"scheme"`
	StartedAt   time.Time        `json:"started_at"`
	DurationMS  int64            `json:"duration_ms"`
	Overall     string           `json:"overall"`
	Resolved    []string         `json:"resolved,omitempty"`
	Checks      []CheckResult    `json:"checks"`
	Performance map[string]int64 `json:"performance_ms,omitempty"`
}
