package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"vaultui/internal/vault"
)

// formatMap renders a normalized map as aligned, sorted, indented
// "key: value" lines.
func formatMap(m map[string]interface{}) string {
	var b strings.Builder
	writeMap(&b, m, "")
	return strings.TrimRight(b.String(), "\n")
}

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
		fmt.Fprintf(b, "%s%s %s\n", keyStyle.Render(indent+k), dimStyle.Render(":"), styledValue(v))
	}
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

// formatPairs renders key/value pairs (e.g. engine tune data).
func formatPairs(pairs []vault.KV) string {
	var b strings.Builder
	for _, p := range pairs {
		fmt.Fprintf(&b, "%s%s %s\n", keyStyle.Render(p.K), dimStyle.Render(":"), valStyle.Render(p.V))
	}
	return strings.TrimRight(b.String(), "\n")
}
