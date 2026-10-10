package tui

import (
	"strings"

	vaultapi "github.com/hashicorp/vault/api"

	"vaultui/internal/vault"
)

// newServerStatusPage builds a read-only server status document from
// sys/seal-status and sys/health.
func newServerStatusPage(cl *vaultapi.Client) Page {
	return newDetailPage(
		"server status",
		func() (string, []pageLink, error) {
			var b strings.Builder

			seal, sealErr := vault.SealStatus(cl)
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
			health, healthErr := vault.Health(cl)
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
