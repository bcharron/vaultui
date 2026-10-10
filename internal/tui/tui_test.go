package tui

import (
	"net"
	"net/http"
	"os"
	"reflect"
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

func TestEndToEnd(t *testing.T) {
	cl := testVault(t)
	m := tea.Model(New(cl))
	m = drive(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})

	// Home -> Engines
	m = typeKeys(t, m, "enter")
	wantView(t, m, "secret/", "legacy/", "transit/", "pki/", "cubbyhole/")

	// Engines sorted: cubbyhole(0) identity(1) legacy(2) pki(3) secret(4) sys(5) transit(6)

	// KV v2: secret/ -> detail (Config/Browse), browse into apps/api
	m = typeKeys(t, m, "down", "down", "down", "down", "enter")
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
	m = typeKeys(t, m, "enter", "down", "down", "enter")
	wantView(t, m, "[Config]", "Browse")
	m = typeKeys(t, m, "right")
	wantView(t, m, "creds", "misc")
	m = typeKeys(t, m, "enter") // read legacy/creds
	wantView(t, m, "foo: bar", "baz: qux")
	m = typeKeys(t, m, "esc", "esc", "esc", "esc")

	// Transit keys
	m = typeKeys(t, m, "enter", "down", "down", "down", "down", "down", "down", "enter")
	wantView(t, m, "[Config]", "Keys")
	m = typeKeys(t, m, "right")
	wantView(t, m, "aesguy", "eddy")
	m = typeKeys(t, m, "enter") // read key aesguy
	wantView(t, m, "aes256-gcm96")
	m = typeKeys(t, m, "esc", "esc", "esc")

	// PKI roles
	m = typeKeys(t, m, "enter", "down", "down", "down", "enter")
	wantView(t, m, "[Config]", "Roles")
	m = typeKeys(t, m, "right")
	wantView(t, m, "testrole")
	m = typeKeys(t, m, "enter")
	wantView(t, m, "test.local")
	m = typeKeys(t, m, "esc", "esc", "esc", "esc")

	// Policies
	m = typeKeys(t, m, "down", "enter")
	wantView(t, m, "default", "root", "team-apps", "jwt-signer", "jwt-reader")
	m = typeKeys(t, m, "down", "down", "down", "down", "enter") // team-apps
	wantView(t, m, "capabilities", "secret/data/apps/*")
	m = typeKeys(t, m, "esc", "esc")
	wantView(t, m, "Secret Engines")

	// Reset the persistent home cursor, then auth methods: jwt roles
	m = homeReset(t, m)
	m = typeKeys(t, m, "down", "down", "enter")
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
	m = homeReset(t, m)
	m = typeKeys(t, m, "down", "down", "down", "enter")
	wantView(t, m, "display_name: token", "policies: root", "orphan: true")
	m = typeKeys(t, m, "secret/data/jwt/service-a")
	m = typeKeys(t, m, "enter")
	wantView(t, m, "capabilities: root", "path: secret/data/jwt/service-a")
	m = typeKeys(t, m, "esc") // clear input
	m = typeKeys(t, m, "esc") // back to home

	// Server status
	m = homeReset(t, m)
	m = typeKeys(t, m, "down", "down", "down", "down", "enter")
	wantView(t, m, "sealed: false", "storage_type: inmem", "standby: false")
	m = typeKeys(t, m, "esc")

	// KV v2 version history: apps/api has two versions
	m = homeReset(t, m)
	m = typeKeys(t, m, "enter", "down", "down", "down", "down", "enter") // secret/ engine detail
	m = typeKeys(t, m, "right")                                          // browse tab
	m = typeKeys(t, m, "enter")                                          // into apps/
	m = typeKeys(t, m, "v")                                              // versions of api (cursor on api)
	wantView(t, m, "v2", "v1", "current")
	m = typeKeys(t, m, "down", "enter") // view version 1
	wantView(t, m, "version: 1", "db_host: db01.internal")
	m = typeKeys(t, m, "esc", "esc", "esc", "esc", "esc") // back to home

	// Leases: drill to the non-expiring root token lease
	m = homeReset(t, m)
	m = typeKeys(t, m, "down", "down", "down", "down", "down", "enter")
	wantView(t, m, "auth/")
	m = typeKeys(t, m, "enter") // into auth/
	wantView(t, m, "jwt/", "token/")
	m = typeKeys(t, m, "down", "enter") // into token/
	m = typeKeys(t, m, "enter")         // into create/
	m = typeKeys(t, m, "enter")         // open the leaf lease
	wantView(t, m, "renewable: false", "ttl: 0", "auth/token/create/")
	m = typeKeys(t, m, "esc", "esc", "esc", "esc", "esc", "esc")

	// Identity: entities and groups with policy links
	m = homeReset(t, m)
	m = typeKeys(t, m, "down", "down", "down", "down", "down", "down", "enter")
	wantView(t, m, "[Entities]", "bob", "service-a")
	m = typeKeys(t, m, "enter") // bob
	wantView(t, m, "linked policies:", "[1] jwt-reader", "[2] jwt-signer", "email: bob@example.com")
	m = typeKeys(t, m, "esc")
	m = typeKeys(t, m, "right")
	wantView(t, m, "[Groups]", "apps-team")
	m = typeKeys(t, m, "enter")
	wantView(t, m, "linked policies:", "[1] jwt-signer", "member_entity_ids:")
}
