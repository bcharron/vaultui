package vault

import (
	"github.com/hashicorp/vault/api"
)

// ListEntities returns the names of all identity entities, sorted.
func ListEntities(c *api.Client) ([]string, error) {
	return listKeys(c, "identity/entity/name")
}

// ReadEntity returns one entity's properties (policies, aliases,
// metadata, group memberships, ...) by name.
func ReadEntity(c *api.Client, name string) (map[string]interface{}, error) {
	return readByName(c, "identity/entity/name/", name)
}

// ListGroups returns the names of all identity groups, sorted.
func ListGroups(c *api.Client) ([]string, error) {
	return listKeys(c, "identity/group/name")
}

// ReadGroup returns one group's properties (policies, members, ...)
// by name.
func ReadGroup(c *api.Client, name string) (map[string]interface{}, error) {
	return readByName(c, "identity/group/name/", name)
}

func readByName(c *api.Client, path, name string) (map[string]interface{}, error) {
	cc, cancel := ctx()
	defer cancel()
	s, err := c.Logical().ReadWithContext(cc, path+name)
	if err != nil {
		return nil, err
	}
	if s == nil || s.Data == nil {
		return nil, errEmpty(path + name)
	}
	return normalize(s.Data)
}
