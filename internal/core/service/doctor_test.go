package service

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nuntius/internal/core/model"
)

type fakeCollector struct{ state model.NetworkState }

func (f fakeCollector) Collect(context.Context) (model.NetworkState, error) { return f.state, nil }

func TestParseDoctorTargetDefaultsToHTTPS(t *testing.T) {
	got, err := parseDoctorTarget("example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got.Host != "example.com" || got.Port != 443 || got.Scheme != "https" {
		t.Fatalf("unexpected target: %#v", got)
	}
}

func TestDoctorAgainstLocalHTTPServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	state := model.NetworkState{
		Interfaces: []model.NetworkInterface{{Name: "eth0", Flags: []string{"up"}, Addresses: []model.IPAddress{{Address: "192.0.2.2", Family: "ipv4"}}}},
		Routes:     []model.Route{{Destination: "0.0.0.0/0", Gateway: "192.0.2.1", Interface: "eth0"}},
	}
	doctor := DoctorService{Collector: fakeCollector{state: state}, Timeout: time.Second}
	got, err := doctor.Diagnose(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got.Overall != "ok" {
		t.Fatalf("expected ok diagnosis: %#v", got)
	}
	joined := ""
	for _, c := range got.Checks {
		joined += c.Name + ":" + string(c.Status) + ";"
	}
	if !strings.Contains(joined, "tcp_connect:pass") || !strings.Contains(joined, "http_request:pass") {
		t.Fatalf("missing successful network checks: %s", joined)
	}
	if _, ok := got.Performance["tcp"]; !ok {
		t.Fatalf("missing TCP timing: %#v", got.Performance)
	}
	if _, ok := got.Performance["http_total"]; !ok {
		t.Fatalf("missing HTTP timing: %#v", got.Performance)
	}
	if _, ok := got.Performance["http_ttfb"]; !ok {
		t.Fatalf("missing HTTP TTFB timing: %#v", got.Performance)
	}
}

func TestBestRouteForIPUsesLongestPrefixThenMetric(t *testing.T) {
	routes := []model.Route{
		{Destination: "0.0.0.0/0", Gateway: "192.0.2.1", Interface: "wan0", Metric: 5},
		{Destination: "10.0.0.0/8", Gateway: "10.0.0.1", Interface: "corp0", Metric: 50},
		{Destination: "10.20.0.0/16", Gateway: "10.20.0.1", Interface: "vpn0", Metric: 20},
		{Destination: "10.20.0.0/16", Gateway: "10.20.0.2", Interface: "vpn1", Metric: 10},
	}
	route, prefix, ok := bestRouteForIP(routes, net.ParseIP("10.20.30.40"))
	if !ok {
		t.Fatal("expected a route")
	}
	if prefix != 16 || route.Interface != "vpn1" {
		t.Fatalf("unexpected best route: prefix=%d route=%#v", prefix, route)
	}
}

func TestRoutePrefixForTargetKeepsAddressFamiliesSeparate(t *testing.T) {
	if _, ok := routePrefixForTarget("0.0.0.0/0", net.ParseIP("2001:db8::1")); ok {
		t.Fatal("IPv4 default route must not match IPv6 target")
	}
	if _, ok := routePrefixForTarget("::/0", net.ParseIP("192.0.2.10")); ok {
		t.Fatal("IPv6 default route must not match IPv4 target")
	}
}
