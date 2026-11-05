package vault

import (
	"sort"

	"github.com/hashicorp/vault/api"
)

// GroupAlias is one identity group alias: the mapping of a group name
// from an auth method (canonical_id) to an identity group. Data keeps
// the full alias fields for display.
type GroupAlias struct {
	ID   string
	Name string
	Data map[string]interface{}
}

// ListGroupAliasesByAccessor returns the identity group aliases
// assigned to the auth method with the given mount accessor, sorted by
// alias name. Vault has no list-by-accessor endpoint, so every alias
// is listed and read, keeping the ones on this accessor.
func ListGroupAliasesByAccessor(c *api.Client, accessor string) ([]GroupAlias, error) {
	ids, err := listKeys(c, "identity/group-alias/id")
	if err != nil {
		return nil, err
	}
	out := []GroupAlias{}
	for _, id := range ids {
		cc, cancel := ctx()
		s, rerr := c.Logical().ReadWithContext(cc, "identity/group-alias/id/"+id)
		cancel()
		if rerr != nil {
			return nil, rerr
		}
		if s == nil || s.Data == nil {
			continue
		}
		data, rerr := normalize(s.Data)
		if rerr != nil {
			return nil, rerr
		}
		if ma, _ := data["mount_accessor"].(string); ma != accessor {
			continue
		}
		name, _ := data["name"].(string)
		out = append(out, GroupAlias{ID: id, Name: name, Data: data})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

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
