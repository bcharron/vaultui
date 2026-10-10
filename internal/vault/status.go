package vault

import (
	"github.com/hashicorp/vault/api"
)

// SealStatus returns the sys/seal-status data (seal state, version,
// build date, cluster identity, storage type, ...) as a normalized map.
func SealStatus(c *api.Client) (map[string]interface{}, error) {
	cc, cancel := ctx()
	defer cancel()
	s, err := c.Logical().ReadWithContext(cc, "sys/seal-status")
	if err != nil {
		return nil, err
	}
	if s == nil || s.Data == nil {
		return nil, nil
	}
	return normalize(s.Data)
}

// Health returns the sys/health data. Note that Vault answers with a
// non-200 status (and thus an error here) when the node is sealed or a
// standby; callers should degrade gracefully.
func Health(c *api.Client) (map[string]interface{}, error) {
	cc, cancel := ctx()
	defer cancel()
	s, err := c.Logical().ReadWithContext(cc, "sys/health")
	if err != nil {
		return nil, err
	}
	if s == nil || s.Data == nil {
		return nil, nil
	}
	return normalize(s.Data)
}
