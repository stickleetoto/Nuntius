//go:build windows

package platform

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"nuntius/internal/core/model"
	"nuntius/internal/core/port"
)

type windowsCollector struct{}

func newNativeCollector() port.Collector { return windowsCollector{} }

func (windowsCollector) Collect(ctx context.Context) (model.NetworkState, error) {
	host, err := os.Hostname()
	if err != nil {
		return model.NetworkState{}, err
	}
	ifs, err := collectInterfaces()
	if err != nil {
		return model.NetworkState{}, err
	}
	state := model.NetworkState{CapturedAt: time.Now().UTC(), Hostname: host, OS: runtime.GOOS, Arch: runtime.GOARCH, Interfaces: ifs}
	if routes, err := windowsRoutes(ctx); err == nil {
		state.Routes = normalizeRoutes(routes)
	} else {
		state.Warnings = append(state.Warnings, "routes: "+err.Error())
	}
	if dns, err := windowsDNS(ctx); err == nil {
		state.DNS = dns
	} else {
		state.Warnings = append(state.Warnings, "dns: "+err.Error())
	}
	if listeners, conns, err := windowsSockets(ctx); err == nil {
		state.Listeners, state.Connections = listeners, conns
	} else {
		state.Warnings = append(state.Warnings, "connections: "+err.Error())
	}
	return state, nil
}

func windowsRoutes(ctx context.Context) ([]model.Route, error) {
	if routes, err := windowsRoutesPowerShell(ctx); err == nil && len(routes) > 0 {
		return routes, nil
	}
	return windowsRoutesLegacy(ctx)
}

func windowsRoutesPowerShell(ctx context.Context) ([]model.Route, error) {
	command := `[Console]::OutputEncoding=[System.Text.Encoding]::UTF8; Get-NetRoute -AddressFamily IPv4 | Select-Object DestinationPrefix,NextHop,InterfaceAlias,RouteMetric | ConvertTo-Csv -NoTypeInformation`
	out, err := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", command).Output()
	if err != nil {
		return nil, err
	}
	r := csv.NewReader(bytes.NewReader(out))
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return nil, err
	}
	columns := map[string]int{}
	for i, name := range header {
		columns[strings.TrimSpace(name)] = i
	}
	required := []string{"DestinationPrefix", "NextHop", "InterfaceAlias", "RouteMetric"}
	for _, name := range required {
		if _, ok := columns[name]; !ok {
			return nil, fmt.Errorf("Get-NetRoute output missing %s", name)
		}
	}
	var routes []model.Route
	for {
		record, err := r.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		metric, _ := strconv.Atoi(strings.TrimSpace(record[columns["RouteMetric"]]))
		routes = append(routes, model.Route{
			Destination: strings.TrimSpace(record[columns["DestinationPrefix"]]),
			Gateway:     strings.TrimSpace(record[columns["NextHop"]]),
			Interface:   strings.TrimSpace(record[columns["InterfaceAlias"]]),
			Metric:      metric,
		})
	}
	return routes, nil
}

func windowsRoutesLegacy(ctx context.Context) ([]model.Route, error) {
	out, err := exec.CommandContext(ctx, "route", "print", "-4").CombinedOutput()
	if err != nil {
		return nil, err
	}
	var routes []model.Route
	s := bufio.NewScanner(strings.NewReader(string(out)))
	inActive := false
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if strings.Contains(line, "Active Routes:") || strings.Contains(line, "활성 경로:") {
			inActive = true
			continue
		}
		if !inActive {
			continue
		}
		if strings.HasPrefix(line, "Persistent Routes:") || strings.HasPrefix(line, "영구 경로:") {
			break
		}
		fields := strings.Fields(line)
		if len(fields) != 5 {
			continue
		}
		metric, err := strconv.Atoi(fields[4])
		if err != nil {
			continue
		}
		routes = append(routes, model.Route{Destination: fields[0] + "/" + maskToPrefix(fields[1]), Gateway: fields[2], Interface: fields[3], Metric: metric, Raw: line})
	}
	return routes, s.Err()
}

func maskToPrefix(mask string) string {
	parts := strings.Split(mask, ".")
	bits := 0
	for _, p := range parts {
		n, _ := strconv.Atoi(p)
		for n > 0 {
			bits += n & 1
			n >>= 1
		}
	}
	return strconv.Itoa(bits)
}

func windowsDNS(ctx context.Context) (model.DNSConfig, error) {
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", `Get-DnsClientServerAddress -AddressFamily IPv4,IPv6 | ForEach-Object { $_.ServerAddresses }`)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return model.DNSConfig{}, fmt.Errorf("powershell DNS query failed: %w", err)
	}
	servers := uniqueStrings(strings.Fields(string(out)))
	return model.DNSConfig{Servers: servers, Source: "Get-DnsClientServerAddress"}, nil
}

func windowsSockets(ctx context.Context) ([]model.Listener, []model.Connection, error) {
	processes := windowsProcessNames(ctx)
	out, err := exec.CommandContext(ctx, "netstat", "-ano").CombinedOutput()
	if err != nil {
		return nil, nil, err
	}
	var listeners []model.Listener
	var conns []model.Connection
	for _, raw := range strings.Split(string(out), "\n") {
		fields := strings.Fields(strings.TrimSpace(raw))
		if len(fields) < 4 {
			continue
		}
		proto := strings.ToLower(fields[0])
		switch fields[0] {
		case "TCP":
			if len(fields) < 5 {
				continue
			}
			pid, _ := strconv.Atoi(fields[4])
			if strings.EqualFold(fields[3], "LISTENING") {
				listeners = append(listeners, model.Listener{Protocol: proto, Local: fields[1], PID: pid, Process: processes[pid]})
			} else {
				conns = append(conns, model.Connection{Protocol: proto, Local: fields[1], Remote: fields[2], State: fields[3], PID: pid, Process: processes[pid]})
			}
		case "UDP":
			pid, _ := strconv.Atoi(fields[len(fields)-1])
			listeners = append(listeners, model.Listener{Protocol: proto, Local: fields[1], PID: pid, Process: processes[pid]})
		}
	}
	return listeners, conns, nil
}

func windowsProcessNames(ctx context.Context) map[int]string {
	out := map[int]string{}
	raw, err := exec.CommandContext(ctx, "tasklist", "/FO", "CSV", "/NH").Output()
	if err != nil {
		return out
	}
	r := csv.NewReader(bytes.NewReader(raw))
	r.FieldsPerRecord = -1
	for {
		record, err := r.Read()
		if err != nil {
			break
		}
		if len(record) < 2 {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(record[1]))
		if err != nil {
			continue
		}
		out[pid] = strings.TrimSpace(record[0])
	}
	return out
}
