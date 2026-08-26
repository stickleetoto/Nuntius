# Changelog

## 0.4.0

### Added

- `nuntius ping` with ICMP reachability samples, loss percentage, RTT min/avg/max, and jitter.
- `nuntius trace` with normalized cross-platform hop output.
- `nuntius path` combining quality and route information.
- Core `PathProbe` port and `PathService` so platform commands stay out of CLI/MCP/core policy code.
- Windows path backend using `ping` and `tracert`.
- Linux path backend using `ping`, `traceroute`, and `tracepath` fallback.
- macOS path backend using `ping` and `traceroute`.
- MCP tools `nuntius_ping`, `nuntius_trace`, and `nuntius_path`.
- Unit tests for RTT parsing, trace parsing, path report behavior, and CLI path options.

### Changed

- Default finite-command timeout increased from 15s to 60s for realistic traceroute runs.
- Path reachability logic does not treat ICMP echo filtering alone as definitive host unreachability when tracing reaches the target.
- Version advanced to 0.4.0.

### Compatibility

- Snapshot schema remains version 1.
- Existing v0.1-v0.3.1 snapshots and CLI/MCP commands remain supported.
- No third-party Go dependencies were added.

### Validation

- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- local Linux ping/trace/path smoke tests against `127.0.0.1`
- MCP initialize/tools/list + `nuntius_ping` smoke test
- Windows/Linux/macOS cross-builds for amd64 and arm64
