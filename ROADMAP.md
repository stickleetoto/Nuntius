# Nuntius Roadmap

## v0.1 — Network state and diff

- [x] cross-platform collector boundary
- [x] `info`, snapshot/list/show/diff
- [x] JSON snapshot storage

## v0.2 — Agent and layered diagnosis

- [x] local MCP stdio server
- [x] layered `doctor`
- [x] Windows process enrichment and route normalization
- [x] timeout/cancellation policy

## v0.3 — Live observation

- [x] `watch`
- [x] category filters and NDJSON
- [x] auto snapshots on change
- [x] doctor performance summary / HTTP TTFB
- [x] MCP finite watch observation

## v0.3.1 — Inspection hardening

- [x] focused `dns`, `routes`, `ports`, `connections`
- [x] target-aware route selection
- [x] richer TLS/HTTP metadata

## v0.4 — Path and latency diagnostics

- [x] `ping <target>`
- [x] packet loss sampling
- [x] RTT min/avg/max
- [x] jitter sampling
- [x] `trace <target>`
- [x] normalized hop timing/path output
- [x] `path <target>` combined path-quality report
- [x] MCP ping/trace/path tools
- [x] Linux traceroute/tracepath fallback
- [x] test/race/vet + six-target cross-build validation

## v0.4.x follow-up

- [ ] field-test localized Windows/macOS ping/traceroute output
- [ ] IPv6 path-command parity testing
- [ ] compare two trace results / path-change diff
- [ ] resolver-specific DNS latency comparison
- [ ] optionally replace command parsing with native ICMP APIs where privilege-free and reliable
- [ ] migrate MCP boundary to official Go MCP SDK when dependency/toolchain policy is adopted

## v0.5 — Environment intelligence

- [ ] Wi-Fi SSID/BSSID/link detail where safely available
- [ ] proxy/VPN detection
- [ ] MTU/link-speed/DHCP detail
- [ ] firewall visibility (read-only)
- [ ] stronger IPv6 parity
- [ ] recent change-history persistence

## v0.6+ — Packaging and distributed observation

- [ ] Homebrew formula
- [ ] winget manifest
- [ ] Debian/RPM packaging
- [ ] signed release artifacts
- [ ] remote Nuntius node design
- [ ] authenticated remote MCP design
