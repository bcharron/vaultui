package tui

import (
	"fmt"
	"sort"
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
			fmt.Fprintf(b, "%s%s:\n", indent, k)
			writeMap(b, sub, indent+"  ")
			continue
		}
		fmt.Fprintf(b, "%s%s: %s\n", indent, k, formatValue(v))
	}
}

func formatValue(v interface{}) string {
	switch tv := v.(type) {
	case nil:
		return "null"
	case string:
		return tv
	case []interface{}:
		parts := make([]string, 0, len(tv))
		for _, e := range tv {
			parts = append(parts, formatValue(e))
		}
		return strings.Join(parts, ", ")
	default:
		return fmt.Sprint(v)
	}
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
		fmt.Fprintf(&b, "%s: %s\n", p.K, p.V)
	}
	return strings.TrimRight(b.String(), "\n")
}
