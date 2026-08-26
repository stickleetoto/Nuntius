package platform

import (
	"testing"
	"time"
)

func TestParseRTT(t *testing.T) {
	cases := []struct {
		text string
		want float64
	}{
		{"64 bytes from 1.1.1.1: time=12.4 ms", 12.4},
		{"Reply from 1.1.1.1: bytes=32 time=18ms TTL=56", 18},
		{"Reply from 127.0.0.1: time<1ms", 0.5},
	}
	for _, tc := range cases {
		got := parseRTT(tc.text, 99*time.Millisecond)
		if got != tc.want {
			t.Fatalf("parseRTT(%q)=%v want %v", tc.text, got, tc.want)
		}
	}
}

func TestParseTraceOutput(t *testing.T) {
	out := `traceroute to 8.8.8.8 (8.8.8.8), 20 hops max
 1  192.168.0.1  1.23 ms
 2  *
 3  8.8.8.8  12.50 ms
`
	got := parseTraceOutput(out, "8.8.8.8", "traceroute", 20)
	if len(got.Hops) != 3 {
		t.Fatalf("expected 3 hops, got %#v", got.Hops)
	}
	if got.Hops[0].Address != "192.168.0.1" || got.Hops[1].TimedOut != true || !got.Reached {
		t.Fatalf("unexpected trace parse: %#v", got)
	}
}
