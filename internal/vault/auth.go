package vault

import (
	"sort"

	"github.com/hashicorp/vault/api"
)

// ListAuthMounts returns all enabled auth methods (jwt, oidc, approle,
// userpass, ...), sorted by path. It reuses the Mount shape since auth
// mounts expose the same fields (KVVersion is left 0).
func ListAuthMounts(c *api.Client) ([]Mount, error) {
	cc, cancel := ctx()
	defer cancel()
	raw, err := c.Sys().ListAuthWithContext(cc)
	if err != nil {
		return nil, err
	}
	out := make([]Mount, 0, len(raw))
	for path, m := range raw {
		out = append(out, Mount{
			Path:        path,
			Type:        m.Type,
			Description: m.Description,
			Accessor:    m.Accessor,
			Options:     m.Options,
			Local:       m.Local,
			SealWrap:    m.SealWrap,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// AuthTune returns the tune configuration of an auth method
// (endpoint sys/auth/{path}/tune) as sorted key/value pairs.
func AuthTune(c *api.Client, authPath string) ([]KV, error) {
	cc, cancel := ctx()
	defer cancel()
	s, err := c.Logical().ReadWithContext(cc, "sys/auth/"+authPath+"tune")
	if err != nil {
		return nil, err
	}
	if s == nil || s.Data == nil {
		return nil, nil
	}
	data, err := normalize(s.Data)
	if err != nil {
		return nil, err
	}
	return tunePairs(data), nil
}
