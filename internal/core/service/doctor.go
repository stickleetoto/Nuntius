package service

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strconv"
	"strings"
	"time"

	"nuntius/internal/core/model"
	"nuntius/internal/core/port"
)

const defaultProbeTimeout = 4 * time.Second

type DoctorService struct {
	Collector port.Collector
	Resolver  *net.Resolver
	Timeout   time.Duration
}

type doctorTarget struct {
	Raw    string
	Host   string
	Port   int
	Scheme string
	URL    string
}

func (s DoctorService) Diagnose(ctx context.Context, target string) (model.Diagnosis, error) {
	parsed, err := parseDoctorTarget(target)
	if err != nil {
		return model.Diagnosis{}, err
	}
	started := time.Now()
	result := model.Diagnosis{
		Target:    parsed.Raw,
		Host:      parsed.Host,
		Port:      parsed.Port,
		Scheme:    parsed.Scheme,
		StartedAt: started.UTC(),
		Overall:   "ok",
	}

	probeTimeout := s.Timeout
	if probeTimeout <= 0 {
		probeTimeout = defaultProbeTimeout
	}
	resolver := s.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}

	stateStart := time.Now()
	state, collectErr := s.Collector.Collect(ctx)
	if collectErr != nil {
		result.Checks = append(result.Checks, check("network_state", "local", model.CheckFail, stateStart, collectErr.Error(), nil))
		result.Overall = "failed"
		finishDiagnosis(&result, started)
		return result, nil
	}
	result.Checks = append(result.Checks, localInterfaceCheck(state))

	resolved := []string{}
	if ip := net.ParseIP(parsed.Host); ip != nil {
		resolved = append(resolved, ip.String())
		result.Checks = append(result.Checks, model.CheckResult{
			Name:    "dns_resolution",
			Layer:   "dns",
			Status:  model.CheckSkipped,
			Message: "target is already an IP address",
		})
	} else {
		stepStart := time.Now()
		stepCtx, cancel := context.WithTimeout(ctx, probeTimeout)
		ips, lookupErr := resolver.LookupIPAddr(stepCtx, parsed.Host)
		cancel()
		if lookupErr != nil {
			result.Checks = append(result.Checks, check("dns_resolution", "dns", model.CheckFail, stepStart, lookupErr.Error(), nil))
			result.Checks = append(result.Checks, model.CheckResult{Name: "target_route", Layer: "route", Status: model.CheckSkipped, Message: "DNS resolution failed"})
			appendSkippedNetworkChecks(&result, parsed, "DNS resolution failed")
			result.Overall = "failed"
			finishDiagnosis(&result, started)
			return result, nil
		}
		for _, addr := range ips {
			resolved = append(resolved, addr.IP.String())
		}
		resolved = uniqueNonEmpty(resolved)
		result.Checks = append(result.Checks, check("dns_resolution", "dns", model.CheckPass, stepStart, fmt.Sprintf("resolved %d address(es)", len(resolved)), map[string]any{"addresses": resolved}))
	}
	result.Resolved = resolved
	if len(resolved) == 0 {
		result.Checks = append(result.Checks, model.CheckResult{Name: "target_route", Layer: "route", Status: model.CheckFail, Message: "no target address available"})
		result.Checks = append(result.Checks, model.CheckResult{Name: "tcp_connect", Layer: "tcp", Status: model.CheckFail, Message: "no target address available"})
		appendSkippedAfterTCP(&result, parsed, "no target address available")
		result.Overall = "failed"
		finishDiagnosis(&result, started)
		return result, nil
	}

	result.Checks = append(result.Checks, targetRouteCheck(state, resolved))

	dialer := &net.Dialer{Timeout: probeTimeout}
	stepStart := time.Now()
	address, dialErr := dialResolved(ctx, dialer, resolved, parsed.Port, probeTimeout)
	if dialErr != nil {
		result.Checks = append(result.Checks, check("tcp_connect", "tcp", model.CheckFail, stepStart, dialErr.Error(), map[string]any{"addresses": resolved, "port": parsed.Port}))
		appendSkippedAfterTCP(&result, parsed, "TCP connection failed")
		result.Overall = "failed"
		finishDiagnosis(&result, started)
		return result, nil
	}
	result.Checks = append(result.Checks, check("tcp_connect", "tcp", model.CheckPass, stepStart, "TCP connection established", map[string]any{"address": address}))

	if parsed.Scheme == "https" {
		stepStart = time.Now()
		stepCtx, cancel := context.WithTimeout(ctx, probeTimeout)
		tlsDialer := tls.Dialer{
			NetDialer: &net.Dialer{Timeout: probeTimeout},
			Config: &tls.Config{
				ServerName: parsed.Host,
				MinVersion: tls.VersionTLS12,
			},
		}
		tlsConn, tlsErr := tlsDialer.DialContext(stepCtx, "tcp", address)
		cancel()
		if tlsErr != nil {
			result.Checks = append(result.Checks, check("tls_handshake", "tls", model.CheckFail, stepStart, tlsErr.Error(), nil))
			result.Checks = append(result.Checks, model.CheckResult{Name: "http_request", Layer: "http", Status: model.CheckSkipped, Message: "TLS handshake failed"})
			result.Overall = "failed"
			finishDiagnosis(&result, started)
			return result, nil
		}
		tlsState := tlsConn.(*tls.Conn).ConnectionState()
		_ = tlsConn.Close()
		tlsDetails := map[string]any{
			"version":             tlsVersionName(tlsState.Version),
			"cipher":              tls.CipherSuiteName(tlsState.CipherSuite),
			"negotiated_protocol": tlsState.NegotiatedProtocol,
			"resumed":             tlsState.DidResume,
		}
		if len(tlsState.PeerCertificates) > 0 {
			cert := tlsState.PeerCertificates[0]
			tlsDetails["certificate_subject"] = cert.Subject.CommonName
			tlsDetails["certificate_issuer"] = cert.Issuer.CommonName
			tlsDetails["certificate_not_before"] = cert.NotBefore.UTC().Format(time.RFC3339)
			tlsDetails["certificate_not_after"] = cert.NotAfter.UTC().Format(time.RFC3339)
			tlsDetails["certificate_days_remaining"] = int64(time.Until(cert.NotAfter).Hours() / 24)
			tlsDetails["certificate_dns_names"] = append([]string(nil), cert.DNSNames...)
		}
		result.Checks = append(result.Checks, check("tls_handshake", "tls", model.CheckPass, stepStart, "TLS handshake succeeded", tlsDetails))
	} else {
		result.Checks = append(result.Checks, model.CheckResult{Name: "tls_handshake", Layer: "tls", Status: model.CheckSkipped, Message: "plain HTTP target"})
	}

	stepStart = time.Now()
	stepCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	request, reqErr := http.NewRequestWithContext(stepCtx, http.MethodHead, parsed.URL, nil)
	if reqErr != nil {
		cancel()
		return model.Diagnosis{}, reqErr
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: probeTimeout}).DialContext,
		TLSHandshakeTimeout:   probeTimeout,
		ResponseHeaderTimeout: probeTimeout,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
	}
	redirects := []string{}
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			redirects = append(redirects, req.URL.String())
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			return nil
		},
	}
	var firstResponseByte time.Time
	trace := &httptrace.ClientTrace{
		GotFirstResponseByte: func() { firstResponseByte = time.Now() },
	}
	request = request.WithContext(httptrace.WithClientTrace(request.Context(), trace))
	resp, httpErr := client.Do(request)
	cancel()
	transport.CloseIdleConnections()
	if httpErr != nil {
		details := map[string]any{}
		if len(redirects) > 0 {
			details["redirects"] = redirects
		}
		if proxyURL, proxyErr := http.ProxyFromEnvironment(request); proxyErr == nil && proxyURL != nil {
			details["proxy"] = proxyURL.String()
		}
		if !firstResponseByte.IsZero() {
			details["ttfb_ms"] = firstResponseByte.Sub(stepStart).Milliseconds()
		}
		result.Checks = append(result.Checks, check("http_request", "http", model.CheckFail, stepStart, httpErr.Error(), details))
		result.Overall = "failed"
	} else {
		_ = resp.Body.Close()
		details := map[string]any{
			"status_code": resp.StatusCode,
			"final_url":   resp.Request.URL.String(),
		}
		if server := strings.TrimSpace(resp.Header.Get("Server")); server != "" {
			details["server"] = server
		}
		if contentType := strings.TrimSpace(resp.Header.Get("Content-Type")); contentType != "" {
			details["content_type"] = contentType
		}
		if len(redirects) > 0 {
			details["redirects"] = redirects
		}
		if proxyURL, proxyErr := http.ProxyFromEnvironment(request); proxyErr == nil && proxyURL != nil {
			details["proxy"] = proxyURL.String()
		}
		if !firstResponseByte.IsZero() {
			details["ttfb_ms"] = firstResponseByte.Sub(stepStart).Milliseconds()
		}
		result.Checks = append(result.Checks, check("http_request", "http", model.CheckPass, stepStart, resp.Status, details))
	}

	for _, c := range result.Checks {
		if c.Status == model.CheckFail {
			result.Overall = "failed"
			break
		}
	}
	finishDiagnosis(&result, started)
	return result, nil
}

func finishDiagnosis(result *model.Diagnosis, started time.Time) {
	result.DurationMS = time.Since(started).Milliseconds()
	performance := map[string]int64{}
	for _, c := range result.Checks {
		if c.Status == model.CheckSkipped {
			continue
		}
		switch c.Layer {
		case "dns":
			performance["dns"] = c.DurationMS
		case "tcp":
			performance["tcp"] = c.DurationMS
		case "tls":
			performance["tls"] = c.DurationMS
		case "http":
			performance["http_total"] = c.DurationMS
			if value, ok := c.Details["ttfb_ms"]; ok {
				switch n := value.(type) {
				case int64:
					performance["http_ttfb"] = n
				case int:
					performance["http_ttfb"] = int64(n)
				case float64:
					performance["http_ttfb"] = int64(n)
				}
			}
		}
	}
	if len(performance) > 0 {
		result.Performance = performance
	}
}

func parseDoctorTarget(raw string) (doctorTarget, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return doctorTarget{}, errors.New("doctor target is required")
	}

	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" {
			return doctorTarget{}, fmt.Errorf("invalid target %q", raw)
		}
		scheme := strings.ToLower(u.Scheme)
		if scheme != "http" && scheme != "https" {
			return doctorTarget{}, fmt.Errorf("unsupported scheme %q (use http or https)", scheme)
		}
		port := 80
		if scheme == "https" {
			port = 443
		}
		if p := u.Port(); p != "" {
			n, err := strconv.Atoi(p)
			if err != nil || n < 1 || n > 65535 {
				return doctorTarget{}, fmt.Errorf("invalid port %q", p)
			}
			port = n
		}
		if u.Path == "" {
			u.Path = "/"
		}
		return doctorTarget{Raw: raw, Host: u.Hostname(), Port: port, Scheme: scheme, URL: u.String()}, nil
	}

	if ip := net.ParseIP(raw); ip != nil {
		u := &url.URL{Scheme: "https", Host: net.JoinHostPort(ip.String(), "443"), Path: "/"}
		return doctorTarget{Raw: raw, Host: ip.String(), Port: 443, Scheme: "https", URL: u.String()}, nil
	}

	if host, portText, err := net.SplitHostPort(raw); err == nil {
		port, err := strconv.Atoi(portText)
		if err != nil || port < 1 || port > 65535 || host == "" {
			return doctorTarget{}, fmt.Errorf("invalid target %q", raw)
		}
		scheme := "http"
		if port == 443 {
			scheme = "https"
		}
		u := &url.URL{Scheme: scheme, Host: net.JoinHostPort(host, portText), Path: "/"}
		return doctorTarget{Raw: raw, Host: host, Port: port, Scheme: scheme, URL: u.String()}, nil
	}

	host := strings.TrimSuffix(raw, ".")
	if host == "" || strings.ContainsAny(host, " /\\") {
		return doctorTarget{}, fmt.Errorf("invalid target %q", raw)
	}
	u := &url.URL{Scheme: "https", Host: host, Path: "/"}
	return doctorTarget{Raw: raw, Host: host, Port: 443, Scheme: "https", URL: u.String()}, nil
}

func localInterfaceCheck(state model.NetworkState) model.CheckResult {
	start := time.Now()
	for _, iface := range state.Interfaces {
		if !hasFlag(iface.Flags, "up") || hasFlag(iface.Flags, "loopback") {
			continue
		}
		if len(iface.Addresses) == 0 {
			continue
		}
		return check("interface_ready", "interface", model.CheckPass, start, "active non-loopback interface found", map[string]any{"interface": iface.Name})
	}
	return check("interface_ready", "interface", model.CheckFail, start, "no active non-loopback interface with an address", nil)
}

func targetRouteCheck(state model.NetworkState, resolved []string) model.CheckResult {
	start := time.Now()
	for _, targetText := range resolved {
		target := net.ParseIP(targetText)
		if target == nil {
			continue
		}
		if target.IsLoopback() {
			return check("target_route", "route", model.CheckSkipped, start, "loopback target does not require routing", map[string]any{"target": target.String()})
		}
		if route, prefix, ok := bestRouteForIP(state.Routes, target); ok {
			details := map[string]any{
				"target":      target.String(),
				"destination": route.Destination,
				"gateway":     route.Gateway,
				"interface":   route.Interface,
				"metric":      route.Metric,
				"prefix_len":  prefix,
			}
			message := "matching route found"
			if prefix == 0 {
				message = "default route selected"
			}
			return check("target_route", "route", model.CheckPass, start, message, details)
		}
	}
	for _, warning := range state.Warnings {
		if strings.HasPrefix(strings.ToLower(warning), "routes:") {
			return check("target_route", "route", model.CheckSkipped, start, "route information unavailable: "+strings.TrimSpace(strings.TrimPrefix(warning, "routes:")), nil)
		}
	}
	return check("target_route", "route", model.CheckFail, start, "no matching route detected for resolved target", map[string]any{"addresses": resolved})
}

func bestRouteForIP(routes []model.Route, target net.IP) (model.Route, int, bool) {
	bestPrefix := -1
	bestMetric := int(^uint(0) >> 1)
	var best model.Route
	found := false
	for _, route := range routes {
		prefix, ok := routePrefixForTarget(route.Destination, target)
		if !ok {
			continue
		}
		metric := route.Metric
		if !found || prefix > bestPrefix || (prefix == bestPrefix && metric < bestMetric) {
			best, bestPrefix, bestMetric, found = route, prefix, metric, true
		}
	}
	return best, bestPrefix, found
}

func routePrefixForTarget(destination string, target net.IP) (int, bool) {
	destination = strings.TrimSpace(destination)
	if strings.EqualFold(destination, "default") || destination == "0/0" {
		return 0, true
	}
	if destination == "0.0.0.0/0" {
		return 0, target.To4() != nil
	}
	if destination == "::/0" {
		return 0, target.To4() == nil
	}
	_, network, err := net.ParseCIDR(destination)
	if err != nil || !network.Contains(target) {
		return 0, false
	}
	ones, bits := network.Mask.Size()
	if bits == 32 && target.To4() == nil {
		return 0, false
	}
	if bits == 128 && target.To4() != nil {
		return 0, false
	}
	return ones, true
}

func dialResolved(ctx context.Context, dialer *net.Dialer, addresses []string, port int, timeout time.Duration) (string, error) {
	var errorsSeen []string
	for _, ip := range addresses {
		address := net.JoinHostPort(ip, strconv.Itoa(port))
		stepCtx, cancel := context.WithTimeout(ctx, timeout)
		conn, err := dialer.DialContext(stepCtx, "tcp", address)
		cancel()
		if err == nil {
			_ = conn.Close()
			return address, nil
		}
		errorsSeen = append(errorsSeen, address+": "+err.Error())
		if ctx.Err() != nil {
			break
		}
	}
	if len(errorsSeen) == 0 {
		return "", errors.New("no resolved address available")
	}
	return "", errors.New(strings.Join(errorsSeen, "; "))
}

func appendSkippedNetworkChecks(result *model.Diagnosis, target doctorTarget, reason string) {
	result.Checks = append(result.Checks, model.CheckResult{Name: "tcp_connect", Layer: "tcp", Status: model.CheckSkipped, Message: reason})
	appendSkippedAfterTCP(result, target, reason)
}

func appendSkippedAfterTCP(result *model.Diagnosis, target doctorTarget, reason string) {
	if target.Scheme == "https" {
		result.Checks = append(result.Checks, model.CheckResult{Name: "tls_handshake", Layer: "tls", Status: model.CheckSkipped, Message: reason})
	} else {
		result.Checks = append(result.Checks, model.CheckResult{Name: "tls_handshake", Layer: "tls", Status: model.CheckSkipped, Message: "plain HTTP target"})
	}
	result.Checks = append(result.Checks, model.CheckResult{Name: "http_request", Layer: "http", Status: model.CheckSkipped, Message: reason})
}

func check(name, layer string, status model.CheckStatus, started time.Time, message string, details map[string]any) model.CheckResult {
	return model.CheckResult{Name: name, Layer: layer, Status: status, DurationMS: time.Since(started).Milliseconds(), Message: message, Details: details}
}

func hasFlag(flags []string, want string) bool {
	for _, flag := range flags {
		if strings.EqualFold(strings.TrimSpace(flag), want) {
			return true
		}
	}
	return false
}

func uniqueNonEmpty(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func tlsVersionName(v uint16) string {
	switch v {
	case tls.VersionTLS13:
		return "TLS 1.3"
	case tls.VersionTLS12:
		return "TLS 1.2"
	default:
		return fmt.Sprintf("0x%04x", v)
	}
}
