package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"nuntius/internal/core/model"
	"nuntius/internal/core/service"
)

type testCollector struct{}

func (testCollector) Collect(context.Context) (model.NetworkState, error) {
	return model.NetworkState{Hostname: "test", OS: "linux", Arch: "amd64"}, nil
}

func TestInitializeAndListTools(t *testing.T) {
	server := New(Dependencies{Info: service.InfoService{Collector: testCollector{}}})
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
	}, "\n") + "\n"
	var out bytes.Buffer
	if err := server.Serve(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 responses, got %q", out.String())
	}
	var initResp map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &initResp); err != nil {
		t.Fatal(err)
	}
	result := initResp["result"].(map[string]any)
	if result["protocolVersion"] != protocolVersion {
		t.Fatalf("unexpected protocol version: %#v", result)
	}
	var toolsResp map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &toolsResp); err != nil {
		t.Fatal(err)
	}
	tools := toolsResp["result"].(map[string]any)["tools"].([]any)
	if len(tools) < 7 {
		t.Fatalf("expected tools, got %#v", tools)
	}
}
