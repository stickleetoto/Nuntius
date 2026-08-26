package platform

import "testing"

func TestNormalizeDestinationDefault(t *testing.T) {
	for _, input := range []string{"default", "0/0", "0.0.0.0/0"} {
		if got := normalizeDestination(input); got != "0.0.0.0/0" {
			t.Fatalf("%q -> %q", input, got)
		}
	}
}
