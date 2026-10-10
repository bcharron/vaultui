package vault

import (
	"errors"
	"fmt"

	"github.com/hashicorp/vault/api"
)

// roleEndpoints maps an engine type to the path segment under the mount
// where its roles (or keys, for transit) live. Unknown types fall back to
// "roles", which is the convention for most credential-style engines.
var roleEndpoints = map[string]string{
	"database":      "roles",
	"pki":           "roles",
	"aws":           "roles",
	"gcp":           "roles",
	"azure":         "roles",
	"consul":        "roles",
	"nomad":         "roles",
	"rabbitmq":      "roles",
	"ssh":           "roles",
	"cassandra":     "roles",
	"elasticsearch": "roles",
	"kubernetes":    "roles",
	"mongodb":       "roles",
	"mongodb-atlas": "roles",
	"mssql":         "roles",
	"mysql":         "roles",
	"openldap":      "roles",
	"postgresql":    "roles",
	"redshift":      "roles",
	"snowflake":     "roles",
	"transit":       "keys",
}

// authRoleEndpoints maps an auth method type to the path segment under
// the mount where its roles/users/groups live. Unknown types fall back
// to "role".
var authRoleEndpoints = map[string]string{
	"jwt":        "role",
	"oidc":       "role",
	"approle":    "role",
	"kubernetes": "role",
	"saml":       "role",
	"azuread":    "role",
	"gcp":        "role",
	"userpass":   "users",
	"okta":       "users",
	"github":     "teams",
	"ldap":       "groups",
}

// AuthRoleListEndpoint returns the sub-path where roles/users/groups are
// listed for the given auth method type.
func AuthRoleListEndpoint(authType string) string {
	if ep, ok := authRoleEndpoints[authType]; ok {
		return ep
	}
	return "role"
}

// RoleListEndpoint returns the sub-path where roles/keys are listed for
// the given secret engine type.
func RoleListEndpoint(engineType string) string {
	if ep, ok := roleEndpoints[engineType]; ok {
		return ep
	}
	return "roles"
}

// ListRoles lists role (or key, or user) names under a mount at the given
// endpoint. It returns an error if the endpoint has no listable entries
// or access is denied.
func ListRoles(c *api.Client, mountPath, endpoint string) ([]string, error) {
	if mountPath == "" {
		return nil, errors.New("no mount")
	}
	return listKeys(c, mountPath+endpoint)
}

// ReadRole reads a single role's (or key's) configuration and returns it
// as a normalized, display-ready map.
func ReadRole(c *api.Client, mountPath, endpoint, name string) (map[string]interface{}, error) {
	cc, cancel := ctx()
	defer cancel()
	path := mountPath + endpoint + "/" + name
	s, err := c.Logical().ReadWithContext(cc, path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if s == nil || s.Data == nil {
		return nil, fmt.Errorf("%s: empty response", path)
	}
	return normalize(s.Data)
}
