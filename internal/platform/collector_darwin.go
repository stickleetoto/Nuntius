//go:build darwin

package platform

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"nuntius/internal/core/model"
	"nuntius/internal/core/port"
)

type darwinCollector struct{}

func newNativeCollector() port.Collector { return darwinCollector{} }

func (darwinCollector) Collect(ctx context.Context) (model.NetworkState, error) {
	host, err := os.Hostname()
	if err != nil {
		return model.NetworkState{}, err
	}
	ifs, err := collectInterfaces()
	if err != nil {
		return model.NetworkState{}, err
	}
	state := model.NetworkState{CapturedAt: time.Now().UTC(), Hostname: host, OS: runtime.GOOS, Arch: runtime.GOARCH, Interfaces: ifs}
	if routes, err := darwinRoutes(ctx); err == nil {
		state.Routes = normalizeRoutes(routes)
	} else {
		state.Warnings = append(state.Warnings, "routes: "+err.Error())
	}
	if dns, err := darwinDNS(ctx); err == nil {
		state.DNS = dns
	} else {
		state.Warnings = append(state.Warnings, "dns: "+err.Error())
	}
	if listeners, conns, err := darwinSockets(ctx); err == nil {
		state.Listeners, state.Connections = listeners, conns
	} else {
		state.Warnings = append(state.Warnings, "connections: "+err.Error())
	}
	return state, nil
}

func darwinRoutes(ctx context.Context) ([]model.Route, error) {
	out, err := exec.CommandContext(ctx, "netstat", "-rn", "-f", "inet").CombinedOutput()
	if err != nil {
		return nil, err
	}
	var routes []model.Route
	s := bufio.NewScanner(strings.NewReader(string(out)))
	for s.Scan() {
		fields := strings.Fields(strings.TrimSpace(s.Text()))
		if len(fields) < 6 || fields[0] == "Destination" {
			continue
		}
		dst := fields[0]
		if dst == "default" {
			dst = "0.0.0.0/0"
		}
		routes = append(routes, model.Route{Destination: dst, Gateway: fields[1], Interface: fields[len(fields)-1], Raw: strings.Join(fields, " ")})
	}
	return routes, s.Err()
}

func darwinDNS(ctx context.Context) (model.DNSConfig, error) {
	out, err := exec.CommandContext(ctx, "scutil", "--dns").CombinedOutput()
	if err != nil {
		return model.DNSConfig{}, err
	}
	var servers, search []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "nameserver[") {
			if i := strings.Index(line, ":"); i >= 0 {
				servers = append(servers, strings.TrimSpace(line[i+1:]))
			}
		}
		if strings.HasPrefix(line, "search domain[") {
			if i := strings.Index(line, ":"); i >= 0 {
				search = append(search, strings.TrimSpace(line[i+1:]))
			}
		}
	}
	return model.DNSConfig{Servers: uniqueStrings(servers), Search: uniqueStrings(search), Source: "scutil --dns"}, nil
}

func darwinSockets(ctx context.Context) ([]model.Listener, []model.Connection, error) {
	out, err := exec.CommandContext(ctx, "lsof", "-nP", "-iTCP", "-iUDP").CombinedOutput()
	if err != nil {
		return nil, nil, fmt.Errorf("lsof: %w", err)
	}
	var listeners []model.Listener
	var conns []model.Connection
	for i, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if i == 0 {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 9 {
			continue
		}
		pid, _ := strconv.Atoi(fields[1])
		proto := strings.ToLower(fields[7])
		endpoint := strings.Join(fields[8:], " ")
		if strings.Contains(endpoint, "(LISTEN)") {
			listeners = append(listeners, model.Listener{Protocol: proto, Local: strings.TrimSuffix(endpoint, " (LISTEN)"), PID: pid, Process: fields[0]})
		} else if strings.Contains(endpoint, "->") {
			parts := strings.SplitN(endpoint, "->", 2)
			conns = append(conns, model.Connection{Protocol: proto, Local: parts[0], Remote: strings.Fields(parts[1])[0], PID: pid, Process: fields[0]})
		}
	}
	return listeners, conns, nil
}
