package model

type Change struct {
	Kind   string `json:"kind"`
	Key    string `json:"key"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}

type DiffResult struct {
	From    string   `json:"from"`
	To      string   `json:"to"`
	Changes []Change `json:"changes"`
}
