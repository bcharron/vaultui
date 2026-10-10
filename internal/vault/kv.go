package vault

import (
	"strings"

	"github.com/hashicorp/vault/api"
)

// KVEntry is one entry in a KV listing: a directory (ending in "/") or a
// secret.
type KVEntry struct {
	Name  string
	IsDir bool
}

// ListKV lists the entries under mount+sub. It understands both KV v1
// (LIST {mount}{sub}) and KV v2 (LIST {mount}metadata/{sub}).
func ListKV(c *api.Client, m Mount, sub string) ([]KVEntry, error) {
	path := m.Path + sub
	if m.KVVersion == 2 {
		path = m.Path + "metadata/" + sub
	}
	keys, err := listKeys(c, path)
	if err != nil {
		return nil, err
	}
	entries := make([]KVEntry, 0, len(keys))
	for _, k := range keys {
		entries = append(entries, KVEntry{Name: k, IsDir: strings.HasSuffix(k, "/")})
	}
	return entries, nil
}

// ReadKV reads a secret and returns its data along with engine-internal
// metadata (nil for KV v1).
func ReadKV(c *api.Client, m Mount, sub string) (data, meta map[string]interface{}, err error) {
	cc, cancel := ctx()
	defer cancel()
	path := m.Path + sub
	if m.KVVersion == 2 {
		path = m.Path + "data/" + sub
	}
	s, rerr := c.Logical().ReadWithContext(cc, path)
	if rerr != nil {
		return nil, nil, rerr
	}
	if s == nil || s.Data == nil {
		return nil, nil, errEmpty(strings.TrimSuffix(path, "/"))
	}
	if m.KVVersion == 2 {
		raw, _ := s.Data["data"].(map[string]interface{})
		rawMeta, _ := s.Data["metadata"].(map[string]interface{})
		data, err = normalize(raw)
		if err != nil {
			return nil, nil, err
		}
		meta, err = normalize(rawMeta)
		if err != nil {
			return nil, nil, err
		}
		return data, meta, nil
	}
	data, err = normalize(s.Data)
	if err != nil {
		return nil, nil, err
	}
	return data, nil, nil
}

type errEmpty string

func (e errEmpty) Error() string { return string(e) + ": secret not found or no read permission" }
