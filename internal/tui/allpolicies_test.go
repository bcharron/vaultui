package tui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	vaultapi "github.com/hashicorp/vault/api"
)

// fakePoliciesVault serves a minimal policies API: "good" is
// readable, "denied" answers 403 the way Vault does for a policy the
// token may not read.
func fakePoliciesVault(t *testing.T) *vaultapi.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/sys/policies/acl", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"keys": []string{"denied", "good"}},
		})
	})
	mux.HandleFunc("/v1/sys/policies/acl/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/v1/sys/policies/acl/")
		if name == "denied" {
			w.WriteHeader(http.StatusForbidden)
			// the real dev vault wraps denials in a multierror; the
			// row must still render as a single line
			json.NewEncoder(w).Encode(map[string]any{
				"errors": []string{"1 error occurred:\n\n* permission denied"},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"policy": "path \"secret/data/apps/*\" {\n  capabilities = [\"read\"]\n}\n",
			},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	cl, err := vaultapi.NewClient(&vaultapi.Config{Address: srv.URL})
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	cl.SetToken("root")
	return cl
}

// loadAllPolicies runs the page's fetch command and feeds the result
// back through Update, the way the App does.
func loadAllPolicies(p *allPoliciesPage) *allPoliciesPage {
	p.Update(resizeMsg{w: 120, h: 30})
	page, _ := p.Update(p.Init()())
	return page.(*allPoliciesPage)
}

func TestAllPoliciesSkipsDeniedPolicy(t *testing.T) {
	p := loadAllPolicies(newAllPoliciesPage(fakePoliciesVault(t), []string{"denied", "good"}))
	v := p.View()
	if p.Loading() {
		t.Fatalf("page still loading")
	}
	if !strings.Contains(v, "secret/data/apps/*") {
		t.Errorf("readable policy's path missing from the union\n---\n%s\n---", v)
	}
	// the denied policy is a red row at the end of the list, not a
	// note line wrapping over the top pane
	if !strings.Contains(v, "(unreadable) denied: permission denied") {
		t.Errorf("denied policy not shown as an error row\n---\n%s\n---", v)
	}
	// the multierror must not leak its newlines into the row layout
	if strings.Contains(v, "1 error occurred") || strings.Contains(v, "\n* ") {
		t.Errorf("error row spans multiple lines\n---\n%s\n---", v)
	}
	if strings.Contains(v, "unreadable, not shown") {
		t.Errorf("note line still piles up above the list\n---\n%s\n---", v)
	}
	if strings.Contains(v, "error:") {
		t.Errorf("one denied policy turned the whole union into an error\n---\n%s\n---", v)
	}
	// cursor on the denied row: the bottom pane shows the error with
	// the request details, not just the same one-line reason
	p.Update(keyMsg("down")) // rows: good's path, then denied
	v = p.View()
	for _, want := range []string{
		"(unreadable) denied: permission denied", // the row stays condensed
		"denied: permission denied",
		"request: GET sys/policies/acl/denied",
		"status: 403",
		"errors:",
		"1 error occurred:",
		"* permission denied",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q\n---\n%s\n---", want, v)
		}
	}
}

func TestAllPoliciesAllDeniedStillShowsRows(t *testing.T) {
	p := loadAllPolicies(newAllPoliciesPage(fakePoliciesVault(t), []string{"denied"}))
	v := p.View()
	if !strings.Contains(v, "(unreadable) denied: permission denied") {
		t.Errorf("denied policy not shown as an error row\n---\n%s\n---", v)
	}
	if strings.Contains(v, "error:") {
		t.Errorf("page shows a full-page error instead of the row\n---\n%s\n---", v)
	}
}
