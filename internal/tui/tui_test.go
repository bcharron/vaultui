package tui

import (
	"net"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	vaultapi "github.com/hashicorp/vault/api"
)

// testVault returns a client for a locally running dev Vault, or skips
// the test when none is reachable. Only loopback addresses are accepted
// so the test can never run against a shared server.
func testVault(t *testing.T) *vaultapi.Client {
	t.Helper()
	addr := os.Getenv("VAULT_ADDR")
	if addr == "" {
		addr = "http://127.0.0.1:8200"
	}
	u := addr
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		t.Skipf("refusing non-http VAULT_ADDR %q", addr)
	}
	host := strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		t.Skipf("refusing non-loopback vault at %q", addr)
	}
	resp, err := http.Get(addr + "/v1/sys/health")
	if err != nil {
		t.Skipf("no vault reachable at %s: %v", addr, err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 500 {
		t.Skipf("vault at %s unhealthy (status %d)", addr, resp.StatusCode)
	}

	cfg := &vaultapi.Config{Address: addr, HttpClient: &http.Client{}}
	cl, err := vaultapi.NewClient(cfg)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	token := os.Getenv("VAULT_TOKEN")
	if token == "" {
		token = "root"
	}
	cl.SetToken(token)
	return cl
}

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

// flatten runs a command and unfolds batch commands into individual
// messages.
func flatten(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if msg == nil {
		return nil
	}
	rv := reflect.ValueOf(msg)
	if rv.Kind() == reflect.Slice && rv.Type().Elem().Kind() == reflect.Func {
		var out []tea.Msg
		for i := 0; i < rv.Len(); i++ {
			if c, ok := rv.Index(i).Interface().(tea.Cmd); ok {
				out = append(out, flatten(c)...)
			}
		}
		return out
	}
	return []tea.Msg{msg}
}

// drive feeds a message to the model and pumps every produced command
// until the queue is empty, mimicking the bubbletea event loop.
func drive(t *testing.T, m tea.Model, msg tea.Msg) tea.Model {
	t.Helper()
	m, cmd := m.Update(msg)
	queue := flatten(cmd)
	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		m, cmd = m.Update(next)
		queue = append(queue, flatten(cmd)...)
	}
	return m
}

func typeKeys(t *testing.T, m tea.Model, keys ...string) tea.Model {
	t.Helper()
	for _, k := range keys {
		m = drive(t, m, keyMsg(k))
	}
	return m
}

// homeReset returns the model to the home page with the cursor at the
// top item, regardless of where it was left.
func homeReset(t *testing.T, m tea.Model) tea.Model {
	t.Helper()
	return typeKeys(t, m, "up", "up", "up", "up", "up", "up", "up")
}

func wantView(t *testing.T, m tea.Model, wants ...string) {
	t.Helper()
	view := m.View()
	for _, w := range wants {
		if !strings.Contains(view, w) {
			t.Errorf("view missing %q\n---\n%s\n---", w, view)
		}
	}
}

// openMenu resets the home cursor and opens the named menu item.
func openMenu(t *testing.T, m tea.Model, item string) tea.Model {
	t.Helper()
	items := []string{"ACL Policies", "Auth Methods", "Identity", "Leases", "Secret Engines", "Server Status", "Token Info"}
	idx := -1
	for i, s := range items {
		if s == item {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatalf("unknown menu item %q", item)
	}
	m = homeReset(t, m)
	for i := 0; i < idx; i++ {
		m = typeKeys(t, m, "down")
	}
	return typeKeys(t, m, "enter")
}

// toHome pops the navigation stack until the home page is on top.
func toHome(t *testing.T, m tea.Model) tea.Model {
	t.Helper()
	for i := 0; i < 15; i++ {
		first := strings.SplitN(m.View(), "\n", 2)[0]
		if strings.HasSuffix(first, "home") {
			return m
		}
		m = typeKeys(t, m, "esc")
	}
	return m
}

// openEngine returns to home, opens the engines list, and opens the
// given mount path. The cursor offset is computed from the live mount
// list so extra mounts (e.g. those enabled by setupCloudEngines) cannot
// shift navigation.
func openEngine(t *testing.T, m tea.Model, cl *vaultapi.Client, path string) tea.Model {
	t.Helper()
	m = toHome(t, m)
	m = openMenu(t, m, "Secret Engines")
	mounts, err := cl.Sys().ListMounts()
	if err != nil {
		t.Fatalf("list mounts: %v", err)
	}
	paths := make([]string, 0, len(mounts))
	for mp := range mounts {
		paths = append(paths, mp)
	}
	sort.Strings(paths)
	idx := -1
	for i, mp := range paths {
		if mp == path {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatalf("mount %s not found in %v", path, paths)
	}
	for i := 0; i < idx; i++ {
		m = typeKeys(t, m, "down")
	}
	return typeKeys(t, m, "enter")
}

func TestEndToEnd(t *testing.T) {
	cl := testVault(t)
	m := tea.Model(New(cl))
	m = drive(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})

	// Home -> Engines
	m = openMenu(t, m, "Secret Engines")
	wantView(t, m, "secret/", "legacy/", "transit/", "pki/", "cubbyhole/")

	// KV v2: secret/ -> detail (Config/Browse), browse into apps/api
	m = openEngine(t, m, cl, "secret/")
	wantView(t, m, "[Config]", "Browse")
	m = typeKeys(t, m, "right")
	wantView(t, m, "apps/", "db/")
	m = typeKeys(t, m, "enter") // into apps/
	wantView(t, m, "api", "web")
	m = typeKeys(t, m, "enter") // read secret apps/api
	wantView(t, m, "version: 2", "db_port: 5433", "notes: rotated port", "-- metadata --", "-- data --")
	m = typeKeys(t, m, "esc", "esc", "esc", "esc") // back to home
	wantView(t, m, "Secret Engines")

	// KV v1: legacy/
	m = openEngine(t, m, cl, "legacy/")
	wantView(t, m, "[Config]", "Browse")
	m = typeKeys(t, m, "right")
	wantView(t, m, "creds", "misc")
	m = typeKeys(t, m, "enter") // read legacy/creds
	wantView(t, m, "foo: bar", "baz: qux")
	m = typeKeys(t, m, "esc", "esc", "esc", "esc")

	// Transit keys
	m = openEngine(t, m, cl, "transit/")
	wantView(t, m, "[Config]", "Keys")
	m = typeKeys(t, m, "right")
	wantView(t, m, "aesguy", "eddy")
	m = typeKeys(t, m, "enter") // read key aesguy
	wantView(t, m, "aes256-gcm96")
	m = typeKeys(t, m, "esc", "esc", "esc")

	// PKI roles
	m = openEngine(t, m, cl, "pki/")
	wantView(t, m, "[Config]", "Roles")
	m = typeKeys(t, m, "right")
	wantView(t, m, "testrole")
	m = typeKeys(t, m, "enter")
	wantView(t, m, "test.local")
	m = typeKeys(t, m, "esc", "esc", "esc", "esc")

	// Policies
	m = openMenu(t, m, "ACL Policies")
	wantView(t, m, "default", "root", "team-apps", "jwt-signer", "jwt-reader")
	m = typeKeys(t, m, "down", "down", "down", "down", "enter") // team-apps
	wantView(t, m, "capabilities", "secret/data/apps/*")
	m = typeKeys(t, m, "esc", "esc")
	wantView(t, m, "Secret Engines")

	// Filter: "/" narrows the policy list live, esc clears it
	m = openMenu(t, m, "ACL Policies")
	m = typeKeys(t, m, "/", "j", "w", "t")
	wantView(t, m, "jwt-signer", "jwt-reader")
	if strings.Contains(m.View(), "team-apps") {
		t.Errorf("filtered policy list still shows non-matching team-apps\n---\n%s\n---", m.View())
	}
	m = typeKeys(t, m, "esc")
	wantView(t, m, "team-apps")
	m = typeKeys(t, m, "esc", "esc")

	// Reset the persistent home cursor, then auth methods: jwt roles
	m = openMenu(t, m, "Auth Methods")
	wantView(t, m, "jwt/", "token/")
	m = typeKeys(t, m, "enter") // jwt/ sorts before token/
	wantView(t, m, "[Config]", "Roles")
	m = typeKeys(t, m, "right")
	wantView(t, m, "demo-service", "service-reader", "-> jwt-signer, jwt-reader", "-> jwt-reader")
	m = typeKeys(t, m, "enter") // read role demo-service
	wantView(t, m, "linked policies:", "[1] jwt-signer", "[2] jwt-reader", "token_policies: jwt-signer, jwt-reader", "bound_audiences: demo-services")
	m = typeKeys(t, m, "1") // jump straight to the linked policy
	wantView(t, m, "policy: jwt-signer", "transit/sign/jwt-signer", "capabilities")
	m = typeKeys(t, m, "esc", "esc", "esc", "esc", "esc") // policy -> role -> auth detail -> auth list -> home
	wantView(t, m, "Secret Engines")

	// Token info + capabilities check
	m = openMenu(t, m, "Token Info")
	wantView(t, m, "display_name: token", "policies: root", "orphan: true")
	m = typeKeys(t, m, "secret/data/jwt/service-a")
	m = typeKeys(t, m, "enter")
	wantView(t, m, "capabilities: root", "path: secret/data/jwt/service-a")
	m = typeKeys(t, m, "esc") // clear input
	m = typeKeys(t, m, "esc") // back to home

	// Server status
	m = openMenu(t, m, "Server Status")
	wantView(t, m, "sealed: false", "storage_type: inmem", "standby: false")
	m = typeKeys(t, m, "esc")

	// KV v2 version history: apps/api has two versions
	m = openEngine(t, m, cl, "secret/")
	m = typeKeys(t, m, "right") // browse tab
	m = typeKeys(t, m, "enter") // into apps/
	m = typeKeys(t, m, "v")     // versions of api (cursor on api)
	wantView(t, m, "v2", "v1", "current")
	m = typeKeys(t, m, "down", "enter") // view version 1
	wantView(t, m, "version: 1", "db_host: db01.internal")
	m = typeKeys(t, m, "esc", "esc", "esc", "esc", "esc") // back to home

	// Leases: drill to the non-expiring root token lease
	m = openMenu(t, m, "Leases")
	wantView(t, m, "auth/")
	m = typeKeys(t, m, "enter") // into auth/
	wantView(t, m, "jwt/", "token/")
	m = typeKeys(t, m, "down", "enter") // into token/
	m = typeKeys(t, m, "enter")         // into create/
	m = typeKeys(t, m, "enter")         // open the leaf lease
	wantView(t, m, "renewable: false", "ttl: 0", "auth/token/create/")
	m = typeKeys(t, m, "esc", "esc", "esc", "esc", "esc", "esc")

	// Identity: entities and groups with policy links
	m = openMenu(t, m, "Identity")
	wantView(t, m, "[Entities]", "bob", "service-a")
	m = typeKeys(t, m, "enter") // bob
	wantView(t, m, "linked policies:", "[1] jwt-reader", "[2] jwt-signer", "email: bob@example.com")
	m = typeKeys(t, m, "esc")
	m = typeKeys(t, m, "right")
	wantView(t, m, "[Groups]", "apps-team")
	m = typeKeys(t, m, "enter")
	wantView(t, m, "linked policies:", "[1] jwt-signer", "member_entity_ids:")

	// AWS + GCP engines: dedicated tabs and inline role annotations
	setupCloudEngines(t, cl)

	// AWS: engine detail with Engine Config, Roles, Static Roles tabs
	m = openEngine(t, m, cl, "aws/")
	wantView(t, m, "[Config]", "Engine Config", "Roles", "Static Roles")
	m = typeKeys(t, m, "right") // Engine Config
	wantView(t, m, "aws/config/root", "aws/config/lease")
	m = typeKeys(t, m, "right") // Roles
	wantView(t, m, "deploy", "s3reader", "-> iam_user", "-> assumed_role")
	m = typeKeys(t, m, "enter") // role detail
	wantView(t, m, "credential_type: iam_user")
	m = typeKeys(t, m, "esc")
	m = typeKeys(t, m, "right") // Static Roles (none without live AWS creds)
	wantView(t, m, "no static roles found")
	m = typeKeys(t, m, "esc", "esc")

	// GCP: engine detail with roleset and account tabs
	m = openEngine(t, m, cl, "gcp/")
	wantView(t, m, "[Config]", "Engine Config", "Rolesets", "Static Accounts", "Impersonated Accounts")
	m = typeKeys(t, m, "right") // Engine Config
	wantView(t, m, "gcp/config")
	m = typeKeys(t, m, "right") // Rolesets (none without live GCP creds)
	wantView(t, m, "no rolesets found")
}

// setupCloudEngines enables the aws and gcp engines on the dev vault
// and writes AWS roles. AWS role definitions are pure configuration (no
// AWS calls), so they can be created without live credentials; AWS
// static roles and GCP rolesets validate against the cloud and stay
// empty here.
func setupCloudEngines(t *testing.T, cl *vaultapi.Client) {
	t.Helper()
	mounts, err := cl.Sys().ListMounts()
	if err != nil {
		t.Fatalf("list mounts: %v", err)
	}
	ensure := func(path, typ string) {
		if _, ok := mounts[path]; ok {
			return
		}
		if err := cl.Sys().Mount(path, &vaultapi.MountInput{Type: typ}); err != nil {
			t.Fatalf("enable %s: %v", path, err)
		}
	}
	ensure("aws/", "aws")
	ensure("gcp/", "gcp")
	write := func(path string, data map[string]interface{}) {
		if _, err := cl.Logical().Write(path, data); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	write("aws/roles/deploy", map[string]interface{}{"credential_type": "iam_user"})
	write("aws/roles/s3reader", map[string]interface{}{
		"credential_type": "assumed_role",
		"role_arns":       "arn:aws:iam::123456789012:role/s3reader",
	})
}
