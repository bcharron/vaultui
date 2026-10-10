package vault

import (
	"fmt"
	"strings"

	"github.com/hashicorp/vault/api"
)

// ListLeases lists the entries under a prefix of the sys/leases/lookup
// tree. Entries ending in "/" are prefixes; other entries are the final
// component of concrete lease IDs (full ID = prefix + entry).
func ListLeases(c *api.Client, prefix string) ([]KVEntry, error) {
	keys, err := listKeys(c, "sys/leases/lookup/"+prefix)
	if err != nil {
		return nil, err
	}
	entries := make([]KVEntry, 0, len(keys))
	for _, k := range keys {
		entries = append(entries, KVEntry{Name: k, IsDir: strings.HasSuffix(k, "/")})
	}
	return entries, nil
}

// LookupLease returns a lease's properties (issue/expire times,
// renewable flag, ttl). The endpoint is a query (POST sys/leases/lookup)
// and does not modify any state.
func LookupLease(c *api.Client, leaseID string) (map[string]interface{}, error) {
	cc, cancel := ctx()
	defer cancel()
	s, err := c.Logical().WriteWithContext(cc, "sys/leases/lookup",
		map[string]interface{}{"lease_id": leaseID})
	if err != nil {
		return nil, err
	}
	if s == nil || s.Data == nil {
		return nil, fmt.Errorf("lease %s not found", leaseID)
	}
	return normalize(s.Data)
}
