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

## Orchestrator v0.7.1 Live E2E

> Test-only section for `chatgpt-roadmap-orchestrator`.
> Do not modify Nuntius product code for these tasks.
> Integration method: merge or fast-forward only.
> Worker results must be independently verified before completion.

### Stage 1 ? ChatGPT single-worker

- [ ] E2E-01 ChatGPT browser worker proof
  <!-- orchestrator: id=E2E-01 worker=chatgpt-browser parallel=deny writes=orchestrator-e2e/chatgpt -->

  Create `orchestrator-e2e/chatgpt/proof.md`.

  Acceptance:
  - file exists
  - contains `E2E-01`
  - contains `worker=chatgpt-browser`
  - no files outside `orchestrator-e2e/chatgpt` are modified by the worker

### Stage 2 ? Claude Local single-worker

- [ ] E2E-02 Claude local worker proof
  <!-- orchestrator: id=E2E-02 worker=claude-local deps=E2E-01 parallel=deny writes=orchestrator-e2e/claude machine=main-pc transport=auto needs=local,windows,terminal -->

  Run the Nuntius test suite locally and create
  `orchestrator-e2e/claude/proof.md`.

  Acceptance:
  - run `go test ./...`
  - record PASS or FAIL in the proof file
  - proof file contains `E2E-02`
  - proof file contains the observed Claude transport (`cli` or `gui`)
  - no files outside `orchestrator-e2e/claude` are modified by the worker

### Stage 3 ? Mixed parallel workers

- [ ] E2E-03 Parallel ChatGPT worker
  <!-- orchestrator: id=E2E-03 worker=chatgpt-browser deps=E2E-02 parallel=allow writes=orchestrator-e2e/parallel/chatgpt -->

  Create `orchestrator-e2e/parallel/chatgpt/proof.md`.

  Acceptance:
  - contains `E2E-03`
  - contains `parallel-worker=chatgpt`
  - modify only the declared write scope

- [ ] E2E-04 Parallel Claude Local worker
  <!-- orchestrator: id=E2E-04 worker=claude-local deps=E2E-02 parallel=allow writes=orchestrator-e2e/parallel/claude machine=main-pc transport=auto needs=local,windows,terminal -->

  Run `go test ./...` locally and create
  `orchestrator-e2e/parallel/claude/proof.md`.

  Acceptance:
  - contains `E2E-04`
  - contains `parallel-worker=claude-local`
  - records whether `go test ./...` passed
  - modify only the declared write scope

