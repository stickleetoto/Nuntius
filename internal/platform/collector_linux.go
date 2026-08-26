//go:build linux

package platform

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"nuntius/internal/core/model"
	"nuntius/internal/core/port"
)

type linuxCollector struct{}

func newNativeCollector() port.Collector { return linuxCollector{} }

func (linuxCollector) Collect(ctx context.Context) (model.NetworkState, error) {
	host, err := os.Hostname()
	if err != nil {
		return model.NetworkState{}, err
	}
	ifs, err := collectInterfaces()
	if err != nil {
		return model.NetworkState{}, err
	}

	state := model.NetworkState{CapturedAt: time.Now().UTC(), Hostname: host, OS: runtime.GOOS, Arch: runtime.GOARCH, Interfaces: ifs}
	if routes, err := linuxRoutes(ctx); err == nil {
		state.Routes = normalizeRoutes(routes)
	} else {
		state.Warnings = append(state.Warnings, "routes: "+err.Error())
	}
	if dns, err := linuxDNS(); err == nil {
		state.DNS = dns
	} else {
		state.Warnings = append(state.Warnings, "dns: "+err.Error())
	}
	if listeners, conns, err := linuxSockets(ctx); err == nil {
		state.Listeners, state.Connections = listeners, conns
	} else {
		state.Warnings = append(state.Warnings, "connections: "+err.Error())
	}
	return state, nil
}

func linuxRoutes(ctx context.Context) ([]model.Route, error) {
	out, err := exec.CommandContext(ctx, "ip", "-o", "route", "show").Output()
	if err != nil {
		return nil, err
	}
	var routes []model.Route
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		r := model.Route{Raw: line}
		if len(fields) > 0 {
			r.Destination = fields[0]
			if r.Destination == "default" {
				r.Destination = "0.0.0.0/0"
			}
		}
		for i := 1; i < len(fields); i++ {
			switch fields[i] {
			case "via":
				if i+1 < len(fields) {
					r.Gateway = fields[i+1]
					i++
				}
			case "dev":
				if i+1 < len(fields) {
					r.Interface = fields[i+1]
					i++
				}
			case "metric":
				if i+1 < len(fields) {
					r.Metric, _ = strconv.Atoi(fields[i+1])
					i++
				}
			}
		}
		routes = append(routes, r)
	}
	return routes, nil
}

func linuxDNS() (model.DNSConfig, error) {
	f, err := os.Open("/etc/resolv.conf")
	if err != nil {
		return model.DNSConfig{}, err
	}
	defer f.Close()
	var servers, search []string
	s := bufio.NewScanner(f)
	for s.Scan() {
		fields := strings.Fields(strings.TrimSpace(s.Text()))
		if len(fields) < 2 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		switch fields[0] {
		case "nameserver":
			servers = append(servers, fields[1])
		case "search", "domain":
			search = append(search, fields[1:]...)
		}
	}
	if err := s.Err(); err != nil {
		return model.DNSConfig{}, err
	}
	return model.DNSConfig{Servers: uniqueStrings(servers), Search: uniqueStrings(search), Source: "/etc/resolv.conf"}, nil
}

func linuxSockets(ctx context.Context) ([]model.Listener, []model.Connection, error) {
	out, err := exec.CommandContext(ctx, "ss", "-H", "-tunap").CombinedOutput()
	if err != nil {
		return nil, nil, fmt.Errorf("ss: %w", err)
	}
	var listeners []model.Listener
	var conns []model.Connection
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		proto := strings.ToLower(fields[0])
		state := fields[1]
		local, remote := fields[4], ""
		if len(fields) > 5 {
			remote = fields[5]
		}
		pid, process := parseSSProcess(line)
		if state == "LISTEN" || (proto == "udp" && remote == "0.0.0.0:*") {
			listeners = append(listeners, model.Listener{Protocol: proto, Local: local, PID: pid, Process: process})
		} else {
			conns = append(conns, model.Connection{Protocol: proto, Local: local, Remote: remote, State: state, PID: pid, Process: process})
		}
	}
	return listeners, conns, nil
}

func parseSSProcess(line string) (int, string) {
	idx := strings.Index(line, "users:((\"")
	if idx < 0 {
		return 0, ""
	}
	rest := line[idx+len("users:((\""):]
	end := strings.Index(rest, "\"")
	if end < 0 {
		return 0, ""
	}
	name := rest[:end]
	pid := 0
	if p := strings.Index(rest, "pid="); p >= 0 {
		pstr := rest[p+4:]
		if comma := strings.IndexByte(pstr, ','); comma >= 0 {
			pstr = pstr[:comma]
		}
		pid, _ = strconv.Atoi(pstr)
	}
	return pid, name
}

var _ = net.IP{}
