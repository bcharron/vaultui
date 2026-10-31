package vault

import (
	"fmt"

	"github.com/hashicorp/vault/api"
)

// EngineConfigEndpoints returns the config sub-paths (relative to the
// mount path) where an engine keeps its own configuration, beyond the
// sys/mounts tune data. Empty for engines without dedicated config
// endpoints.
func EngineConfigEndpoints(engineType string) []string {
	switch engineType {
	case "aws":
		return []string{"config/root", "config/lease"}
	case "gcp", "azure", "kubernetes", "openldap":
		return []string{"config"}
	case "consul", "nomad":
		return []string{"config/access", "config/lease"}
	case "rabbitmq":
		return []string{"config/connection", "config/lease"}
	case "pki":
		return []string{"config/urls", "config/cluster"}
	}
	return nil
}

// AuthConfigEndpoints returns the config sub-paths (relative to the
// auth mount path) where an auth method keeps its own configuration,
// beyond the sys/auth tune data (e.g. auth/jwt/config with
// bound_issuer, jwks_url, ...). Empty for auth methods without
// dedicated config endpoints.
func AuthConfigEndpoints(authType string) []string {
	switch authType {
	case "jwt", "oidc", "kubernetes", "azure", "gcp", "oci", "saml", "ldap", "okta", "cf", "radius":
		return []string{"config"}
	case "kerberos":
		return []string{"config", "config/ldap"}
	case "cert":
		return []string{"config", "crl/config"}
	case "aws":
		return []string{"config/root"}
	}
	return nil
}

// ReadEngineConfig reads one config sub-path under a mount and returns
// it as a normalized, display-ready map. A nil map means the endpoint
// returned no data (common when it has not been configured yet).
func ReadEngineConfig(c *api.Client, path string) (map[string]interface{}, error) {
	cc, cancel := ctx()
	defer cancel()
	s, err := c.Logical().ReadWithContext(cc, path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if s == nil || s.Data == nil {
		return nil, nil
	}
	return normalize(s.Data)
}
