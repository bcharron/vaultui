package vault

import (
	"sort"

	"github.com/hashicorp/vault/api"
)

// ListPolicies returns the names of all ACL policies, sorted.
func ListPolicies(c *api.Client) ([]string, error) {
	cc, cancel := ctx()
	defer cancel()
	names, err := c.Sys().ListPoliciesWithContext(cc)
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

// ReadPolicy returns the raw HCL/JSON rules of a single policy.
func ReadPolicy(c *api.Client, name string) (string, error) {
	cc, cancel := ctx()
	defer cancel()
	return c.Sys().GetPolicyWithContext(cc, name)
}
