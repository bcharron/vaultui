package tui

import (
	"strings"

	vaultapi "github.com/hashicorp/vault/api"

	"github.com/bcharron/vaultui/internal/vault"
)

// newServerStatusPage builds a server status document from
// sys/seal-status and sys/health: a curated headline first, the raw
// data below.
func newServerStatusPage(cl *vaultapi.Client) Page {
	return newDetailPage(
		"server status",
		func() (string, []pageLink, error) {
			var b strings.Builder

			seal, sealErr := vault.SealStatus(cl)
			health, healthErr := vault.Health(cl)

			b.WriteString(statusSummary(seal, health))
			b.WriteString("\n\n")

			b.WriteString(dimStyle.Render("-- seal status --"))
			b.WriteString("\n")
			if sealErr != nil {
				b.WriteString(errStyle.Render("error: " + sealErr.Error()))
			} else {
				b.WriteString(formatMap(seal))
			}

			b.WriteString("\n\n")
			b.WriteString(dimStyle.Render("-- health --"))
			b.WriteString("\n")
			if healthErr != nil {
				// Vault answers non-200 (as an error) when sealed or a
				// standby; still informative to show.
				b.WriteString(errStyle.Render("error: " + healthErr.Error()))
			} else {
				b.WriteString(formatMap(health))
			}
			return b.String(), nil, nil
		},
	)
}

// statusSummary renders the one-line headline: seal state, version,
// storage type, standby state and cluster name.
func statusSummary(seal, health map[string]interface{}) string {
	var parts []string
	if sealed, ok := seal["sealed"].(bool); ok {
		if sealed {
			parts = append(parts, errStyle.Render("SEALED"))
		} else {
			parts = append(parts, typeStyle.Render("UNSEALED"))
		}
	}
	if v, ok := seal["version"].(string); ok && v != "" {
		parts = append(parts, dimStyle.Render("v"+v))
	}
	if t, ok := seal["storage_type"].(string); ok && t != "" {
		parts = append(parts, dimStyle.Render(t))
	}
	if sb, ok := health["standby"].(bool); ok {
		if sb {
			parts = append(parts, errStyle.Render("standby"))
		} else {
			parts = append(parts, dimStyle.Render("active"))
		}
	}
	if cn, ok := health["cluster_name"].(string); ok && cn != "" {
		parts = append(parts, dimStyle.Render("cluster: "+cn))
	}
	return strings.Join(parts, dimStyle.Render(" · "))
}
