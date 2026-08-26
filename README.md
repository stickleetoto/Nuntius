# Nuntius CLI

**Cross-platform network inspection, snapshot/diff, live watch, layered diagnosis, path quality, and local MCP access.**

Nuntius is a read-oriented network diagnostic utility for Windows, Linux, and macOS. The CLI and MCP server share the same core services and structured models.

## v0.4 commands

```text
nuntius info [--json]
nuntius dns [--json]
nuntius routes [--json]
nuntius ports [--json]
nuntius connections [--json]
nuntius snapshot <name> [--json]
nuntius list [--json]
nuntius show <name> [--json]
nuntius diff <from> <to> [--json]
nuntius doctor <host|host:port|url> [--json]
nuntius ping <host> [--count 4] [--probe-timeout 2s] [--json]
nuntius trace <host> [--max-hops 20] [--probe-timeout 2s] [--json]
nuntius path <host> [--count 4] [--max-hops 20] [--probe-timeout 2s] [--json]
nuntius watch [--interval 2s] [--json] [filters] [--auto-snapshot]
nuntius mcp
nuntius version
```

Global finite-command timeout defaults to 60 seconds and can be changed with `--timeout` or `NUNTIUS_TIMEOUT`.

## New in v0.4

- `ping`: ICMP reachability sampling with sent/received/loss statistics.
- RTT min/avg/max plus a simple jitter metric (mean absolute RTT delta between consecutive successful samples).
- `trace`: normalized hop tracing on Windows/Linux/macOS.
- `path`: one report combining ping quality and route tracing.
- MCP tools `nuntius_ping`, `nuntius_trace`, and `nuntius_path`.
- Structured path models shared by CLI JSON and MCP structured content.
- Linux uses `traceroute` with `tracepath` fallback; Windows uses `tracert`; macOS uses `traceroute`.
- Existing snapshot schema remains version 1.

## Path examples

```bash
nuntius ping 1.1.1.1
nuntius ping example.com --count 10 --probe-timeout 1s
nuntius trace example.com --max-hops 24
nuntius path example.com
nuntius path example.com --json
```

Example human output:

```text
Nuntius Ping: example.com (93.184.216.34)
  #1  12.40ms
  #2  13.10ms
  #3  12.80ms
  #4  13.00ms

Sent 4  Received 4  Loss 0.0%
RTT  min 12.40ms  avg 12.83ms  max 13.10ms  jitter 0.40ms
```

### Path implementation note

v0.4 intentionally uses the operating system's normal ICMP/traceroute programs instead of requiring raw-socket privileges. Nuntius parses and normalizes their results into one cross-platform model. This keeps ordinary use unprivileged on common systems.

Required commands:

- Windows: `ping`, `tracert` (normally built in)
- Linux: `ping`, plus `traceroute` or `tracepath`
- macOS: `ping`, `traceroute` (normally available)

If a required path command is missing, Nuntius returns a clear error/warning instead of silently inventing path data.

## Doctor

`doctor` remains the layered application-connectivity check:

```text
local interface -> target route -> DNS -> TCP -> TLS -> HTTP
```

It reports DNS/TCP/TLS/HTTP timing, target-aware longest-prefix route selection, certificate information, redirects, selected HTTP response metadata, and environment-proxy use.

Use `path` alongside `doctor` when the question is packet loss, jitter, or hop path rather than application-layer connectivity.

## Watch

Default categories are interface, DNS, route, and listening ports. Active connections are opt-in.

```bash
nuntius watch
nuntius watch --dns --route
nuntius watch --all
nuntius watch --json
nuntius watch --auto-snapshot --snapshot-prefix lab
```

Filters: `--dns --route --interface --ports --connections --host --all`

## MCP

Start the local stdio server:

```bash
nuntius mcp
```

Current tools:

```text
nuntius_info
nuntius_dns
nuntius_routes
nuntius_ports
nuntius_connections
nuntius_snapshot
nuntius_list_snapshots
nuntius_show_snapshot
nuntius_diff
nuntius_doctor
nuntius_ping
nuntius_trace
nuntius_path
nuntius_watch_once
```

Example generic MCP config:

```json
{
  "mcpServers": {
    "nuntius": {
      "command": "/absolute/path/to/nuntius",
      "args": ["mcp"]
    }
  }
}
```

MCP remains local stdio and read-oriented. Snapshot creation only writes Nuntius-owned local snapshot files.

## Storage and compatibility

By default Nuntius uses the operating-system user config directory and creates `Nuntius/snapshots`. Set `NUNTIUS_HOME` to override it.

Snapshot schema is still version 1, so earlier v0.1-v0.3.1 snapshot files stay readable.

## Platform state collection

- Windows: Go interfaces, `Get-NetRoute`/`route`, `Get-DnsClientServerAddress`, `netstat`, `tasklist`
- Linux: Go interfaces, `ip route`, `/etc/resolv.conf`, `ss`
- macOS: Go interfaces, `netstat`, `scutil --dns`, `lsof`

## Build

No third-party Go dependencies are required.

```bash
go build -o nuntius ./cmd/nuntius
```

Release builds are provided for Windows/Linux/macOS on amd64 and arm64.
