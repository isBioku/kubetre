// Package e2e runs the API and controller together against a real Kubernetes API server.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
	"github.com/isBioku/kubetre/internal/api"
	"github.com/isBioku/kubetre/internal/controller"
)

func TestWorkspaceLifecycleThroughAPI(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("KUBEBUILDER_ASSETS not set; run `make test`")
	}
	ctrl.SetLogger(zap.New(zap.WriteTo(os.Stderr), zap.Level(zapcore.ErrorLevel)))
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
	k8s, _ := client.New(cfg, client.Options{Scheme: scheme})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Every sample must be valid against the CRD schemas (strict: no unknown fields).
	samples, _ := filepath.Glob(filepath.Join(root, "config", "samples", "*.yaml"))
	for _, f := range samples {
		if strings.HasPrefix(filepath.Base(f), "workspace-") {
			continue // workspaces are created through the API below
		}
		applyYAML(t, k8s, f)
	}

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{Scheme: scheme, Metrics: metricsserver.Options{BindAddress: "0"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := (&controller.WorkspaceReconciler{Client: mgr.GetClient(), Scheme: scheme}).SetupWithManager(mgr); err != nil {
		t.Fatal(err)
	}
	go func() { _ = mgr.Start(ctx) }()

	srv := httptest.NewServer((&api.Server{Client: k8s, Auth: api.DevAuthenticator{}}).Handler())
	t.Cleanup(srv.Close)

	// 1. A researcher sees the template catalogue with its form schema.
	code, body := do(t, srv, "GET", "/api/v1/templates", "rita@example.com", "TREUser", "")
	if code != 200 || !strings.Contains(body, `"costCentre"`) {
		t.Fatalf("templates: %d %s", code, body)
	}

	// 2. An admin creates a workspace; the API validates parameters against the template.
	create := `{"name":"genomics","displayName":"Genomics","templateRef":"base",
	  "parameters":{"costCentre":"CC-1234","dataClassification":"sensitive"},
	  "owners":["olive@example.com"],"researchers":["rita@example.com"]}`
	if code, body = do(t, srv, "POST", "/api/v1/workspaces", "admin@example.com", "TREAdmin", create); code != 202 {
		t.Fatalf("create: %d %s", code, body)
	}

	// 3. The controller makes it Ready; the researcher can see it and their role.
	deadline := time.Now().Add(30 * time.Second)
	var view map[string]any
	for time.Now().Before(deadline) {
		_, body = do(t, srv, "GET", "/api/v1/workspaces/genomics", "rita@example.com", "TREUser", "")
		_ = json.Unmarshal([]byte(body), &view)
		if view["phase"] == treV1.PhaseReady {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if view["phase"] != treV1.PhaseReady || view["namespace"] != "ws-genomics" {
		t.Fatalf("workspace never became ready: %s", body)
	}
	if roles := view["yourRoles"].([]any); len(roles) != 1 || roles[0] != api.RoleResearcher {
		t.Fatalf("yourRoles = %v", roles)
	}

	ns := &corev1.Namespace{}
	if err := k8s.Get(ctx, types.NamespacedName{Name: "ws-genomics"}, ns); err != nil {
		t.Fatal(err)
	}
	quota := &corev1.ResourceQuota{}
	if err := k8s.Get(ctx, types.NamespacedName{Namespace: "ws-genomics", Name: controller.QuotaName}, quota); err != nil {
		t.Fatal(err)
	}
	if got := quota.Spec.Hard[corev1.ResourceRequestsStorage]; got.String() != "500Gi" {
		t.Fatalf("quota storage = %s", got.String())
	}

	// 4. The CRD forbids switching a workspace to another template.
	ws := &treV1.Workspace{}
	_ = k8s.Get(ctx, types.NamespacedName{Name: "genomics"}, ws)
	ws.Spec.TemplateRef = "something-else"
	if err := k8s.Update(ctx, ws); err == nil || !strings.Contains(err.Error(), "templateRef is immutable") {
		t.Fatalf("expected immutability error, got %v", err)
	}

	// 5. An outsider cannot see it, and an admin deletes it.
	if code, _ = do(t, srv, "GET", "/api/v1/workspaces/genomics", "eve@example.com", "TREUser", ""); code != 404 {
		t.Fatalf("outsider got %d", code)
	}
	if code, _ = do(t, srv, "DELETE", "/api/v1/workspaces/genomics", "admin@example.com", "TREAdmin", ""); code != 202 {
		t.Fatalf("delete: %d", code)
	}
	deadline = time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		_ = k8s.Get(ctx, types.NamespacedName{Name: "ws-genomics"}, ns)
		if !ns.DeletionTimestamp.IsZero() {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("namespace deletion was never requested")
}

func applyYAML(t *testing.T, c client.Client, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dec := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(raw), 4096)
	for {
		u := &unstructured.Unstructured{}
		if err := dec.Decode(&u.Object); err != nil {
			break
		}
		if len(u.Object) == 0 {
			continue
		}
		if err := c.Create(context.Background(), u, client.FieldValidation("Strict")); err != nil {
			t.Fatalf("apply %s: %v", path, err)
		}
	}
}

func do(t *testing.T, srv *httptest.Server, method, path, user, roles, body string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	req.Header.Set("X-Dev-User", user)
	req.Header.Set("X-Dev-Roles", roles)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	return resp.StatusCode, buf.String()
}
