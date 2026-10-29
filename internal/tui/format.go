package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/bcharron/vaultui/internal/vault"
)

// formatMap renders a normalized map as aligned, sorted, indented
// "key: value" lines.
func formatMap(m map[string]interface{}) string {
	var b strings.Builder
	writeMap(&b, m, "")
	return strings.TrimRight(b.String(), "\n")
}

// writeMap renders the map's sorted entries as "key: value" lines.
// Nested maps and arrays of maps (e.g. identity aliases) become
// indented sections instead of Go's default map printing.
func writeMap(b *strings.Builder, m map[string]interface{}, indent string) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := m[k]
		if sub, ok := v.(map[string]interface{}); ok {
			fmt.Fprintf(b, "%s%s\n", keyStyle.Render(indent+k), dimStyle.Render(":"))
			writeMap(b, sub, indent+"  ")
			continue
		}
		if arr, ok := v.([]interface{}); ok && hasMapElem(arr) {
			fmt.Fprintf(b, "%s%s\n", keyStyle.Render(indent+k), dimStyle.Render(":"))
			for i, e := range arr {
				if i > 0 {
					b.WriteString("\n")
				}
				if em, ok := e.(map[string]interface{}); ok {
					writeMap(b, em, indent+"  ")
					continue
				}
				fmt.Fprintf(b, "%s%s\n", indent+"  ", styledValue(e))
			}
			continue
		}
		val := styledValue(v)
		if f, ok := v.(float64); ok {
			val += ttlSuffix(k, strconv.FormatFloat(f, 'f', -1, 64))
		}
		fmt.Fprintf(b, "%s%s %s\n", keyStyle.Render(indent+k), dimStyle.Render(":"), val)
	}
}

// hasMapElem reports whether the array contains at least one map,
// making it render as a nested section rather than an inline list.
func hasMapElem(arr []interface{}) bool {
	for _, e := range arr {
		if _, ok := e.(map[string]interface{}); ok {
			return true
		}
	}
	return false
}

// styledValue renders a decoded JSON value with type-based colors:
// booleans green/red, numbers amber, strings bright, null dimmed.
func styledValue(v interface{}) string {
	switch tv := v.(type) {
	case nil:
		return dimStyle.Render("null")
	case bool:
		if tv {
			return boolTrueStyle.Render("true")
		}
		return boolFalseStyle.Render("false")
	case float64:
		return numStyle.Render(strconv.FormatFloat(tv, 'f', -1, 64))
	case string:
		return valStyle.Render(tv)
	case []interface{}:
		parts := make([]string, 0, len(tv))
		for _, e := range tv {
			parts = append(parts, styledValue(e))
		}
		return strings.Join(parts, ", ")
	default:
		return valStyle.Render(fmt.Sprint(v))
	}
}

// extractPolicies pulls policy names out of a role's data, covering the
// key variations used across Vault auth methods (token_policies,
// policies).
func extractPolicies(m map[string]interface{}) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, key := range []string{"token_policies", "policies"} {
		switch v := m[key].(type) {
		case []interface{}:
			for _, e := range v {
				if s, ok := e.(string); ok {
					add(s)
				}
			}
		case string:
			for _, s := range strings.Split(v, ",") {
				add(s)
			}
		}
	}
	return out
}

// roleAnnotation returns the short hint shown next to role names in a
// role group tab: the AWS credential type, the GCP roleset secret type.
func roleAnnotation(engineType string, m map[string]interface{}) string {
	str := func(k string) string {
		if s, ok := m[k].(string); ok {
			return s
		}
		return ""
	}
	switch engineType {
	case "aws":
		return str("credential_type")
	case "gcp":
		return str("secret_type")
	}
	return ""
}

// secretDoc renders a KV secret's metadata (v2) and data sections.
func secretDoc(data, meta map[string]interface{}) string {
	var b strings.Builder
	if meta != nil {
		b.WriteString(dimStyle.Render("-- metadata --"))
		b.WriteString("\n" + formatMap(meta) + "\n\n")
		b.WriteString(dimStyle.Render("-- data --"))
		b.WriteString("\n")
	}
	b.WriteString(formatMap(data))
	return b.String()
}

// ttlSuffix returns the dimmed " (32d)" annotation appended to
// ttl-like numeric values so raw seconds stay readable. Zero and
// negative values get no annotation.
func ttlSuffix(key, raw string) string {
	if !isDurationKey(key) {
		return ""
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n <= 0 {
		return ""
	}
	return dimStyle.Render(" (" + humanDuration(n) + ")")
}

// isDurationKey reports whether a map key holds a duration in seconds
// (ttls, periods, lease durations).
func isDurationKey(k string) bool {
	lk := strings.ToLower(k)
	return strings.Contains(lk, "ttl") || lk == "period" || lk == "lease_duration"
}

// humanDuration renders a number of seconds in the largest two units:
// "32d", "1d4h", "1h30m", "45s", "0s".
func humanDuration(sec int) string {
	if sec <= 0 {
		return "0s"
	}
	switch {
	case sec >= 86400:
		if h := (sec % 86400) / 3600; h > 0 {
			return fmt.Sprintf("%dd%dh", sec/86400, h)
		}
		return fmt.Sprintf("%dd", sec/86400)
	case sec >= 3600:
		if m := (sec % 3600) / 60; m > 0 {
			return fmt.Sprintf("%dh%dm", sec/3600, m)
		}
		return fmt.Sprintf("%dh", sec/3600)
	case sec >= 60:
		if s := sec % 60; s > 0 {
			return fmt.Sprintf("%dm%ds", sec/60, s)
		}
		return fmt.Sprintf("%dm", sec/60)
	default:
		return fmt.Sprintf("%ds", sec)
	}
}

// formatPairs renders key/value pairs (e.g. engine tune data).
func formatPairs(pairs []vault.KV) string {
	var b strings.Builder
	for _, p := range pairs {
		fmt.Fprintf(&b, "%s%s %s\n", keyStyle.Render(p.K), dimStyle.Render(":"), valStyle.Render(p.V)+ttlSuffix(p.K, p.V))
	}
	return strings.TrimRight(b.String(), "\n")
}
