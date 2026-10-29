package tui

import (
	"strings"
	"testing"
	"time"
)

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

func TestStatusSummary(t *testing.T) {
	seal := map[string]interface{}{"sealed": false, "version": "1.18.2", "storage_type": "inmem"}
	health := map[string]interface{}{"standby": false, "cluster_name": "vault-dev"}
	got := statusSummary(seal, health)
	for _, want := range []string{"UNSEALED", "v1.18.2", "inmem", "active", "cluster: vault-dev"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary missing %q: %s", want, got)
		}
	}

	got = statusSummary(map[string]interface{}{"sealed": true}, nil)
	for _, want := range []string{"SEALED"} {
		if !strings.Contains(got, want) {
			t.Errorf("sealed summary missing %q: %s", want, got)
		}
	}
	if strings.Contains(got, "active") {
		t.Errorf("sealed summary must not claim active: %s", got)
	}

	if got := statusSummary(nil, nil); got != "" {
		t.Errorf("empty summary = %q, want \"\"", got)
	}
}

func TestTokenExpiryLabel(t *testing.T) {
	at := time.Now().Add(-90 * time.Second)
	if got := tokenExpiryLabel(1800, at); got != "28m30s left" {
		t.Errorf("countdown = %q, want %q", got, "28m30s left")
	}
	if got := tokenExpiryLabel(60, at); got != "expired" {
		t.Errorf("past expiry = %q, want %q", got, "expired")
	}
	if got := tokenExpiryLabel(0, at); got != "no expiry" {
		t.Errorf("zero ttl = %q, want %q", got, "no expiry")
	}
	if got := tokenExpiryLabel(-1, at); got != "" {
		t.Errorf("unknown ttl = %q, want \"\"", got)
	}
	if got := tokenExpiryLabel(0, time.Time{}); got != "no expiry" {
		t.Errorf("zero-value time = %q, want %q", got, "no expiry")
	}
}
