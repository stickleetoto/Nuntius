# Nuntius Architecture — v0.4

## Principle

Nuntius has one core and multiple interfaces. Human CLI and local MCP must call the same services and receive the same transport-neutral models.

```text
                    CLI
                     |
                     v
Platform -> Ports -> Core Services <- Storage
                     ^
                     |
                    MCP
```

## Core services

- `InfoService`: current normalized network state
- `SnapshotService`: state persistence
- `DiffService`: semantic state comparison
- `DoctorService`: interface/route/DNS/TCP/TLS/HTTP diagnosis
- `WatchService`: polling-based state change observation
- `PathService`: ICMP quality and hop-path diagnostics

## Ports

`internal/core/port` defines replaceable boundaries:

- `Collector`
- `SnapshotStorage`
- `PathProbe`

`PathService` knows nothing about `ping.exe`, `tracert`, `traceroute`, or `tracepath`. Those commands live behind the platform `PathProbe` implementation.

## v0.4 path flow

```text
nuntius ping/trace/path
        |
        v
    PathService
        |
        v
     PathProbe
        |
  +-----+------------------+
  |            |           |
Windows       Linux       macOS
ping/tracert  ping +      ping +
              traceroute  traceroute
              /tracepath
```

The platform layer converts OS command output into:

- `PingResult`
- `PingSample`
- `TraceResult`
- `TraceHop`

The CLI only formats these models; MCP returns the same values as structured content.

### Ping quality

For each requested echo sample Nuntius records success/failure and RTT. It derives:

- sent / received
- packet-loss percentage
- minimum / average / maximum RTT
- jitter as mean absolute change between consecutive successful RTT observations

This jitter value is a practical diagnostic summary, not an RTP/RFC-specific jitter estimator.

### Trace behavior

Nuntius asks the platform traceroute tool for numeric-address output and normalizes hop number, address, RTT, and timeout status. Windows `tracert` returns multiple probes per hop; v0.4 reports their parsed mean RTT. Linux/macOS request one probe per hop when supported.

### Reachability interpretation

ICMP echo may be filtered even when TCP/HTTP traffic works. Therefore `PathReport` treats zero ping replies plus a traceroute that still reaches the target as `degraded`, not definitively unreachable.

## Doctor vs Path

`DoctorService` answers "which application connectivity layer failed?".

`PathService` answers "what are packet loss, RTT variation, and hop-path behavior?".

They intentionally remain separate so a normal `doctor` run does not become slow due to a full traceroute.

## Watch

Watch continues to reuse normalized snapshot-diff semantics. Polling keeps behavior portable; active connections remain opt-in because of churn.

## Timeout/cancellation

Finite CLI commands default to a 60-second overall context in v0.4 because traceroute can legitimately take longer than earlier diagnostic commands. `--timeout` and `NUNTIUS_TIMEOUT` override it.

Path probes also have explicit per-probe/per-hop `--probe-timeout` values. OS commands run via `exec.CommandContext` so cancellation propagates.

## Safety boundary

Current network operations are inspection/probing only. Nuntius does not change DNS, routes, interfaces, firewall rules, or OS network configuration. Snapshot persistence is limited to Nuntius-owned files.
