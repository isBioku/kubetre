package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
	"github.com/isBioku/kubetre/internal/api"
	"github.com/isBioku/kubetre/internal/controller"
	"github.com/isBioku/kubetre/internal/tre"
)

// The AzureTRE API runs with only the kubetre-api service account's ClusterRole. Every
// action the UI takes must work under it: a unit test's fake client checks no RBAC, and a
// missing verb shows up in the UI as "The TRE API is currently unavailable".
func TestAzureTREAPIWorksUnderItsRBAC(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("KUBEBUILDER_ASSETS not set; run `make test`")
	}
	root := filepath.Join("..", "..")
	env := &envtest.Environment{CRDDirectoryPaths: []string{filepath.Join(root, "config", "crd", "bases")}, ErrorIfCRDPathMissing: true}
	cfg, err := env.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = env.Stop() })
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = treV1.AddToScheme(scheme)
	admin, _ := client.New(cfg, client.Options{Scheme: scheme})
	ctx := context.Background()

	for _, ns := range []string{"kubetre-system", controller.NamespacePrefix + "study1"} {
		if err := admin.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}); err != nil {
			t.Fatal(err)
		}
	}
	applyYAML(t, admin, filepath.Join(root, "config", "rbac", "api_role.yaml"))
	applyYAML(t, admin, filepath.Join(root, "config", "rbac", "bindings.yaml"))
	for _, f := range []string{"workspacetemplate-vm.yaml", "servicetemplate-virtual-desktops.yaml", "servicetemplate-windows-vm.yaml"} {
		applyYAML(t, admin, filepath.Join(root, "config", "templates", f))
	}

	asAPI := rest.CopyConfig(cfg)
	asAPI.Impersonate = rest.ImpersonationConfig{UserName: "system:serviceaccount:kubetre-system:kubetre-api"}
	c, err := client.New(asAPI, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer((&tre.Server{Client: c, Auth: api.DevAuthenticator{}, APIScope: "api://x",
		GatewayURL: "https://tre.example.org/gateway"}).Handler())
	t.Cleanup(srv.Close)

	call := func(method, path, user, body string) map[string]any {
		t.Helper()
		roles := ""
		if user == "admin@example.com" {
			roles = "TREAdmin"
		}
		code, out := do(t, srv, method, path, user, roles, body)
		if code >= 300 {
			t.Fatalf("%s %s as %s: %d %s", method, path, user, code, out)
		}
		m := map[string]any{}
		_ = json.Unmarshal([]byte(out), &m)
		return m
	}
	ready := func(obj client.Object) {
		t.Helper()
		if err := admin.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
			t.Fatal(err)
		}
		switch o := obj.(type) {
		case *treV1.Workspace:
			o.Status.Phase, o.Status.Namespace = treV1.PhaseReady, controller.NamespacePrefix+o.Name
			o.Status.VMNetwork = &treV1.VMNetworkStatus{AddressPrefix: "10.240.0.0/26", FirewallPriority: 1001}
		case *treV1.WorkspaceService:
			o.Status.Phase = treV1.PhaseReady
		}
		if err := admin.Status().Update(ctx, obj); err != nil {
			t.Fatal(err)
		}
	}

	// The workspace, created the way the UI creates it, then renamed for the test's namespace.
	if err := admin.Create(ctx, &treV1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "study1"}, Spec: treV1.WorkspaceSpec{
		DisplayName: "Study", TemplateRef: "vm-workspace", Owners: []string{"olu@example.com"},
		Researchers: []string{"rita@example.com"}}}); err != nil {
		t.Fatal(err)
	}
	ready(&treV1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "study1"}})
	call("POST", "/api/workspaces", "admin@example.com",
		`{"templateName":"vm-workspace","properties":{"display_name":"Other","description":"d","costCentre":"CC-1234"}}`)
	for _, p := range []string{"/api/workspaces", "/api/workspaces/study1", "/api/workspaces/study1/scopeid",
		"/api/workspace-templates", "/api/workspace-templates/vm-workspace", "/api/workspace-service-templates",
		"/api/workspaces/study1/workspace-service-templates/virtual-desktops/user-resource-templates",
		"/api/operations", "/api/workspaces/study1/operations", "/api/workspaces/study1/users"} {
		call("GET", p, "olu@example.com", "")
	}

	base := "/api/workspaces/study1/workspace-services"
	svc := call("POST", base, "olu@example.com",
		`{"templateName":"virtual-desktops","properties":{"display_name":"Virtual Desktops","description":"d"}}`)["operation"].(map[string]any)["resourceId"].(string)
	ns := controller.NamespacePrefix + "study1"
	ready(&treV1.WorkspaceService{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: svc}})
	ur := base + "/" + svc + "/user-resources"
	vm := call("POST", ur, "rita@example.com",
		`{"templateName":"windows-vm","properties":{"display_name":"VM","description":"d","size":"medium"}}`)["operation"].(map[string]any)["resourceId"].(string)
	call("GET", ur, "rita@example.com", "")
	call("GET", ur+"/"+vm+"/operations", "rita@example.com", "")

	// Disable, then delete: the path that failed live with a missing "update" verb.
	call("PATCH", ur+"/"+vm, "rita@example.com", `{"isEnabled":false}`)
	call("DELETE", ur+"/"+vm, "rita@example.com", "")
	call("PATCH", base+"/"+svc, "olu@example.com", `{"isEnabled":false}`)
	call("DELETE", base+"/"+svc, "olu@example.com", "")
	call("PATCH", "/api/workspaces/study1", "admin@example.com", `{"isEnabled":false}`)
	call("DELETE", "/api/workspaces/study1", "admin@example.com", "")

	got := &treV1.Workspace{}
	if err := admin.Get(ctx, types.NamespacedName{Name: "study1"}, got); err == nil && got.DeletionTimestamp.IsZero() {
		t.Fatal("the workspace should be deleted")
	}
	if code, body := do(t, srv, "GET", "/api/workspaces/missing", "olu@example.com", "", ""); code != http.StatusNotFound || !strings.Contains(body, "not found") {
		t.Fatalf("missing workspace: %d %s", code, body)
	}
}
