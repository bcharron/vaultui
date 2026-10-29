package tui

import "testing"

func TestHumanDuration(t *testing.T) {
	cases := []struct {
		sec  int
		want string
	}{
		{0, "0s"},
		{-5, "0s"},
		{45, "45s"},
		{60, "1m"},
		{65, "1m5s"},
		{3600, "1h"},
		{5400, "1h30m"},
		{86400, "1d"},
		{100000, "1d3h"},
		{2764800, "32d"},
	}
	for _, c := range cases {
		if got := humanDuration(c.sec); got != c.want {
			t.Errorf("humanDuration(%d) = %q, want %q", c.sec, got, c.want)
		}
	}
}

func TestPlural(t *testing.T) {
	cases := []struct {
		n    int
		noun string
		want string
	}{
		{1, "keys", "key"},
		{2, "keys", "keys"},
		{1, "policies", "policy"},
		{1, "leases", "lease"},
		{1, "entities", "entity"},
		{3, "mounts", "mounts"},
	}
	for _, c := range cases {
		if got := plural(c.n, c.noun); got != c.want {
			t.Errorf("plural(%d, %q) = %q, want %q", c.n, c.noun, got, c.want)
		}
	}
}

func TestIsDurationKey(t *testing.T) {
	for _, k := range []string{"ttl", "TTL", "lease_duration", "max_lease_ttl", "token_ttl", "period"} {
		if !isDurationKey(k) {
			t.Errorf("isDurationKey(%q) = false, want true", k)
		}
	}
	for _, k := range []string{"version", "created_time", "num_uses", "periodic"} {
		if isDurationKey(k) {
			t.Errorf("isDurationKey(%q) = true, want false", k)
		}
	}
}
