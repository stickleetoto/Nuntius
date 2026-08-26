package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"nuntius/internal/core/model"
	"nuntius/internal/core/service"
	nuntiusmcp "nuntius/internal/mcp"
	"nuntius/internal/platform"
	"nuntius/internal/storage/jsonstore"
	"nuntius/pkg/version"
)

const defaultCommandTimeout = 60 * time.Second

type App struct {
	out    io.Writer
	err    io.Writer
	info   service.InfoService
	snap   service.SnapshotService
	diff   service.DiffService
	doctor service.DoctorService
	watch  service.WatchService
	path   service.PathService
}

func New(out, errOut io.Writer) (*App, error) {
	root, err := jsonstore.DefaultRoot()
	if err != nil {
		return nil, err
	}
	collector := platform.NewCollector()
	pathProbe := platform.NewPathProbe()
	store := jsonstore.New(root)
	return &App{
		out:    out,
		err:    errOut,
		info:   service.InfoService{Collector: collector},
		snap:   service.SnapshotService{Collector: collector, Storage: store},
		diff:   service.DiffService{Storage: store},
		doctor: service.DoctorService{Collector: collector, Timeout: 4 * time.Second},
		watch:  service.WatchService{Collector: collector, Storage: store},
		path:   service.PathService{Probe: pathProbe},
	}, nil
}

func (a *App) Run(parent context.Context, args []string) error {
	if len(args) == 0 {
		a.printHelp()
		return nil
	}
	parsed, err := parseGlobalArgs(args)
	if err != nil {
		return err
	}
	args = parsed.args
	if len(args) == 0 {
		a.printHelp()
		return nil
	}

	ctx := parent
	if args[0] != "mcp" && args[0] != "watch" {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(parent, parsed.timeout)
		defer cancel()
	}

	switch args[0] {
	case "help", "-h", "--help":
		a.printHelp()
		return nil
	case "version", "--version":
		fmt.Fprintln(a.out, version.Version)
		return nil
	case "info":
		state, err := a.info.Get(ctx)
		if err != nil {
			return err
		}
		return a.printValue(state, parsed.json)
	case "dns":
		state, err := a.info.Get(ctx)
		if err != nil {
			return err
		}
		return a.printDNS(state.DNS, parsed.json)
	case "routes":
		state, err := a.info.Get(ctx)
		if err != nil {
			return err
		}
		return a.printRoutes(state.Routes, parsed.json)
	case "ports":
		state, err := a.info.Get(ctx)
		if err != nil {
			return err
		}
		return a.printListeners(state.Listeners, parsed.json)
	case "connections":
		state, err := a.info.Get(ctx)
		if err != nil {
			return err
		}
		return a.printConnections(state.Connections, parsed.json)
	case "snapshot":
		if len(args) != 2 {
			return errors.New("usage: nuntius snapshot <name>")
		}
		snap, err := a.snap.Create(ctx, args[1])
		if err != nil {
			return err
		}
		if parsed.json {
			return a.printValue(snap, true)
		}
		fmt.Fprintf(a.out, "Snapshot %q saved at %s\n", snap.Name, snap.CreatedAt.Local().Format(time.RFC3339))
		return nil
	case "list":
		items, err := a.snap.List(ctx)
		if err != nil {
			return err
		}
		if parsed.json {
			return a.printValue(items, true)
		}
		if len(items) == 0 {
			fmt.Fprintln(a.out, "No snapshots.")
			return nil
		}
		for _, s := range items {
			fmt.Fprintf(a.out, "%-24s %s  %s/%s\n", s.Name, s.CreatedAt.Local().Format(time.RFC3339), s.State.OS, s.State.Arch)
		}
		return nil
	case "show":
		if len(args) != 2 {
			return errors.New("usage: nuntius show <name>")
		}
		snap, err := a.snap.Load(ctx, args[1])
		if err != nil {
			return err
		}
		return a.printSnapshot(snap, parsed.json)
	case "diff":
		if len(args) != 3 {
			return errors.New("usage: nuntius diff <from> <to>")
		}
		result, err := a.diff.Compare(ctx, args[1], args[2])
		if err != nil {
			return err
		}
		return a.printDiff(result, parsed.json)
	case "doctor":
		if len(args) != 2 {
			return errors.New("usage: nuntius doctor <target>")
		}
		result, err := a.doctor.Diagnose(ctx, args[1])
		if err != nil {
			return err
		}
		return a.printDiagnosis(result, parsed.json)
	case "ping":
		target, opts, err := parsePathArgs(args[1:], false)
		if err != nil {
			return err
		}
		result, err := a.path.Ping(ctx, target, opts.Count, opts.ProbeTimeout)
		if err != nil {
			return err
		}
		return a.printPing(result, parsed.json)
	case "trace":
		target, opts, err := parsePathArgs(args[1:], true)
		if err != nil {
			return err
		}
		result, err := a.path.Trace(ctx, target, opts.MaxHops, opts.ProbeTimeout)
		if err != nil {
			return err
		}
		return a.printTrace(result, parsed.json)
	case "path":
		target, opts, err := parsePathArgs(args[1:], true)
		if err != nil {
			return err
		}
		result, err := a.path.Inspect(ctx, target, opts.Count, opts.MaxHops, opts.ProbeTimeout)
		if err != nil {
			return err
		}
		return a.printPath(result, parsed.json)
	case "watch":
		opts, err := parseWatchArgs(args[1:])
		if err != nil {
			return err
		}
		if !parsed.json {
			fmt.Fprintf(a.out, "Watching network changes every %s (Ctrl+C to stop)\n", opts.Interval)
		}
		return a.watch.Stream(parent, opts, func(batch model.WatchBatch) error {
			return a.printWatchBatch(batch, parsed.json)
		})
	case "mcp":
		if len(args) != 1 {
			return errors.New("usage: nuntius mcp")
		}
		server := nuntiusmcp.New(nuntiusmcp.Dependencies{Info: a.info, Snapshot: a.snap, Diff: a.diff, Doctor: a.doctor, Watch: a.watch, Path: a.path, Timeout: parsed.timeout})
		return server.RunStdio(parent)
	default:
		return fmt.Errorf("unknown command %q (try: nuntius help)", args[0])
	}
}

type globalArgs struct {
	args    []string
	json    bool
	timeout time.Duration
}

type pathArgs struct {
	Count        int
	MaxHops      int
	ProbeTimeout time.Duration
}

func parsePathArgs(args []string, allowTrace bool) (string, pathArgs, error) {
	opts := pathArgs{Count: service.DefaultPingCount, MaxHops: service.DefaultMaxHops, ProbeTimeout: service.DefaultProbeTimeout}
	var target string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--count":
			if i+1 >= len(args) {
				return "", opts, errors.New("--count requires an integer")
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 1 || n > 100 {
				return "", opts, errors.New("--count must be between 1 and 100")
			}
			opts.Count = n
		case strings.HasPrefix(arg, "--count="):
			n, err := strconv.Atoi(strings.TrimPrefix(arg, "--count="))
			if err != nil || n < 1 || n > 100 {
				return "", opts, errors.New("--count must be between 1 and 100")
			}
			opts.Count = n
		case arg == "--max-hops":
			if !allowTrace {
				return "", opts, errors.New("--max-hops is only valid for trace/path")
			}
			if i+1 >= len(args) {
				return "", opts, errors.New("--max-hops requires an integer")
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 1 || n > 64 {
				return "", opts, errors.New("--max-hops must be between 1 and 64")
			}
			opts.MaxHops = n
		case strings.HasPrefix(arg, "--max-hops="):
			if !allowTrace {
				return "", opts, errors.New("--max-hops is only valid for trace/path")
			}
			n, err := strconv.Atoi(strings.TrimPrefix(arg, "--max-hops="))
			if err != nil || n < 1 || n > 64 {
				return "", opts, errors.New("--max-hops must be between 1 and 64")
			}
			opts.MaxHops = n
		case arg == "--probe-timeout":
			if i+1 >= len(args) {
				return "", opts, errors.New("--probe-timeout requires a duration")
			}
			i++
			d, err := time.ParseDuration(args[i])
			if err != nil || d < 100*time.Millisecond || d > 30*time.Second {
				return "", opts, errors.New("--probe-timeout must be between 100ms and 30s")
			}
			opts.ProbeTimeout = d
		case strings.HasPrefix(arg, "--probe-timeout="):
			d, err := time.ParseDuration(strings.TrimPrefix(arg, "--probe-timeout="))
			if err != nil || d < 100*time.Millisecond || d > 30*time.Second {
				return "", opts, errors.New("--probe-timeout must be between 100ms and 30s")
			}
			opts.ProbeTimeout = d
		case strings.HasPrefix(arg, "-"):
			return "", opts, fmt.Errorf("unknown path option %q", arg)
		default:
			if target != "" {
				return "", opts, errors.New("only one target may be specified")
			}
			target = arg
		}
	}
	if strings.TrimSpace(target) == "" {
		return "", opts, errors.New("target is required")
	}
	return target, opts, nil
}

func parseGlobalArgs(args []string) (globalArgs, error) {
	timeout := defaultCommandTimeout
	if raw := strings.TrimSpace(os.Getenv("NUNTIUS_TIMEOUT")); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			return globalArgs{}, errors.New("NUNTIUS_TIMEOUT must be a positive Go duration such as 15s")
		}
		timeout = parsed
	}
	out := globalArgs{timeout: timeout}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json":
			out.json = true
		case arg == "--timeout":
			if i+1 >= len(args) {
				return globalArgs{}, errors.New("--timeout requires a duration, for example --timeout 10s")
			}
			i++
			parsed, err := time.ParseDuration(args[i])
			if err != nil || parsed <= 0 {
				return globalArgs{}, errors.New("--timeout must be a positive Go duration such as 10s")
			}
			out.timeout = parsed
		case strings.HasPrefix(arg, "--timeout="):
			parsed, err := time.ParseDuration(strings.TrimPrefix(arg, "--timeout="))
			if err != nil || parsed <= 0 {
				return globalArgs{}, errors.New("--timeout must be a positive Go duration such as 10s")
			}
			out.timeout = parsed
		default:
			out.args = append(out.args, arg)
		}
	}
	return out, nil
}

func parseWatchArgs(args []string) (service.WatchOptions, error) {
	opts := service.WatchOptions{Interval: service.DefaultWatchInterval}
	var categories []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--interval":
			if i+1 >= len(args) {
				return service.WatchOptions{}, errors.New("--interval requires a duration, for example --interval 2s")
			}
			i++
			d, err := time.ParseDuration(args[i])
			if err != nil || d < service.MinWatchInterval {
				return service.WatchOptions{}, fmt.Errorf("--interval must be at least %s", service.MinWatchInterval)
			}
			opts.Interval = d
		case strings.HasPrefix(arg, "--interval="):
			d, err := time.ParseDuration(strings.TrimPrefix(arg, "--interval="))
			if err != nil || d < service.MinWatchInterval {
				return service.WatchOptions{}, fmt.Errorf("--interval must be at least %s", service.MinWatchInterval)
			}
			opts.Interval = d
		case arg == "--count":
			if i+1 >= len(args) {
				return service.WatchOptions{}, errors.New("--count requires a non-negative integer")
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 0 {
				return service.WatchOptions{}, errors.New("--count requires a non-negative integer")
			}
			opts.Count = n
		case strings.HasPrefix(arg, "--count="):
			n, err := strconv.Atoi(strings.TrimPrefix(arg, "--count="))
			if err != nil || n < 0 {
				return service.WatchOptions{}, errors.New("--count requires a non-negative integer")
			}
			opts.Count = n
		case arg == "--auto-snapshot":
			opts.AutoSnapshot = true
		case arg == "--snapshot-prefix":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return service.WatchOptions{}, errors.New("--snapshot-prefix requires a value")
			}
			i++
			opts.SnapshotPrefix = args[i]
		case strings.HasPrefix(arg, "--snapshot-prefix="):
			opts.SnapshotPrefix = strings.TrimSpace(strings.TrimPrefix(arg, "--snapshot-prefix="))
			if opts.SnapshotPrefix == "" {
				return service.WatchOptions{}, errors.New("--snapshot-prefix requires a value")
			}
		case arg == "--dns":
			categories = append(categories, "dns")
		case arg == "--route":
			categories = append(categories, "route")
		case arg == "--interface":
			categories = append(categories, "interface")
		case arg == "--ports":
			categories = append(categories, "listener")
		case arg == "--connections":
			categories = append(categories, "connection")
		case arg == "--host":
			categories = append(categories, "host")
		case arg == "--all":
			categories = []string{"host", "interface", "dns", "route", "listener", "connection"}
		default:
			return service.WatchOptions{}, fmt.Errorf("unknown watch option %q", arg)
		}
	}
	opts.Categories = categories
	return opts, nil
}

func (a *App) printValue(v any, jsonMode bool) error {
	if jsonMode {
		enc := json.NewEncoder(a.out)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	switch x := v.(type) {
	case model.NetworkState:
		a.printState(x)
		return nil
	}
	return errors.New("human formatter not implemented for this value")
}

func (a *App) printDNS(d model.DNSConfig, jsonMode bool) error {
	if jsonMode {
		return a.printValue(d, true)
	}
	fmt.Fprintln(a.out, "DNS")
	if len(d.Servers) == 0 {
		fmt.Fprintln(a.out, "  (none detected)")
	} else {
		for _, server := range d.Servers {
			fmt.Fprintf(a.out, "  %s\n", server)
		}
	}
	if len(d.Search) > 0 {
		fmt.Fprintf(a.out, "Search: %s\n", strings.Join(d.Search, ", "))
	}
	if d.Source != "" {
		fmt.Fprintf(a.out, "Source: %s\n", d.Source)
	}
	return nil
}

func (a *App) printRoutes(routes []model.Route, jsonMode bool) error {
	if jsonMode {
		return a.printValue(routes, true)
	}
	fmt.Fprintln(a.out, "Routes")
	if len(routes) == 0 {
		fmt.Fprintln(a.out, "  (none detected)")
		return nil
	}
	for _, r := range routes {
		fmt.Fprintf(a.out, "  %-20s via %-18s if %-12s metric %d\n", r.Destination, r.Gateway, r.Interface, r.Metric)
	}
	return nil
}

func (a *App) printListeners(listeners []model.Listener, jsonMode bool) error {
	if jsonMode {
		return a.printValue(listeners, true)
	}
	fmt.Fprintf(a.out, "Listening ports (%d)\n", len(listeners))
	for _, l := range listeners {
		proc := ""
		if l.Process != "" {
			proc = " " + l.Process
		}
		pid := ""
		if l.PID > 0 {
			pid = fmt.Sprintf(" pid=%d", l.PID)
		}
		fmt.Fprintf(a.out, "  %-4s %-24s%s%s\n", strings.ToUpper(l.Protocol), l.Local, pid, proc)
	}
	return nil
}

func (a *App) printConnections(connections []model.Connection, jsonMode bool) error {
	if jsonMode {
		return a.printValue(connections, true)
	}
	fmt.Fprintf(a.out, "Active connections (%d)\n", len(connections))
	for _, c := range connections {
		proc := ""
		if c.Process != "" {
			proc = " " + c.Process
		}
		pid := ""
		if c.PID > 0 {
			pid = fmt.Sprintf(" pid=%d", c.PID)
		}
		state := ""
		if c.State != "" {
			state = " " + c.State
		}
		fmt.Fprintf(a.out, "  %-4s %-24s -> %-24s%s%s%s\n", strings.ToUpper(c.Protocol), c.Local, c.Remote, state, pid, proc)
	}
	return nil
}

func (a *App) printSnapshot(s model.Snapshot, jsonMode bool) error {
	if jsonMode {
		return a.printValue(s, true)
	}
	fmt.Fprintf(a.out, "Snapshot: %s\nCreated : %s\n\n", s.Name, s.CreatedAt.Local().Format(time.RFC3339))
	a.printState(s.State)
	return nil
}

func (a *App) printState(s model.NetworkState) {
	fmt.Fprintf(a.out, "Nuntius %s\n", version.Version)
	fmt.Fprintf(a.out, "Host      %s\nPlatform  %s/%s\nCaptured  %s\n", s.Hostname, s.OS, s.Arch, s.CapturedAt.Local().Format(time.RFC3339))
	fmt.Fprintln(a.out, "\nInterfaces")
	for _, iface := range s.Interfaces {
		fmt.Fprintf(a.out, "  %s  mtu=%d", iface.Name, iface.MTU)
		if iface.MAC != "" {
			fmt.Fprintf(a.out, "  mac=%s", iface.MAC)
		}
		fmt.Fprintln(a.out)
		for _, addr := range iface.Addresses {
			fmt.Fprintf(a.out, "    %-4s %s\n", strings.ToUpper(addr.Family), addr.CIDR)
		}
	}
	fmt.Fprintln(a.out, "\nDNS")
	if len(s.DNS.Servers) == 0 {
		fmt.Fprintln(a.out, "  (none detected)")
	} else {
		for _, d := range s.DNS.Servers {
			fmt.Fprintf(a.out, "  %s\n", d)
		}
	}
	fmt.Fprintln(a.out, "\nRoutes")
	if len(s.Routes) == 0 {
		fmt.Fprintln(a.out, "  (none detected)")
	} else {
		for _, r := range s.Routes {
			fmt.Fprintf(a.out, "  %-20s via %-18s if %-12s metric %d\n", r.Destination, r.Gateway, r.Interface, r.Metric)
		}
	}
	fmt.Fprintf(a.out, "\nListeners   %d\nConnections %d\n", len(s.Listeners), len(s.Connections))
	if len(s.Warnings) > 0 {
		fmt.Fprintln(a.out, "\nWarnings")
		for _, w := range s.Warnings {
			fmt.Fprintf(a.out, "  - %s\n", w)
		}
	}
}

func (a *App) printDiff(d model.DiffResult, jsonMode bool) error {
	if jsonMode {
		return a.printValue(d, true)
	}
	fmt.Fprintf(a.out, "Diff: %s -> %s\n", d.From, d.To)
	if len(d.Changes) == 0 {
		fmt.Fprintln(a.out, "No network changes detected.")
		return nil
	}
	groups := map[string][]model.Change{}
	for _, c := range d.Changes {
		groups[c.Key] = append(groups[c.Key], c)
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(a.out, "\n%s\n", k)
		for _, c := range groups[k] {
			switch c.Kind {
			case "added":
				fmt.Fprintf(a.out, "  + %s\n", c.After)
			case "removed":
				fmt.Fprintf(a.out, "  - %s\n", c.Before)
			case "changed":
				fmt.Fprintf(a.out, "  ~ %s -> %s\n", c.Before, c.After)
			}
		}
	}
	return nil
}

func (a *App) printDiagnosis(d model.Diagnosis, jsonMode bool) error {
	if jsonMode {
		return a.printValue(d, true)
	}
	fmt.Fprintf(a.out, "Nuntius Doctor: %s\n", d.Target)
	fmt.Fprintf(a.out, "Target  %s://%s:%d\nOverall %s\n\n", d.Scheme, d.Host, d.Port, strings.ToUpper(d.Overall))
	for _, c := range d.Checks {
		mark := "-"
		switch c.Status {
		case model.CheckPass:
			mark = "OK"
		case model.CheckFail:
			mark = "FAIL"
		case model.CheckSkipped:
			mark = "SKIP"
		}
		fmt.Fprintf(a.out, "%-5s %-18s %-5dms %s\n", mark, c.Layer, c.DurationMS, c.Message)
		printDiagnosisDetails(a.out, c)
	}
	if len(d.Resolved) > 0 {
		fmt.Fprintf(a.out, "\nResolved: %s\n", strings.Join(d.Resolved, ", "))
	}
	if len(d.Performance) > 0 {
		fmt.Fprintln(a.out, "\nTiming")
		for _, key := range []string{"dns", "tcp", "tls", "http_ttfb", "http_total"} {
			if value, ok := d.Performance[key]; ok {
				fmt.Fprintf(a.out, "  %-12s %dms\n", key, value)
			}
		}
	}
	fmt.Fprintf(a.out, "Duration: %dms\n", d.DurationMS)
	return nil
}

func (a *App) printPing(p model.PingResult, jsonMode bool) error {
	if jsonMode {
		return a.printValue(p, true)
	}
	fmt.Fprintf(a.out, "Nuntius Ping: %s", p.Target)
	if p.ResolvedIP != "" && p.ResolvedIP != p.Target {
		fmt.Fprintf(a.out, " (%s)", p.ResolvedIP)
	}
	fmt.Fprintln(a.out)
	for _, sample := range p.Samples {
		if sample.Success {
			fmt.Fprintf(a.out, "  #%d  %.2fms\n", sample.Sequence, sample.RTTMS)
		} else {
			fmt.Fprintf(a.out, "  #%d  timeout\n", sample.Sequence)
		}
	}
	fmt.Fprintf(a.out, "\nSent %d  Received %d  Loss %.1f%%\n", p.Sent, p.Received, p.LossPercent)
	if p.Received > 0 {
		fmt.Fprintf(a.out, "RTT  min %.2fms  avg %.2fms  max %.2fms  jitter %.2fms\n", p.MinMS, p.AvgMS, p.MaxMS, p.JitterMS)
	}
	return nil
}

func (a *App) printTrace(t model.TraceResult, jsonMode bool) error {
	if jsonMode {
		return a.printValue(t, true)
	}
	fmt.Fprintf(a.out, "Nuntius Trace: %s", t.Target)
	if t.ResolvedIP != "" && t.ResolvedIP != t.Target {
		fmt.Fprintf(a.out, " (%s)", t.ResolvedIP)
	}
	fmt.Fprintf(a.out, "  tool=%s\n", t.Tool)
	if len(t.Hops) == 0 {
		fmt.Fprintln(a.out, "  (no hops returned)")
	}
	for _, hop := range t.Hops {
		if hop.TimedOut || hop.Address == "" {
			fmt.Fprintf(a.out, "  %2d  *\n", hop.Hop)
			continue
		}
		fmt.Fprintf(a.out, "  %2d  %-39s %8.2fms\n", hop.Hop, hop.Address, hop.RTTMS)
	}
	fmt.Fprintf(a.out, "\nReached: %t  Hops: %d  Duration: %dms\n", t.Reached, len(t.Hops), t.DurationMS)
	return nil
}

func (a *App) printPath(p model.PathReport, jsonMode bool) error {
	if jsonMode {
		return a.printValue(p, true)
	}
	fmt.Fprintf(a.out, "Nuntius Path: %s\nOverall: %s\n\n", p.Target, strings.ToUpper(p.Overall))
	if p.Ping != nil {
		fmt.Fprintf(a.out, "Quality  loss %.1f%%  avg %.2fms  jitter %.2fms\n", p.Ping.LossPercent, p.Ping.AvgMS, p.Ping.JitterMS)
	}
	if p.Trace != nil {
		fmt.Fprintf(a.out, "Route    hops %d  reached=%t\n", len(p.Trace.Hops), p.Trace.Reached)
		for _, hop := range p.Trace.Hops {
			if hop.TimedOut || hop.Address == "" {
				fmt.Fprintf(a.out, "  %2d  *\n", hop.Hop)
			} else {
				fmt.Fprintf(a.out, "  %2d  %-39s %8.2fms\n", hop.Hop, hop.Address, hop.RTTMS)
			}
		}
	}
	if len(p.Warnings) > 0 {
		fmt.Fprintln(a.out, "\nWarnings")
		for _, warning := range p.Warnings {
			fmt.Fprintf(a.out, "  - %s\n", warning)
		}
	}
	fmt.Fprintf(a.out, "Duration: %dms\n", p.DurationMS)
	return nil
}

func printDiagnosisDetails(out io.Writer, c model.CheckResult) {
	if len(c.Details) == 0 {
		return
	}
	text := func(key string) string {
		if value, ok := c.Details[key]; ok {
			return fmt.Sprint(value)
		}
		return ""
	}
	switch c.Name {
	case "target_route":
		if dst := text("destination"); dst != "" {
			fmt.Fprintf(out, "      route %s", dst)
			if gateway := text("gateway"); gateway != "" {
				fmt.Fprintf(out, " via %s", gateway)
			}
			if iface := text("interface"); iface != "" {
				fmt.Fprintf(out, " dev %s", iface)
			}
			fmt.Fprintln(out)
		}
	case "tls_handshake":
		if version := text("version"); version != "" {
			fmt.Fprintf(out, "      %s", version)
			if cipher := text("cipher"); cipher != "" {
				fmt.Fprintf(out, "  %s", cipher)
			}
			fmt.Fprintln(out)
		}
		if subject := text("certificate_subject"); subject != "" {
			fmt.Fprintf(out, "      cert %s", subject)
			if expires := text("certificate_not_after"); expires != "" {
				fmt.Fprintf(out, "  expires %s", expires)
			}
			fmt.Fprintln(out)
		}
	case "http_request":
		if finalURL := text("final_url"); finalURL != "" && finalURL != "<nil>" {
			fmt.Fprintf(out, "      final %s\n", finalURL)
		}
		if redirects, ok := c.Details["redirects"].([]string); ok && len(redirects) > 0 {
			fmt.Fprintf(out, "      redirects %d\n", len(redirects))
		}
		if proxy := text("proxy"); proxy != "" {
			fmt.Fprintf(out, "      proxy %s\n", proxy)
		}
	}
}

func (a *App) printWatchBatch(batch model.WatchBatch, jsonMode bool) error {
	if jsonMode {
		// Watch JSON is NDJSON: one change batch per line for stream-friendly consumers.
		return json.NewEncoder(a.out).Encode(batch)
	}
	for _, event := range batch.Events {
		value := friendlyWatchValue(event.Category, event.After)
		prefix := "+"
		switch event.Kind {
		case "removed":
			prefix, value = "-", friendlyWatchValue(event.Category, event.Before)
		case "changed":
			prefix, value = "~", friendlyWatchValue(event.Category, event.Before)+" -> "+friendlyWatchValue(event.Category, event.After)
		}
		fmt.Fprintf(a.out, "%s  %-10s %s %s\n", event.ObservedAt.Local().Format("15:04:05"), strings.ToUpper(event.Category), prefix, value)
	}
	if batch.SnapshotName != "" {
		fmt.Fprintf(a.out, "           SNAPSHOT   saved %s\n", batch.SnapshotName)
	}
	return nil
}

func friendlyWatchValue(category, raw string) string {
	parts := strings.Split(raw, "|")
	switch category {
	case "listener":
		if len(parts) >= 2 {
			out := strings.ToUpper(parts[0]) + " " + parts[1]
			for _, p := range parts[2:] {
				if p != "pid=0" && p != "proc=" {
					out += " " + p
				}
			}
			return out
		}
	case "connection":
		if len(parts) >= 2 {
			out := strings.ToUpper(parts[0]) + " " + parts[1]
			for _, p := range parts[2:] {
				if p != "pid=0" && p != "proc=" {
					out += " " + p
				}
			}
			return out
		}
	case "route":
		if len(parts) > 0 {
			out := parts[0]
			for _, p := range parts[1:] {
				switch {
				case strings.HasPrefix(p, "via=") && p != "via=":
					out += " via " + strings.TrimPrefix(p, "via=")
				case strings.HasPrefix(p, "if=") && p != "if=":
					out += " dev " + strings.TrimPrefix(p, "if=")
				case strings.HasPrefix(p, "metric=") && p != "metric=0":
					out += " " + p
				}
			}
			return out
		}
	}
	return raw
}

func (a *App) printHelp() {
	fmt.Fprintf(a.out, `Nuntius CLI %s
Cross-platform network state inspection, diff, diagnosis, and MCP access.

Usage:
  nuntius info [--json] [--timeout 15s]
  nuntius dns [--json]
  nuntius routes [--json]
  nuntius ports [--json]
  nuntius connections [--json]
  nuntius snapshot <name> [--json]
  nuntius list [--json]
  nuntius show <name> [--json]
  nuntius diff <from> <to> [--json]
  nuntius doctor <host|host:port|url> [--json] [--timeout 15s]
  nuntius ping <host> [--count 4] [--probe-timeout 2s] [--json]
  nuntius trace <host> [--max-hops 20] [--probe-timeout 2s] [--json]
  nuntius path <host> [--count 4] [--max-hops 20] [--probe-timeout 2s] [--json]
  nuntius watch [--interval 2s] [--json] [filters] [--auto-snapshot]
  nuntius mcp
  nuntius version

Environment:
  NUNTIUS_HOME      Override config/snapshot directory.
  NUNTIUS_TIMEOUT   Default command/MCP tool timeout (example: 15s).

Watch:
  Default categories: interface, DNS, route, listener. Active connections are opt-in.
  Filters: --dns --route --interface --ports --connections --host --all
  Options: --interval 2s --count N --auto-snapshot --snapshot-prefix NAME
  With --json, watch emits one NDJSON batch per line.

Path diagnostics:
  ping  measures ICMP reachability, packet loss, RTT, and jitter.
  trace shows the hop path using tracert/traceroute (tracepath fallback on Linux).
  path combines ping quality and route tracing into one report.

MCP:
  nuntius mcp starts a local stdio MCP server. It writes protocol data only to stdout.
`, version.Version)
}

func Main() int {
	app, err := New(os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.Run(ctx, os.Args[1:]); err != nil {
		if errors.Is(err, context.Canceled) {
			return 130
		}
		if errors.Is(err, context.DeadlineExceeded) {
			fmt.Fprintln(os.Stderr, "error: command timed out")
			return 1
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}
