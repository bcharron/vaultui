package vault

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/vault/api"
)

func ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 15*time.Second)
}

// Mount describes a mounted secrets engine.
type Mount struct {
	Path        string // includes the trailing slash, e.g. "secret/"
	Type        string // e.g. "kv", "database", "transit", "pki"
	Description string
	Accessor    string
	Options     map[string]string
	Local       bool
	SealWrap    bool
	KVVersion   int // 1 or 2 for type "kv"; 0 otherwise
}

// ListMounts returns all secret engine mounts, sorted by path.
func ListMounts(c *api.Client) ([]Mount, error) {
	cc, cancel := ctx()
	defer cancel()
	raw, err := c.Sys().ListMountsWithContext(cc)
	if err != nil {
		return nil, err
	}
	mounts := make([]Mount, 0, len(raw))
	for path, m := range raw {
		mt := Mount{
			Path:        path,
			Type:        m.Type,
			Description: m.Description,
			Accessor:    m.Accessor,
			Options:     m.Options,
			Local:       m.Local,
			SealWrap:    m.SealWrap,
		}
		if m.Type == "kv" {
			mt.KVVersion = 1
			if v, ok := m.Options["version"]; ok && v == "2" {
				mt.KVVersion = 2
			}
		}
		mounts = append(mounts, mt)
	}
	sort.Slice(mounts, func(i, j int) bool { return mounts[i].Path < mounts[j].Path })
	return mounts, nil
}

// KV is a display key/value pair.
type KV struct {
	K string
	V string
}

// Tune returns the tune configuration (endpoint
// sys/mounts/{path}/tune) as sorted key/value pairs.
func Tune(c *api.Client, mountPath string) ([]KV, error) {
	cc, cancel := ctx()
	defer cancel()
	s, err := c.Logical().ReadWithContext(cc, "sys/mounts/"+mountPath+"tune")
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
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]KV, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, KV{K: k, V: formatTuneValue(k, data[k])})
	}
	return pairs, nil
}

func formatTuneValue(key string, v interface{}) string {
	if strings.HasSuffix(key, "_lease_ttl") {
		if f, ok := v.(float64); ok {
			if f > 0 {
				return time.Duration(f * float64(time.Second)).String()
			}
			return "0s (server default)"
		}
	}
	if m, ok := v.(map[string]interface{}); ok {
		sub := make([]string, 0, len(m))
		for sk, sv := range m {
			sub = append(sub, sk+"="+fmt.Sprint(sv))
		}
		sort.Strings(sub)
		return strings.Join(sub, " ")
	}
	return formatValue(v)
}

// normalize converts a map with mixed value types (map[string]string,
// map[string]interface{}, ...) into a plain map[string]interface{} so
// rendering code has a single shape to deal with.
func normalize(m map[string]interface{}) (map[string]interface{}, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	var out map[string]interface{}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// formatValue renders a decoded JSON value for display.
func formatValue(v interface{}) string {
	switch tv := v.(type) {
	case nil:
		return "null"
	case string:
		return tv
	case []interface{}:
		parts := make([]string, 0, len(tv))
		for _, e := range tv {
			parts = append(parts, formatValue(e))
		}
		return strings.Join(parts, ", ")
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(b)
	}
}

func listKeys(c *api.Client, path string) ([]string, error) {
	cc, cancel := ctx()
	defer cancel()
	s, err := c.Logical().ListWithContext(cc, path)
	if err != nil {
		return nil, err
	}
	if s == nil || s.Data == nil {
		return nil, nil
	}
	raw, _ := s.Data["keys"].([]interface{})
	out := make([]string, 0, len(raw))
	for _, k := range raw {
		if ks, ok := k.(string); ok {
			out = append(out, ks)
		}
	}
	sort.Strings(out)
	return out, nil
}
