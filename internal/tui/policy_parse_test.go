package tui

import (
	"reflect"
	"testing"
)

func TestParsePolicyDoc(t *testing.T) {
	raw := `
# apps team: full control of the apps tree
# second comment line
path "secret/data/apps/*" {
  capabilities = ["read", "list", "create", "delete", "update"]
}

path "transit/sign/jwt-signer" {
  capabilities = ["update"]
}
`
	paths, err := parsePolicyDoc(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("got %d paths, want 2", len(paths))
	}
	// alphabetical order
	if paths[0].path != "secret/data/apps/*" || paths[1].path != "transit/sign/jwt-signer" {
		t.Errorf("paths not sorted: %q, %q", paths[0].path, paths[1].path)
	}
	if !reflect.DeepEqual(paths[0].comments, []string{"apps team: full control of the apps tree", "second comment line"}) {
		t.Errorf("comments = %v", paths[0].comments)
	}
	if paths[1].comments != nil {
		t.Errorf("unexpected comments on second path: %v", paths[1].comments)
	}
	if got := capSummary(paths[0].caps); got != "crudl" {
		t.Errorf("summary = %q, want crudl", got)
	}
	if got := capSummary(paths[1].caps); got != "u" {
		t.Errorf("summary = %q, want u", got)
	}
}

func TestParsePolicyDocAttrs(t *testing.T) {
	raw := `
path "secret/data/payments" {
  capabilities             = ["read", "update"]
  allowed_parameters       = { "ttl" = ["1h", "2h"] }
  denied_parameters        = { "owner" = [] }
  min_wrapping_ttl         = "1m"
}
`
	paths, err := parsePolicyDoc(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("got %d paths, want 1", len(paths))
	}
	pp := paths[0]
	if got := capSummary(pp.caps); got != "ru" {
		t.Errorf("summary = %q, want ru", got)
	}
	if len(pp.attrs) != 3 {
		t.Fatalf("got %d attrs, want 3: %+v", len(pp.attrs), pp.attrs)
	}
	want := []policyAttr{
		{name: "allowed_parameters", values: []string{"ttl: [1h, 2h]"}},
		{name: "denied_parameters", values: []string{"owner: []"}},
		{name: "min_wrapping_ttl", values: []string{"1m"}},
	}
	// capabilities is hoisted out of attrs; compare the rest
	if !reflect.DeepEqual(pp.attrs, want) {
		t.Errorf("attrs = %+v, want %+v", pp.attrs, want)
	}
}

func TestParsePolicyDocFallbacks(t *testing.T) {
	// empty document parses to no paths without error
	paths, err := parsePolicyDoc("")
	if err != nil || len(paths) != 0 {
		t.Errorf("empty doc: paths=%v err=%v", paths, err)
	}

	// non-path top-level statements must fail so callers show the raw
	_, err = parsePolicyDoc(`foo "bar" { x = 1 }`)
	if err == nil {
		t.Error("non-path statement should not parse")
	}

	// broken HCL fails
	_, err = parsePolicyDoc(`path "x" {`)
	if err == nil {
		t.Error("broken HCL should not parse")
	}
}

func TestSortCaps(t *testing.T) {
	got := sortCaps([]string{"deny", "list", "create", "sudo", "read", "zzz", "update", "delete", "patch"})
	want := []string{"create", "read", "update", "delete", "list", "patch", "sudo", "deny", "zzz"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sortCaps = %v, want %v", got, want)
	}
}
