package platform

import (
	"math"
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
	cases := []struct {
		name         string
		output       string
		targetIP     string
		wantRTT      float64
		wantAddress  string
		wantTimedOut bool
		wantReached  bool
	}{
		{
			name:        "all sub-millisecond",
			output:      "1  <1 ms  <1ms  < 1 ms  192.168.0.1\n",
			targetIP:    "192.168.0.1",
			wantRTT:     0.5,
			wantAddress: "192.168.0.1",
			wantReached: true,
		},
		{
			name:        "mixed",
			output:      "1  <1 ms  2 ms  2,5 ms  192.168.0.1\n",
			wantRTT:     (0.5 + 2 + 2.5) / 3,
			wantAddress: "192.168.0.1",
		},
		{
			name:        "localized decimal comma",
			output:      "1  192.168.0.1  1,25 ms  2,75 ms\n",
			wantRTT:     2,
			wantAddress: "192.168.0.1",
		},
		{
			name:        "ordinary",
			output:      "1  8.8.8.8  12.50 ms\n",
			targetIP:    "8.8.8.8",
			wantRTT:     12.50,
			wantAddress: "8.8.8.8",
			wantReached: true,
		},
		{
			name:         "timeout",
			output:       "1  *  *  *\n",
			targetIP:     "8.8.8.8",
			wantTimedOut: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseTraceOutput(tc.output, tc.targetIP, "traceroute", 20)
			if len(got.Hops) != 1 {
				t.Fatalf("expected 1 hop, got %#v", got.Hops)
			}

			hop := got.Hops[0]
			if math.Abs(hop.RTTMS-tc.wantRTT) > 1e-9 {
				t.Fatalf("RTTMS=%v want %v", hop.RTTMS, tc.wantRTT)
			}
			if hop.Address != tc.wantAddress {
				t.Fatalf("Address=%q want %q", hop.Address, tc.wantAddress)
			}
			if hop.TimedOut != tc.wantTimedOut {
				t.Fatalf("TimedOut=%v want %v", hop.TimedOut, tc.wantTimedOut)
			}
			if got.Reached != tc.wantReached {
				t.Fatalf("Reached=%v want %v", got.Reached, tc.wantReached)
			}
		})
	}
}
