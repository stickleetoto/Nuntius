package model

import "time"

type PingSample struct {
	Sequence int     `json:"sequence"`
	Success  bool    `json:"success"`
	RTTMS    float64 `json:"rtt_ms,omitempty"`
	Error    string  `json:"error,omitempty"`
}

type PingResult struct {
	Target      string       `json:"target"`
	ResolvedIP  string       `json:"resolved_ip,omitempty"`
	StartedAt   time.Time    `json:"started_at"`
	DurationMS  int64        `json:"duration_ms"`
	Sent        int          `json:"sent"`
	Received    int          `json:"received"`
	LossPercent float64      `json:"loss_percent"`
	MinMS       float64      `json:"min_ms,omitempty"`
	AvgMS       float64      `json:"avg_ms,omitempty"`
	MaxMS       float64      `json:"max_ms,omitempty"`
	JitterMS    float64      `json:"jitter_ms,omitempty"`
	Samples     []PingSample `json:"samples"`
}

type TraceHop struct {
	Hop      int     `json:"hop"`
	Address  string  `json:"address,omitempty"`
	RTTMS    float64 `json:"rtt_ms,omitempty"`
	TimedOut bool    `json:"timed_out,omitempty"`
}

type TraceResult struct {
	Target     string     `json:"target"`
	ResolvedIP string     `json:"resolved_ip,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	DurationMS int64      `json:"duration_ms"`
	MaxHops    int        `json:"max_hops"`
	Reached    bool       `json:"reached"`
	Tool       string     `json:"tool,omitempty"`
	Hops       []TraceHop `json:"hops"`
}

type PathReport struct {
	Target     string       `json:"target"`
	StartedAt  time.Time    `json:"started_at"`
	DurationMS int64        `json:"duration_ms"`
	Overall    string       `json:"overall"`
	Ping       *PingResult  `json:"ping,omitempty"`
	Trace      *TraceResult `json:"trace,omitempty"`
	Warnings   []string     `json:"warnings,omitempty"`
}
