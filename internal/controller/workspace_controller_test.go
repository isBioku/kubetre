package controller

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
	"github.com/isBioku/kubetre/test/xrdcrd"
)

var (
	k8s     client.Client
	cfg     *rest.Config
	testEnv *envtest.Environment
)

func TestMain(m *testing.M) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		println("KUBEBUILDER_ASSETS is not set; run `make test`")
		os.Exit(1)
	}
	ctrl.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(os.Stderr), zap.Level(zapLevel())))

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		CRDs:                  []*apiextensionsv1.CustomResourceDefinition{xrdcrd.FromFile(filepath.Join("..", "..", "crossplane", "xrd-workspacenetwork.yaml"))},
		ErrorIfCRDPathMissing: true,
	}
	var err error
	if cfg, err = testEnv.Start(); err != nil {
		panic(err)
	}

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = treV1.AddToScheme(scheme)

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{Scheme: scheme, Metrics: metricsserver.Options{BindAddress: "0"}})
	if err != nil {
		panic(err)
	}
	pool, err := ParseVMPool("10.240.0.0/24", 26)
	if err != nil {
		panic(err)
	}
	if err := (&WorkspaceReconciler{Client: mgr.GetClient(), Scheme: scheme, APIReader: mgr.GetAPIReader(), VMPool: pool}).SetupWithManager(mgr); err != nil {
		panic(err)
	}
	if err := (&WorkspaceServiceReconciler{Client: mgr.GetClient(), Scheme: scheme}).SetupWithManager(mgr); err != nil {
		panic(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		if err := mgr.Start(ctx); err != nil {
			panic(err)
		}
	}()
	k8s, _ = client.New(cfg, client.Options{Scheme: scheme})

	code := m.Run()
	cancel()
	_ = testEnv.Stop()
	os.Exit(code)
}

func zapLevel() zapcore.Level { return zapcore.ErrorLevel }

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", what)
}

func newTemplate(name string, fqdns ...string) *treV1.WorkspaceTemplate {
	return &treV1.WorkspaceTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: treV1.WorkspaceTemplateSpec{
			DisplayName: "Base workspace",
			Version:     "0.1.0",
			Quota: corev1.ResourceList{
				corev1.ResourceRequestsCPU:    resource.MustParse("4"),
				corev1.ResourceRequestsMemory: resource.MustParse("8Gi"),
			},
			Egress: treV1.EgressSpec{AllowedFQDNs: fqdns},
		},
	}
}

func newWorkspace(name, tmpl string, fqdns ...string) *treV1.Workspace {
	return &treV1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: treV1.WorkspaceSpec{
			DisplayName: name,
			TemplateRef: tmpl,
			Owners:      []string{"owner@example.com"},
			Egress:      treV1.EgressSpec{AllowedFQDNs: fqdns},
		},
	}
}

func getWS(t *testing.T, name string) *treV1.Workspace {
	t.Helper()
	ws := &treV1.Workspace{}
	if err := k8s.Get(context.Background(), types.NamespacedName{Name: name}, ws); err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	return ws
}

func condition(ws *treV1.Workspace, t string) *metav1.Condition {
	return meta.FindStatusCondition(ws.Status.Conditions, t)
}

func TestWorkspaceBecomesReadyWithIsolatedNamespace(t *testing.T) {
	ctx := context.Background()
	must(t, k8s.Create(ctx, newTemplate("base-ready")))
	must(t, k8s.Create(ctx, newWorkspace("alpha", "base-ready")))

	eventually(t, "workspace ready", func() bool { return getWS(t, "alpha").Status.Phase == treV1.PhaseReady })

	ws := getWS(t, "alpha")
	if ws.Status.Namespace != "ws-alpha" {
		t.Fatalf("namespace = %q", ws.Status.Namespace)
	}
	if c := condition(ws, treV1.ConditionEgressPolicy); c == nil || c.Reason != "NoEgressRequested" {
		t.Fatalf("egress condition = %+v", c)
	}

	ns := &corev1.Namespace{}
	must(t, k8s.Get(ctx, types.NamespacedName{Name: "ws-alpha"}, ns))
	if ns.Labels["pod-security.kubernetes.io/enforce"] != "restricted" || ns.Labels[LabelWorkspace] != "alpha" {
		t.Fatalf("namespace labels = %v", ns.Labels)
	}

	quota := &corev1.ResourceQuota{}
	must(t, k8s.Get(ctx, types.NamespacedName{Namespace: "ws-alpha", Name: QuotaName}, quota))
	if got := quota.Spec.Hard[corev1.ResourceRequestsCPU]; got.String() != "4" {
		t.Fatalf("quota cpu = %s", got.String())
	}

	rb := &rbacv1.RoleBinding{}
	must(t, k8s.Get(ctx, types.NamespacedName{Namespace: "ws-alpha", Name: DeployerName}, rb))
	if rb.RoleRef.Name != DeployerClusterRole || rb.Subjects[0].Name != DeployerName {
		t.Fatalf("deployer binding = %+v", rb)
	}

	nps := &networkingv1.NetworkPolicyList{}
	must(t, k8s.List(ctx, nps, client.InNamespace("ws-alpha")))
	if len(nps.Items) != 5 {
		t.Fatalf("expected 5 network policies, got %d", len(nps.Items))
	}
	var denyAll bool
	for _, np := range nps.Items {
		if np.Name == "kubetre-default-deny" && len(np.Spec.Ingress) == 0 && len(np.Spec.Egress) == 0 && len(np.Spec.PolicyTypes) == 2 {
			denyAll = true
		}
	}
	if !denyAll {
		t.Fatal("default-deny policy missing or not deny-all")
	}
	for _, np := range nps.Items {
		if np.Name != "kubetre-allow-ingress-gateway" {
			continue
		}
		peer := np.Spec.Ingress[0].From[0]
		if peer.PodSelector == nil || peer.PodSelector.MatchLabels["app.kubernetes.io/name"] != "guacd" || peer.NamespaceSelector == nil {
			t.Fatalf("desktop ingress must be limited to guacd pods in gateway namespaces: %+v", peer)
		}
	}
}

func TestMissingTemplateFailsThenRecovers(t *testing.T) {
	ctx := context.Background()
	must(t, k8s.Create(ctx, newWorkspace("bravo", "late-template")))

	eventually(t, "workspace failed", func() bool {
		c := condition(getWS(t, "bravo"), treV1.ConditionReady)
		return c != nil && c.Reason == "TemplateNotFound"
	})
	if getWS(t, "bravo").Status.Phase != treV1.PhaseFailed {
		t.Fatal("expected Failed phase")
	}

	must(t, k8s.Create(ctx, newTemplate("late-template")))
	eventually(t, "workspace ready after template created", func() bool {
		return getWS(t, "bravo").Status.Phase == treV1.PhaseReady
	})
}

func TestEgressFailsClosedWithoutCilium(t *testing.T) {
	ctx := context.Background()
	must(t, k8s.Create(ctx, newTemplate("base-egress", "pypi.org")))
	must(t, k8s.Create(ctx, newWorkspace("charlie", "base-egress")))

	eventually(t, "egress condition reports Cilium missing", func() bool {
		c := condition(getWS(t, "charlie"), treV1.ConditionEgressPolicy)
		return c != nil && c.Reason == "CiliumNotInstalled" && c.Status == metav1.ConditionFalse
	})
	// The workspace is usable; internet egress is simply blocked by default-deny.
	eventually(t, "workspace ready", func() bool { return getWS(t, "charlie").Status.Phase == treV1.PhaseReady })
}

func TestDeletionRemovesNamespaceBeforeReleasingWorkspace(t *testing.T) {
	ctx := context.Background()
	must(t, k8s.Create(ctx, newTemplate("base-delete")))
	must(t, k8s.Create(ctx, newWorkspace("delta", "base-delete")))
	eventually(t, "workspace ready", func() bool { return getWS(t, "delta").Status.Phase == treV1.PhaseReady })

	must(t, k8s.Delete(ctx, getWS(t, "delta")))

	eventually(t, "namespace deletion requested", func() bool {
		ns := &corev1.Namespace{}
		err := k8s.Get(ctx, types.NamespacedName{Name: "ws-delta"}, ns)
		return apierrors.IsNotFound(err) || (err == nil && !ns.DeletionTimestamp.IsZero())
	})
	// envtest has no namespace controller, so the namespace stays Terminating and the
	// finalizer must keep the Workspace record alive.
	eventually(t, "workspace held in Deleting", func() bool { return getWS(t, "delta").Status.Phase == treV1.PhaseDeleting })
}

// Runs last: it installs the Cilium CRD for the rest of the process.
func TestZZEgressEnforcedWithCilium(t *testing.T) {
	ctx := context.Background()
	if _, err := envtest.InstallCRDs(cfg, envtest.CRDInstallOptions{Paths: []string{"../../test/crossplane/testdata/crds/cilium"}}); err != nil {
		t.Fatalf("install cilium crd: %v", err)
	}
	must(t, k8s.Create(ctx, newTemplate("base-cilium", "pypi.org", "*.r-project.org")))
	must(t, k8s.Create(ctx, newWorkspace("echo", "base-cilium", "PyPI.org", "github.com")))

	eventually(t, "egress enforced", func() bool {
		c := condition(getWS(t, "echo"), treV1.ConditionEgressPolicy)
		return c != nil && c.Reason == "Enforced"
	})

	cnp := &unstructured.Unstructured{}
	cnp.SetAPIVersion("cilium.io/v2")
	cnp.SetKind("CiliumNetworkPolicy")
	must(t, k8s.Get(ctx, types.NamespacedName{Namespace: "ws-echo", Name: EgressPolicy}, cnp))
	egress, _, _ := unstructured.NestedSlice(cnp.Object, "spec", "egress")
	fqdns := egress[1].(map[string]any)["toFQDNs"].([]any)
	want := []map[string]any{{"matchPattern": "*.r-project.org"}, {"matchName": "github.com"}, {"matchName": "pypi.org"}}
	if len(fqdns) != len(want) {
		t.Fatalf("toFQDNs = %v", fqdns)
	}
	for i, w := range want {
		for k, v := range w {
			if fqdns[i].(map[string]any)[k] != v {
				t.Fatalf("toFQDNs[%d] = %v, want %v", i, fqdns[i], w)
			}
		}
	}

	// Removing every destination removes the policy.
	ws := getWS(t, "echo")
	ws.Spec.Egress.AllowedFQDNs = nil
	must(t, k8s.Update(ctx, ws))
	tmpl := &treV1.WorkspaceTemplate{}
	must(t, k8s.Get(ctx, types.NamespacedName{Name: "base-cilium"}, tmpl))
	tmpl.Spec.Egress.AllowedFQDNs = nil
	must(t, k8s.Update(ctx, tmpl))
	eventually(t, "cilium policy removed", func() bool {
		return apierrors.IsNotFound(k8s.Get(ctx, types.NamespacedName{Namespace: "ws-echo", Name: EgressPolicy}, cnp))
	})
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
