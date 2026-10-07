package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
)

const baseSchema = `{
  "type": "object",
  "required": ["costCentre"],
  "properties": {
    "costCentre": {"type": "string", "pattern": "^CC-[0-9]{4}$"},
    "storageGi":  {"type": "integer", "minimum": 10, "maximum": 1000}
  },
  "additionalProperties": false
}`

func newTestServer(t *testing.T, objs ...runtime.Object) *httptest.Server {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := treV1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	tmpl := &treV1.WorkspaceTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "base"},
		Spec: treV1.WorkspaceTemplateSpec{
			DisplayName: "Base", Version: "0.1.0",
			ParametersSchema: &apiextensionsv1.JSON{Raw: []byte(baseSchema)},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithRuntimeObjects(append(objs, tmpl)...).
		WithStatusSubresource(&treV1.Workspace{}).Build()
	srv := httptest.NewServer((&Server{Client: c, Auth: DevAuthenticator{}}).Handler())
	t.Cleanup(srv.Close)
	return srv
}

func call(t *testing.T, srv *httptest.Server, method, path, user, roles, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	if user != "" {
		req.Header.Set("X-Dev-User", user)
		req.Header.Set("X-Dev-Roles", roles)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func existing(name string, owners, researchers []string) *treV1.Workspace {
	return &treV1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       treV1.WorkspaceSpec{DisplayName: name, TemplateRef: "base", Owners: owners, Researchers: researchers},
	}
}

const validBody = `{"name":"genomics","displayName":"Genomics","templateRef":"base",
  "parameters":{"costCentre":"CC-1234","storageGi":100},
  "researchers":["rita@example.com"],"allowedFQDNs":["pypi.org","*.r-project.org"]}`

func TestUnauthenticatedIsRejected(t *testing.T) {
	srv := newTestServer(t)
	if code, _ := call(t, srv, "GET", "/api/v1/workspaces", "", "", ""); code != http.StatusUnauthorized {
		t.Fatalf("code = %d", code)
	}
}

func TestAdminCreatesWorkspace(t *testing.T) {
	srv := newTestServer(t)
	code, body := call(t, srv, "POST", "/api/v1/workspaces", "admin@example.com", "TREAdmin", validBody)
	if code != http.StatusAccepted {
		t.Fatalf("code = %d body = %v", code, body)
	}
	if body["phase"] != "Pending" {
		t.Fatalf("phase = %v", body["phase"])
	}
	// The creating admin becomes owner when none are given.
	if owners := body["owners"].([]any); len(owners) != 1 || owners[0] != "admin@example.com" {
		t.Fatalf("owners = %v", owners)
	}
	if code, _ := call(t, srv, "POST", "/api/v1/workspaces", "admin@example.com", "TREAdmin", validBody); code != http.StatusConflict {
		t.Fatalf("duplicate create code = %d", code)
	}
}

func TestNonAdminCannotCreateOrDelete(t *testing.T) {
	srv := newTestServer(t, existing("ws1", []string{"olive@example.com"}, nil))
	if code, _ := call(t, srv, "POST", "/api/v1/workspaces", "olive@example.com", "TREUser", validBody); code != http.StatusForbidden {
		t.Fatalf("create code = %d", code)
	}
	if code, _ := call(t, srv, "DELETE", "/api/v1/workspaces/ws1", "olive@example.com", "TREUser", ""); code != http.StatusForbidden {
		t.Fatalf("delete code = %d", code)
	}
}

func TestParametersAreValidatedAgainstTemplateSchema(t *testing.T) {
	srv := newTestServer(t)
	cases := map[string]string{
		"missing required": `{"name":"a1","displayName":"A","templateRef":"base","parameters":{}}`,
		"bad pattern":      `{"name":"a2","displayName":"A","templateRef":"base","parameters":{"costCentre":"x"}}`,
		"out of range":     `{"name":"a3","displayName":"A","templateRef":"base","parameters":{"costCentre":"CC-1234","storageGi":5}}`,
		"unknown property": `{"name":"a4","displayName":"A","templateRef":"base","parameters":{"costCentre":"CC-1234","gpu":true}}`,
		"bad name":         `{"name":"Not_Valid","displayName":"A","templateRef":"base","parameters":{"costCentre":"CC-1234"}}`,
		"bad fqdn":         `{"name":"a5","displayName":"A","templateRef":"base","parameters":{"costCentre":"CC-1234"},"allowedFQDNs":["not a host"]}`,
		"unknown template": `{"name":"a6","displayName":"A","templateRef":"nope","parameters":{}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if code, resp := call(t, srv, "POST", "/api/v1/workspaces", "admin@example.com", "TREAdmin", body); code != http.StatusUnprocessableEntity {
				t.Fatalf("code = %d body = %v", code, resp)
			}
		})
	}
}

func TestSchemaCannotFetchRemoteReferences(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = treV1.AddToScheme(scheme)
	tmpl := &treV1.WorkspaceTemplate{ObjectMeta: metav1.ObjectMeta{Name: "evil"}, Spec: treV1.WorkspaceTemplateSpec{
		ParametersSchema: &apiextensionsv1.JSON{Raw: []byte(`{"$ref":"file:///etc/passwd"}`)},
	}}
	if err := validateParameters(tmpl, json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected file reference to be refused")
	}
}

func TestMembersSeeOnlyTheirWorkspaces(t *testing.T) {
	srv := newTestServer(t,
		existing("ws-a", []string{"olive@example.com"}, []string{"Rita@Example.com"}),
		existing("ws-b", []string{"other@example.com"}, nil),
	)

	_, body := call(t, srv, "GET", "/api/v1/workspaces", "rita@example.com", "TREUser", "")
	list := body["workspaces"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["name"] != "ws-a" {
		t.Fatalf("researcher sees %v", list)
	}
	if roles := list[0].(map[string]any)["yourRoles"].([]any); len(roles) != 1 || roles[0] != RoleResearcher {
		t.Fatalf("roles = %v", roles)
	}

	_, body = call(t, srv, "GET", "/api/v1/workspaces", "admin@example.com", "TREAdmin", "")
	if n := len(body["workspaces"].([]any)); n != 2 {
		t.Fatalf("admin sees %d workspaces", n)
	}

	// Non-members get 404, not 403, so workspace names do not leak.
	if code, _ := call(t, srv, "GET", "/api/v1/workspaces/ws-b", "rita@example.com", "TREUser", ""); code != http.StatusNotFound {
		t.Fatalf("non-member get code = %d", code)
	}
	if code, _ := call(t, srv, "GET", "/api/v1/workspaces/ws-a", "rita@example.com", "TREUser", ""); code != http.StatusOK {
		t.Fatalf("member get code = %d", code)
	}
}

func TestAdminDeletesWorkspace(t *testing.T) {
	srv := newTestServer(t, existing("ws-gone", []string{"o@example.com"}, nil))
	if code, _ := call(t, srv, "DELETE", "/api/v1/workspaces/ws-gone", "admin@example.com", "TREAdmin", ""); code != http.StatusAccepted {
		t.Fatalf("delete code = %d", code)
	}
	if code, _ := call(t, srv, "GET", "/api/v1/workspaces/ws-gone", "admin@example.com", "TREAdmin", ""); code != http.StatusNotFound {
		t.Fatalf("get after delete code = %d", code)
	}
}
