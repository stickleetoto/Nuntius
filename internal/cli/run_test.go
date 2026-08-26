package cli

import (
	"testing"
	"time"
)

func TestParseGlobalArgsTimeout(t *testing.T) {
	t.Setenv("NUNTIUS_TIMEOUT", "")
	got, err := parseGlobalArgs([]string{"doctor", "example.com", "--json", "--timeout", "3s"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.json || got.timeout != 3*time.Second || len(got.args) != 2 {
		t.Fatalf("unexpected parse: %#v", got)
	}
}

func TestParseWatchArgs(t *testing.T) {
	got, err := parseWatchArgs([]string{"--interval", "1s", "--dns", "--ports", "--count=3", "--auto-snapshot"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Interval != time.Second || got.Count != 3 || !got.AutoSnapshot {
		t.Fatalf("unexpected watch options: %#v", got)
	}
	if len(got.Categories) != 2 || got.Categories[0] != "dns" || got.Categories[1] != "listener" {
		t.Fatalf("unexpected categories: %#v", got.Categories)
	}
}

func TestParseWatchArgsRejectsFastPolling(t *testing.T) {
	if _, err := parseWatchArgs([]string{"--interval", "100ms"}); err == nil {
		t.Fatal("expected interval validation error")
	}
}

func TestFriendlyWatchValueListener(t *testing.T) {
	got := friendlyWatchValue("listener", "tcp|0.0.0.0:8080|pid=42|proc=demo.exe")
	want := "TCP 0.0.0.0:8080 pid=42 proc=demo.exe"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestFriendlyWatchValueRoute(t *testing.T) {
	got := friendlyWatchValue("route", "0.0.0.0/0|via=192.168.1.1|if=Wi-Fi|metric=25")
	want := "0.0.0.0/0 via 192.168.1.1 dev Wi-Fi metric=25"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestParsePathArgs(t *testing.T) {
	target, got, err := parsePathArgs([]string{"example.com", "--count=5", "--max-hops", "12", "--probe-timeout=1500ms"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target != "example.com" || got.Count != 5 || got.MaxHops != 12 || got.ProbeTimeout != 1500*time.Millisecond {
		t.Fatalf("unexpected path args: target=%q opts=%#v", target, got)
	}
}
