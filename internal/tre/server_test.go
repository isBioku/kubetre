package tre

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
	"github.com/isBioku/kubetre/internal/api"
	"github.com/isBioku/kubetre/internal/controller"
)

const (
	admin  = "admin@example.com"
	owner  = "olu@example.com"
	rita   = "rita@example.com"
	sam    = "sam@example.com"
	nobody = "eve@example.com"
)

type env struct {
	t   *testing.T
	srv *httptest.Server
	c   client.Client
}

func newEnv(t *testing.T, objs ...client.Object) *env {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := treV1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	vmSize := `{"type":"object","properties":{
	  "size":{"type":"string","enum":["small","medium"],"default":"small","updateable":true},
	  "diskGi":{"type":"integer","minimum":30,"default":64}},"additionalProperties":false}`
	base := []client.Object{
		&treV1.WorkspaceTemplate{ObjectMeta: metav1.ObjectMeta{Name: "vm-workspace"}, Spec: treV1.WorkspaceTemplateSpec{
			DisplayName: "Workspace with virtual machines", Version: "0.1.0",
			ParametersSchema: &apiextensionsv1.JSON{Raw: []byte(`{"type":"object","required":["costCentre"],
			  "properties":{"costCentre":{"type":"string","pattern":"^CC-[0-9]{4}$"}},"additionalProperties":false}`)},
		}},
		&treV1.ServiceTemplate{ObjectMeta: metav1.ObjectMeta{Name: "virtual-desktops"}, Spec: treV1.ServiceTemplateSpec{
			Kind: treV1.KindWorkspaceService, DisplayName: "Virtual Desktops"}},
		&treV1.ServiceTemplate{ObjectMeta: metav1.ObjectMeta{Name: "windows-vm"}, Spec: treV1.ServiceTemplateSpec{
			Kind: treV1.KindUserResource, ParentTemplate: "virtual-desktops", DisplayName: "Windows VM",
			RequiresVirtualMachines: true, OwnerAccount: true, Chart: &treV1.ChartRef{URL: "oci://r/charts/research-vm", Version: "0.2.0"},
			ValuesSchema: &apiextensionsv1.JSON{Raw: []byte(vmSize)}}},
		&treV1.ServiceTemplate{ObjectMeta: metav1.ObjectMeta{Name: "linux-desktop"}, Spec: treV1.ServiceTemplateSpec{
			Kind: treV1.KindUserResource, ParentTemplate: "virtual-desktops", DisplayName: "Linux desktop",
			Chart: &treV1.ChartRef{URL: "oci://r/charts/linux-desktop", Version: "0.2.0"}}},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(append(base, objs...)...).
		WithStatusSubresource(&treV1.Workspace{}, &treV1.WorkspaceService{}).Build()
	s := &Server{Client: c, Auth: api.DevAuthenticator{}, APIScope: "api://kubetre-api",
		GatewayURL: "https://tre.example.org/gateway", Version: "0.2.0"}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return &env{t: t, srv: srv, c: c}
}

func (e *env) call(method, path, user, body string, headers ...string) (int, map[string]any) {
	e.t.Helper()
	req, _ := http.NewRequest(method, e.srv.URL+path, strings.NewReader(body))
	if user != "" {
		req.Header.Set("X-Dev-User", user)
		if user == admin {
			req.Header.Set("X-Dev-Roles", "TREAdmin")
		}
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func (e *env) expect(code int, method, path, user, body string, headers ...string) map[string]any {
	e.t.Helper()
	got, out := e.call(method, path, user, body, headers...)
	if got != code {
		e.t.Fatalf("%s %s as %s: got %d, want %d: %v", method, path, user, got, code, out)
	}
	return out
}

func m(v any) map[string]any { return v.(map[string]any) }
func list(v any) []any       { return v.([]any) }

func vmWorkspace(name string) *treV1.Workspace {
	return &treV1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: name, Generation: 1},
		Spec: treV1.WorkspaceSpec{DisplayName: "Study one", TemplateRef: "vm-workspace", Owners: []string{owner},
			Researchers: []string{rita, sam}},
		Status: treV1.WorkspaceStatus{Phase: treV1.PhaseReady, ObservedGeneration: 1,
			VMNetwork: &treV1.VMNetworkStatus{AddressPrefix: "10.240.0.0/26"}},
	}
}

func (e *env) setPhase(obj client.Object, phase string) {
	e.t.Helper()
	ctx := context.Background()
	if err := e.c.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
		e.t.Fatal(err)
	}
	switch o := obj.(type) {
	case *treV1.WorkspaceService:
		o.Status.Phase, o.Status.ObservedGeneration = phase, o.Generation
	case *treV1.Workspace:
		o.Status.Phase, o.Status.ObservedGeneration = phase, o.Generation
	}
	if err := e.c.Status().Update(ctx, obj); err != nil {
		e.t.Fatal(err)
	}
}

func TestMetadataAndHealthAreServed(t *testing.T) {
	e := newEnv(t)
	if v := e.expect(200, "GET", "/api/.metadata", "", "")["api_version"]; v != "0.2.0" {
		t.Fatalf("api_version = %v", v)
	}
	e.expect(200, "GET", "/api/health", "", "")
	e.expect(401, "GET", "/api/workspaces", "", "")
}

func TestOnlyAdminsCreateWorkspacesAndCreatorBecomesOwner(t *testing.T) {
	e := newEnv(t)
	body := `{"templateName":"vm-workspace","properties":{"display_name":"Genomics study","description":"d",
	  "costCentre":"CC-1234","researchers":["rita@example.com"],"allowed_fqdns":["pypi.org"]}}`
	e.expect(403, "POST", "/api/workspaces", owner, body)
	op := m(e.expect(202, "POST", "/api/workspaces", admin, body)["operation"])
	if op["action"] != "install" || !strings.HasPrefix(op["resourcePath"].(string), "/workspaces/genomics-study-") {
		t.Fatalf("operation = %v", op)
	}

	wss := list(e.expect(200, "GET", "/api/workspaces", admin, "")["workspaces"])
	if len(wss) != 1 {
		t.Fatalf("want 1 workspace, got %d", len(wss))
	}
	ws := m(wss[0])
	props := m(ws["properties"])
	if props["display_name"] != "Genomics study" || props["scope_id"] != "api://kubetre-api" || props["costCentre"] != "CC-1234" {
		t.Fatalf("properties = %v", props)
	}
	if owners := list(props["owners"]); len(owners) != 1 || owners[0] != admin {
		t.Fatalf("creator should be the default owner, got %v", owners)
	}

	// Researchers see the workspace with their role; outsiders do not see it at all.
	if got := list(e.expect(200, "GET", "/api/workspaces", rita, "")["workspaces"]); len(got) != 1 {
		t.Fatalf("researcher should see the workspace")
	}
	path := "/api" + op["resourcePath"].(string)
	roles := list(m(e.expect(200, "GET", path, rita, "")["workspace"])["userRoles"])
	if len(roles) != 1 || roles[0] != "WorkspaceResearcher" {
		t.Fatalf("userRoles = %v", roles)
	}
	if got := list(e.expect(200, "GET", "/api/workspaces", nobody, "")["workspaces"]); len(got) != 0 {
		t.Fatalf("outsider should see no workspaces")
	}
	e.expect(404, "GET", path, nobody, "")
	if v := m(e.expect(200, "GET", path+"/scopeid", rita, "")["workspaceAuth"])["scopeId"]; v != "api://kubetre-api" {
		t.Fatalf("scopeId = %v", v)
	}
}

func TestWorkspaceCreateValidatesTemplateProperties(t *testing.T) {
	e := newEnv(t)
	e.expect(422, "POST", "/api/workspaces", admin, `{"templateName":"vm-workspace","properties":{"display_name":"x","costCentre":"nope"}}`)
	e.expect(422, "POST", "/api/workspaces", admin, `{"templateName":"vm-workspace","properties":{"costCentre":"CC-1234"}}`)
	e.expect(422, "POST", "/api/workspaces", admin,
		`{"templateName":"vm-workspace","properties":{"display_name":"x","costCentre":"CC-1234","allowed_fqdns":["not a host"]}}`)
	e.expect(400, "POST", "/api/workspaces", admin, `{"templateName":"missing","properties":{"display_name":"x"}}`)
}

func TestWorkspaceMustBeDisabledBeforeDeletion(t *testing.T) {
	e := newEnv(t, vmWorkspace("study1"))
	e.expect(400, "DELETE", "/api/workspaces/study1", admin, "")
	ws := m(e.expect(200, "GET", "/api/workspaces/study1", admin, "")["workspace"])
	e.expect(409, "PATCH", "/api/workspaces/study1", admin, `{"isEnabled":false}`, "etag", "stale")
	e.expect(202, "PATCH", "/api/workspaces/study1", admin, `{"isEnabled":false}`, "etag", ws["_etag"].(string))
	op := m(e.expect(202, "DELETE", "/api/workspaces/study1", admin, "")["operation"])
	got := m(e.expect(200, "GET", "/api"+op["resourcePath"].(string)+"/operations/"+op["id"].(string), admin, "")["operation"])
	if got["status"] != "deleted" {
		t.Fatalf("status = %v", got["status"])
	}
}

// The full journey AzureTRE's UI takes: the owner adds Virtual Desktops, then each
// researcher creates their own VM under it and can only see and connect to their own.
func TestResearchersCreateTheirOwnVMsUnderVirtualDesktops(t *testing.T) {
	e := newEnv(t, vmWorkspace("study1"))
	base := "/api/workspaces/study1/workspace-services"
	svcBody := `{"templateName":"virtual-desktops","properties":{"display_name":"Virtual Desktops","description":"d"}}`
	e.expect(403, "POST", base, rita, svcBody)
	e.expect(400, "POST", base, owner, `{"templateName":"windows-vm","properties":{"display_name":"x"}}`)
	op := m(e.expect(202, "POST", base, owner, svcBody)["operation"])
	svcName := op["resourceId"].(string)
	opPath := "/api" + op["resourcePath"].(string) + "/operations/" + op["id"].(string)
	if s := m(e.expect(200, "GET", opPath, owner, "")["operation"])["status"]; s != "deploying" {
		t.Fatalf("status before ready = %v", s)
	}

	urBase := base + "/" + svcName + "/user-resources"
	vmBody := `{"templateName":"windows-vm","properties":{"display_name":"Rita VM","description":"d","size":"medium"}}`
	e.expect(409, "POST", urBase, rita, vmBody) // the parent is not deployed yet

	svc := &treV1.WorkspaceService{ObjectMeta: metav1.ObjectMeta{Namespace: controller.NamespacePrefix + "study1", Name: svcName}}
	e.setPhase(svc, treV1.PhaseReady)
	if s := m(e.expect(200, "GET", opPath, owner, "")["operation"])["status"]; s != "deployed" {
		t.Fatalf("status after ready = %v", s)
	}

	e.expect(404, "POST", urBase, nobody, vmBody)
	e.expect(404, "GET", urBase, nobody, "")
	e.expect(422, "POST", urBase, rita, `{"templateName":"windows-vm","properties":{"display_name":"x","size":"huge"}}`)
	e.expect(400, "POST", urBase, rita, `{"templateName":"virtual-desktops","properties":{"display_name":"x"}}`)
	ritaVM := m(e.expect(202, "POST", urBase, rita, vmBody)["operation"])["resourceId"].(string)
	e.expect(202, "POST", urBase, sam, `{"templateName":"linux-desktop","properties":{"display_name":"Sam desktop","description":"d"}}`)

	if got := list(e.expect(200, "GET", urBase, rita, "")["userResources"]); len(got) != 1 {
		t.Fatalf("rita should see only her own resource, got %d", len(got))
	} else {
		ur := m(got[0])
		props := m(ur["properties"])
		if props["connection_uri"] != "https://tre.example.org/gateway/connect/study1/"+ritaVM || props["size"] != "medium" {
			t.Fatalf("properties = %v", props)
		}
		if ur["ownerId"] != rita || ur["parentWorkspaceServiceId"] != svcName || ur["resourceType"] != "user-resource" {
			t.Fatalf("user resource = %v", ur)
		}
	}
	if got := list(e.expect(200, "GET", urBase, owner, "")["userResources"]); len(got) != 2 {
		t.Fatalf("the workspace owner should see both resources, got %d", len(got))
	}
	e.expect(404, "GET", urBase+"/"+ritaVM, sam, "")
	e.expect(404, "PATCH", urBase+"/"+ritaVM, sam, `{"isEnabled":false}`)

	// Only fields marked updateable can change.
	e.expect(422, "PATCH", urBase+"/"+ritaVM, rita, `{"properties":{"diskGi":512}}`)
	e.expect(202, "PATCH", urBase+"/"+ritaVM, rita, `{"properties":{"size":"small"}}`)

	// Workspace services listed at the workspace level exclude user resources.
	if got := list(e.expect(200, "GET", base, rita, "")["workspaceServices"]); len(got) != 1 {
		t.Fatalf("want 1 workspace service, got %d", len(got))
	}

	// A parent with user resources cannot go, and nothing goes while enabled.
	e.expect(202, "PATCH", base+"/"+svcName, owner, `{"isEnabled":false}`)
	e.expect(400, "DELETE", base+"/"+svcName, owner, "")
	e.expect(400, "DELETE", urBase+"/"+ritaVM, rita, "")
	e.expect(202, "PATCH", urBase+"/"+ritaVM, rita, `{"isEnabled":false}`)
	e.expect(202, "DELETE", urBase+"/"+ritaVM, rita, "")
	got := &treV1.WorkspaceService{}
	if err := e.c.Get(context.Background(), types.NamespacedName{Namespace: svc.Namespace, Name: ritaVM}, got); err == nil {
		t.Fatalf("rita's VM should be deleted")
	}
}

func TestUserResourceTemplatesHideVMsWithoutVMNetworking(t *testing.T) {
	plain := vmWorkspace("plain")
	plain.Status.VMNetwork = nil
	e := newEnv(t, vmWorkspace("study1"), plain)
	path := func(ws string) string {
		return "/api/workspaces/" + ws + "/workspace-service-templates/virtual-desktops/user-resource-templates"
	}
	if got := list(e.expect(200, "GET", path("study1"), rita, "")["templates"]); len(got) != 2 {
		t.Fatalf("VM workspace should offer 2 templates, got %d", len(got))
	}
	got := list(e.expect(200, "GET", path("plain"), rita, "")["templates"])
	if len(got) != 1 || m(got[0])["name"] != "linux-desktop" {
		t.Fatalf("plain workspace should offer only the desktop, got %v", got)
	}
	if got := list(e.expect(200, "GET", "/api/workspace-service-templates", rita, "")["templates"]); len(got) != 1 {
		t.Fatalf("only workspace service templates are listed, got %v", got)
	}
}

func TestTemplatesRenderAsAzureTREForms(t *testing.T) {
	e := newEnv(t)
	tmpl := e.expect(200, "GET", "/api/workspace-service-templates/virtual-desktops/user-resource-templates/windows-vm", rita, "")
	props := m(tmpl["properties"])
	for _, k := range []string{"display_name", "description", "size", "diskGi"} {
		if _, ok := props[k]; !ok {
			t.Fatalf("template is missing %s: %v", k, props)
		}
	}
	if tmpl["resourceType"] != "user-resource" || tmpl["version"] != "0.2.0" {
		t.Fatalf("template = %v", tmpl)
	}
	upd := m(e.expect(200, "GET", "/api/workspace-service-templates/virtual-desktops/user-resource-templates/windows-vm?is_update=true", rita, "")["properties"])
	if m(upd["diskGi"])["readOnly"] != true || m(upd["size"])["readOnly"] != nil {
		t.Fatalf("update form: diskGi must be read-only and size editable: %v", upd)
	}
	ws := m(e.expect(200, "GET", "/api/workspace-templates/vm-workspace", admin, "")["properties"])
	for _, k := range []string{"display_name", "owners", "researchers", "costCentre"} {
		if _, ok := ws[k]; !ok {
			t.Fatalf("workspace template is missing %s", k)
		}
	}
	e.expect(404, "GET", "/api/workspace-service-templates/windows-vm", rita, "")
}

// A VM's account is named after its owner, set once at creation and never by the caller.
func TestVMAccountIsNamedAfterItsOwner(t *testing.T) {
	e := newEnv(t, vmWorkspace("study1"))
	base := "/api/workspaces/study1/workspace-services"
	svcName := m(e.expect(202, "POST", base, owner,
		`{"templateName":"virtual-desktops","properties":{"display_name":"Virtual Desktops","description":"d"}}`)["operation"])["resourceId"].(string)
	e.setPhase(&treV1.WorkspaceService{ObjectMeta: metav1.ObjectMeta{Namespace: controller.NamespacePrefix + "study1", Name: svcName}}, treV1.PhaseReady)
	urBase := base + "/" + svcName + "/user-resources"

	vm := m(e.expect(202, "POST", urBase, rita,
		`{"templateName":"windows-vm","properties":{"display_name":"VM","description":"d","size":"medium","username":"administrator"}}`)["operation"])["resourceId"].(string)
	desk := m(e.expect(202, "POST", urBase, rita,
		`{"templateName":"linux-desktop","properties":{"display_name":"Desk","description":"d"}}`)["operation"])["resourceId"].(string)

	values := func(name string) map[string]any {
		svc := &treV1.WorkspaceService{}
		if err := e.c.Get(context.Background(), types.NamespacedName{Namespace: controller.NamespacePrefix + "study1", Name: name}, svc); err != nil {
			t.Fatal(err)
		}
		return rawMap(svc.Spec.Values.Raw)
	}
	if got := values(vm)["username"]; got != "rita" {
		t.Fatalf("VM username = %v, want rita", got)
	}
	if _, ok := values(desk)["username"]; ok {
		t.Fatal("a container desktop should not get a VM username")
	}

	// Later updates still validate, and cannot rename the account.
	e.expect(202, "PATCH", urBase+"/"+vm, rita, `{"properties":{"size":"small","username":"root"}}`)
	if got := values(vm); got["username"] != "rita" || got["size"] != "small" {
		t.Fatalf("after update: %v", got)
	}
}
