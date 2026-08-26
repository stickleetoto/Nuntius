package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"nuntius/internal/core/service"
	"nuntius/pkg/version"
)

const protocolVersion = "2025-06-18"

var supportedProtocolVersions = map[string]struct{}{
	"2025-06-18": {},
	"2025-03-26": {},
	"2024-11-05": {},
}

type Dependencies struct {
	Info     service.InfoService
	Snapshot service.SnapshotService
	Diff     service.DiffService
	Doctor   service.DoctorService
	Watch    service.WatchService
	Path     service.PathService
	Timeout  time.Duration
}

type Server struct{ deps Dependencies }

func New(deps Dependencies) *Server {
	if deps.Timeout <= 0 {
		deps.Timeout = 15 * time.Second
	}
	return &Server{deps: deps}
}

func (s *Server) RunStdio(ctx context.Context) error { return s.Serve(ctx, os.Stdin, os.Stdout) }

func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	enc := json.NewEncoder(out)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			if err := enc.Encode(errorResponse(nil, -32700, "parse error")); err != nil {
				return err
			}
			continue
		}
		if req.JSONRPC != "2.0" || req.Method == "" {
			if req.ID != nil {
				if err := enc.Encode(errorResponse(req.ID, -32600, "invalid request")); err != nil {
					return err
				}
			}
			continue
		}
		// Notifications never receive a response.
		if req.ID == nil {
			continue
		}
		response := s.handle(ctx, req)
		if err := enc.Encode(response); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      any            `json:"id,omitempty"`
	Result  any            `json:"result,omitempty"`
	Error   *responseError `json:"error,omitempty"`
}

type responseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (s *Server) handle(ctx context.Context, req request) response {
	switch req.Method {
	case "initialize":
		negotiated := protocolVersion
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if json.Unmarshal(req.Params, &params) == nil {
			if _, ok := supportedProtocolVersions[params.ProtocolVersion]; ok {
				negotiated = params.ProtocolVersion
			}
		}
		return success(req.ID, map[string]any{
			"protocolVersion": negotiated,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "nuntius", "version": version.Version},
			"instructions":    "Nuntius provides read-only network inspection and diagnosis tools. Snapshot tools only write local Nuntius state files.",
		})
	case "ping":
		return success(req.ID, map[string]any{})
	case "tools/list":
		return success(req.ID, map[string]any{"tools": toolDefinitions()})
	case "tools/call":
		result, err := s.callTool(ctx, req.Params)
		if err != nil {
			return errorResponse(req.ID, -32602, err.Error())
		}
		return success(req.ID, result)
	default:
		return errorResponse(req.ID, -32601, "method not found")
	}
}

func (s *Server) callTool(parent context.Context, raw json.RawMessage) (map[string]any, error) {
	var call struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &call); err != nil {
		return nil, errors.New("invalid tools/call params")
	}
	if call.Name == "" {
		return nil, errors.New("tool name is required")
	}
	ctx, cancel := context.WithTimeout(parent, s.deps.Timeout)
	defer cancel()

	var value any
	var err error
	switch call.Name {
	case "nuntius_info":
		value, err = s.deps.Info.Get(ctx)
	case "nuntius_dns", "nuntius_routes", "nuntius_ports", "nuntius_connections":
		state, infoErr := s.deps.Info.Get(ctx)
		if infoErr != nil {
			err = infoErr
			break
		}
		switch call.Name {
		case "nuntius_dns":
			value = state.DNS
		case "nuntius_routes":
			value = state.Routes
		case "nuntius_ports":
			value = state.Listeners
		case "nuntius_connections":
			value = state.Connections
		}
	case "nuntius_snapshot":
		name, argErr := stringArg(call.Arguments, "name")
		if argErr != nil {
			return nil, argErr
		}
		value, err = s.deps.Snapshot.Create(ctx, name)
	case "nuntius_list_snapshots":
		value, err = s.deps.Snapshot.List(ctx)
	case "nuntius_show_snapshot":
		name, argErr := stringArg(call.Arguments, "name")
		if argErr != nil {
			return nil, argErr
		}
		value, err = s.deps.Snapshot.Load(ctx, name)
	case "nuntius_diff":
		from, argErr := stringArg(call.Arguments, "from")
		if argErr != nil {
			return nil, argErr
		}
		to, argErr := stringArg(call.Arguments, "to")
		if argErr != nil {
			return nil, argErr
		}
		value, err = s.deps.Diff.Compare(ctx, from, to)
	case "nuntius_doctor":
		target, argErr := stringArg(call.Arguments, "target")
		if argErr != nil {
			return nil, argErr
		}
		value, err = s.deps.Doctor.Diagnose(ctx, target)
	case "nuntius_ping":
		target, argErr := stringArg(call.Arguments, "target")
		if argErr != nil {
			return nil, argErr
		}
		count, argErr := optionalIntArg(call.Arguments, "count", service.DefaultPingCount)
		if argErr != nil {
			return nil, argErr
		}
		if count < 1 || count > 100 {
			return nil, errors.New("argument \"count\" must be between 1 and 100")
		}
		timeoutMS, argErr := optionalIntArg(call.Arguments, "probe_timeout_ms", int(service.DefaultProbeTimeout/time.Millisecond))
		if argErr != nil {
			return nil, argErr
		}
		if timeoutMS < 100 || timeoutMS > 30000 {
			return nil, errors.New("argument \"probe_timeout_ms\" must be between 100 and 30000")
		}
		value, err = s.deps.Path.Ping(ctx, target, count, time.Duration(timeoutMS)*time.Millisecond)
	case "nuntius_trace":
		target, argErr := stringArg(call.Arguments, "target")
		if argErr != nil {
			return nil, argErr
		}
		maxHops, argErr := optionalIntArg(call.Arguments, "max_hops", service.DefaultMaxHops)
		if argErr != nil {
			return nil, argErr
		}
		if maxHops < 1 || maxHops > 64 {
			return nil, errors.New("argument \"max_hops\" must be between 1 and 64")
		}
		timeoutMS, argErr := optionalIntArg(call.Arguments, "probe_timeout_ms", int(service.DefaultProbeTimeout/time.Millisecond))
		if argErr != nil {
			return nil, argErr
		}
		if timeoutMS < 100 || timeoutMS > 30000 {
			return nil, errors.New("argument \"probe_timeout_ms\" must be between 100 and 30000")
		}
		value, err = s.deps.Path.Trace(ctx, target, maxHops, time.Duration(timeoutMS)*time.Millisecond)
	case "nuntius_path":
		target, argErr := stringArg(call.Arguments, "target")
		if argErr != nil {
			return nil, argErr
		}
		count, argErr := optionalIntArg(call.Arguments, "count", service.DefaultPingCount)
		if argErr != nil {
			return nil, argErr
		}
		if count < 1 || count > 100 {
			return nil, errors.New("argument \"count\" must be between 1 and 100")
		}
		maxHops, argErr := optionalIntArg(call.Arguments, "max_hops", service.DefaultMaxHops)
		if argErr != nil {
			return nil, argErr
		}
		if maxHops < 1 || maxHops > 64 {
			return nil, errors.New("argument \"max_hops\" must be between 1 and 64")
		}
		timeoutMS, argErr := optionalIntArg(call.Arguments, "probe_timeout_ms", int(service.DefaultProbeTimeout/time.Millisecond))
		if argErr != nil {
			return nil, argErr
		}
		if timeoutMS < 100 || timeoutMS > 30000 {
			return nil, errors.New("argument \"probe_timeout_ms\" must be between 100 and 30000")
		}
		value, err = s.deps.Path.Inspect(ctx, target, count, maxHops, time.Duration(timeoutMS)*time.Millisecond)
	case "nuntius_watch_once":
		intervalMS, argErr := optionalIntArg(call.Arguments, "interval_ms", int(service.DefaultWatchInterval/time.Millisecond))
		if argErr != nil {
			return nil, argErr
		}
		if intervalMS < int(service.MinWatchInterval/time.Millisecond) || intervalMS > 10000 {
			return nil, fmt.Errorf("argument %q must be between %d and 10000", "interval_ms", int(service.MinWatchInterval/time.Millisecond))
		}
		categories, argErr := optionalStringSliceArg(call.Arguments, "categories")
		if argErr != nil {
			return nil, argErr
		}
		value, err = s.deps.Watch.ObserveOnce(ctx, time.Duration(intervalMS)*time.Millisecond, categories)
	default:
		return nil, fmt.Errorf("unknown tool %q", call.Name)
	}
	if err != nil {
		return map[string]any{
			"content": []map[string]any{{"type": "text", "text": err.Error()}},
			"isError": true,
		}, nil
	}
	data, marshalErr := json.MarshalIndent(value, "", "  ")
	if marshalErr != nil {
		return nil, marshalErr
	}
	return map[string]any{
		"content":           []map[string]any{{"type": "text", "text": string(data)}},
		"structuredContent": value,
		"isError":           false,
	}, nil
}

func toolDefinitions() []map[string]any {
	empty := map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
	nameArg := objectSchema(map[string]any{"name": map[string]any{"type": "string", "description": "Snapshot name"}}, []string{"name"})
	return []map[string]any{
		{"name": "nuntius_info", "description": "Inspect the current local network state.", "inputSchema": empty},
		{"name": "nuntius_dns", "description": "Inspect the currently configured DNS resolvers and search domains.", "inputSchema": empty},
		{"name": "nuntius_routes", "description": "Inspect the normalized local routing table.", "inputSchema": empty},
		{"name": "nuntius_ports", "description": "List locally listening TCP/UDP endpoints with process metadata when available.", "inputSchema": empty},
		{"name": "nuntius_connections", "description": "List active network connections with process metadata when available.", "inputSchema": empty},
		{"name": "nuntius_snapshot", "description": "Capture the current network state as a named local snapshot.", "inputSchema": nameArg},
		{"name": "nuntius_list_snapshots", "description": "List locally stored Nuntius network snapshots.", "inputSchema": empty},
		{"name": "nuntius_show_snapshot", "description": "Read a named Nuntius network snapshot.", "inputSchema": nameArg},
		{"name": "nuntius_diff", "description": "Compare two stored network snapshots.", "inputSchema": objectSchema(map[string]any{
			"from": map[string]any{"type": "string", "description": "Earlier snapshot name"},
			"to":   map[string]any{"type": "string", "description": "Later snapshot name"},
		}, []string{"from", "to"})},
		{"name": "nuntius_doctor", "description": "Run layered interface, route, DNS, TCP, TLS, and HTTP diagnosis for a target.", "inputSchema": objectSchema(map[string]any{
			"target": map[string]any{"type": "string", "description": "Host, host:port, or http(s) URL"},
		}, []string{"target"})},
		{"name": "nuntius_ping", "description": "Measure ICMP reachability, packet loss, RTT, and jitter for a host.", "inputSchema": objectSchema(map[string]any{
			"target":           map[string]any{"type": "string", "description": "Host or IP address"},
			"count":            map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "description": "Echo probes (default 4)"},
			"probe_timeout_ms": map[string]any{"type": "integer", "minimum": 100, "maximum": 30000, "description": "Timeout for each probe in milliseconds (default 2000)"},
		}, []string{"target"})},
		{"name": "nuntius_trace", "description": "Trace the network hop path to a host using the platform traceroute facility.", "inputSchema": objectSchema(map[string]any{
			"target":           map[string]any{"type": "string", "description": "Host or IP address"},
			"max_hops":         map[string]any{"type": "integer", "minimum": 1, "maximum": 64, "description": "Maximum hop count (default 20)"},
			"probe_timeout_ms": map[string]any{"type": "integer", "minimum": 100, "maximum": 30000, "description": "Per-hop timeout in milliseconds (default 2000)"},
		}, []string{"target"})},
		{"name": "nuntius_path", "description": "Combine packet-loss/jitter measurement and hop tracing into one path-quality report.", "inputSchema": objectSchema(map[string]any{
			"target":           map[string]any{"type": "string", "description": "Host or IP address"},
			"count":            map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "description": "Echo probes (default 4)"},
			"max_hops":         map[string]any{"type": "integer", "minimum": 1, "maximum": 64, "description": "Maximum hop count (default 20)"},
			"probe_timeout_ms": map[string]any{"type": "integer", "minimum": 100, "maximum": 30000, "description": "Probe timeout in milliseconds (default 2000)"},
		}, []string{"target"})},
		{"name": "nuntius_watch_once", "description": "Capture network state twice and return changes observed across a short interval.", "inputSchema": objectSchema(map[string]any{
			"interval_ms": map[string]any{"type": "integer", "minimum": 500, "maximum": 10000, "description": "Observation interval in milliseconds (default 2000)"},
			"categories":  map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": []string{"host", "interface", "dns", "route", "listener", "connection"}}, "description": "Optional change categories; default excludes active connections"},
		}, nil)},
	}
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	schema := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func stringArg(args map[string]any, name string) (string, error) {
	value, ok := args[name].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("argument %q must be a non-empty string", name)
	}
	return strings.TrimSpace(value), nil
}

func optionalIntArg(args map[string]any, name string, defaultValue int) (int, error) {
	value, ok := args[name]
	if !ok || value == nil {
		return defaultValue, nil
	}
	switch n := value.(type) {
	case float64:
		if n != float64(int(n)) {
			return 0, fmt.Errorf("argument %q must be an integer", name)
		}
		return int(n), nil
	case int:
		return n, nil
	default:
		return 0, fmt.Errorf("argument %q must be an integer", name)
	}
}

func optionalStringSliceArg(args map[string]any, name string) ([]string, error) {
	value, ok := args[name]
	if !ok || value == nil {
		return nil, nil
	}
	raw, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("argument %q must be an array of strings", name)
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		text, ok := item.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("argument %q must contain only non-empty strings", name)
		}
		out = append(out, strings.TrimSpace(text))
	}
	return out, nil
}

func success(id any, result any) response {
	return response{JSONRPC: "2.0", ID: id, Result: result}
}

func errorResponse(id any, code int, message string) response {
	return response{JSONRPC: "2.0", ID: id, Error: &responseError{Code: code, Message: message}}
}
