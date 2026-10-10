package vault

import (
	"errors"

	"github.com/hashicorp/vault/api"
)

// LookupSelf returns the authenticated token's own properties
// (policies, TTLs, display name, ...) as a normalized map.
func LookupSelf(c *api.Client) (map[string]interface{}, error) {
	cc, cancel := ctx()
	defer cancel()
	s, err := c.Auth().Token().LookupSelfWithContext(cc)
	if err != nil {
		return nil, err
	}
	if s == nil || s.Data == nil {
		return nil, errors.New("token lookup: empty response")
	}
	return normalize(s.Data)
}

// CapabilitiesSelf returns the capabilities the authenticated token
// holds on the given path (read, list, update, ...).
func CapabilitiesSelf(c *api.Client, path string) ([]string, error) {
	cc, cancel := ctx()
	defer cancel()
	return c.Sys().CapabilitiesSelfWithContext(cc, path)
}
