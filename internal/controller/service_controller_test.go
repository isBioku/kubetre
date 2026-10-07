package controller

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
)

func readyWorkspace(t *testing.T, name string, level treV1.PodSecurityLevel) string {
	t.Helper()
	ctx := context.Background()
	tmpl := newTemplate("tpl-"+name, "")
	tmpl.Spec.Egress.AllowedFQDNs = nil
	tmpl.Spec.PodSecurity = level
	must(t, k8s.Create(ctx, tmpl))
	must(t, k8s.Create(ctx, newWorkspace(name, tmpl.Name)))
	eventually(t, "workspace ready", func() bool { return getWS(t, name).Status.Phase == treV1.PhaseReady })
	return NamespaceFor(getWS(t, name))
}

func serviceTemplate(name string, required treV1.PodSecurityLevel) *treV1.ServiceTemplate {
	return &treV1.ServiceTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: treV1.ServiceTemplateSpec{
			DisplayName: name, PerUser: true, RequiredPodSecurity: required,
			Chart:  treV1.ChartRef{URL: "oci://example.azurecr.io/charts/" + name, Version: "0.1.0", Provider: "azure"},
			Values: &apiextensionsv1.JSON{Raw: []byte(`{"image":{"tag":"1.0"},"resources":{"cpu":"1","memory":"2Gi"}}`)},
		},
	}
}

func getSvc(t *testing.T, ns, name string) *treV1.WorkspaceService {
	t.Helper()
	s := &treV1.WorkspaceService{}
	must(t, k8s.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: name}, s))
	return s
}

func svcReason(t *testing.T, ns, name string) string {
	c := meta.FindStatusCondition(getSvc(t, ns, name).Status.Conditions, treV1.ConditionReady)
	if c == nil {
		return ""
	}
	return c.Reason
}

func TestServiceRefusedWhenPodSecurityTooStrict(t *testing.T) {
	ctx := context.Background()
	ns := readyWorkspace(t, "svc-strict", treV1.PodSecurityRestricted)
	must(t, k8s.Create(ctx, serviceTemplate("vm-strict", treV1.PodSecurityBaseline)))
	must(t, k8s.Create(ctx, &treV1.WorkspaceService{
		ObjectMeta: metav1.ObjectMeta{Name: "rita-vm", Namespace: ns},
		Spec:       treV1.WorkspaceServiceSpec{TemplateRef: "vm-strict", DisplayName: "VM", Owner: "rita@example.com"},
	}))
	eventually(t, "refused for pod security", func() bool { return svcReason(t, ns, "rita-vm") == "PodSecurityTooStrict" })
}

func TestServiceOutsideWorkspaceIsRefused(t *testing.T) {
	ctx := context.Background()
	must(t, k8s.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "not-a-workspace"}}))
	must(t, k8s.Create(ctx, serviceTemplate("desk-outside", treV1.PodSecurityRestricted)))
	must(t, k8s.Create(ctx, &treV1.WorkspaceService{
		ObjectMeta: metav1.ObjectMeta{Name: "stray", Namespace: "not-a-workspace"},
		Spec:       treV1.WorkspaceServiceSpec{TemplateRef: "desk-outside", DisplayName: "x"},
	}))
	eventually(t, "refused outside workspace", func() bool { return svcReason(t, "not-a-workspace", "stray") == "NotInWorkspace" })
}

// Runs after the Cilium test (alphabetical "ZZZ"): installs Flux CRDs for the rest of the run.
func TestZZZServiceInstalledThroughFlux(t *testing.T) {
	ctx := context.Background()
	ns := readyWorkspace(t, "svc-flux", treV1.PodSecurityBaseline)
	must(t, k8s.Create(ctx, serviceTemplate("desktop", treV1.PodSecurityRestricted)))
	must(t, k8s.Create(ctx, &treV1.WorkspaceService{
		ObjectMeta: metav1.ObjectMeta{Name: "rita-desktop", Namespace: ns},
		Spec: treV1.WorkspaceServiceSpec{
			TemplateRef: "desktop", DisplayName: "Rita's desktop", Owner: "rita@example.com",
			// Users may override template values, but not the kubetre block.
			Values: &apiextensionsv1.JSON{Raw: []byte(`{"resources":{"memory":"4Gi"},"kubetre":{"owner":"mallory"}}`)},
		},
	}))

	eventually(t, "reports missing Flux", func() bool { return svcReason(t, ns, "rita-desktop") == "FluxNotInstalled" })

	if _, err := envtest.InstallCRDs(cfg, envtest.CRDInstallOptions{Paths: []string{"../../test/crossplane/testdata/crds/flux"}}); err != nil {
		t.Fatal(err)
	}
	// Nudge a reconcile instead of waiting for the one-minute requeue.
	svc := getSvc(t, ns, "rita-desktop")
	svc.Spec.DisplayName = "Rita's desktop (2)"
	must(t, k8s.Update(ctx, svc))

	hr := &unstructured.Unstructured{}
	hr.SetGroupVersionKind(helmReleaseGVK)
	eventually(t, "helmrelease created", func() bool {
		return k8s.Get(ctx, types.NamespacedName{Namespace: ns, Name: "rita-desktop"}, hr) == nil
	})
	if sa, _, _ := unstructured.NestedString(hr.Object, "spec", "serviceAccountName"); sa != DeployerName {
		t.Fatalf("serviceAccountName = %q", sa)
	}
	values, _, _ := unstructured.NestedMap(hr.Object, "spec", "values")
	res := values["resources"].(map[string]any)
	if res["memory"] != "4Gi" || res["cpu"] != "1" {
		t.Fatalf("values not merged: %v", res)
	}
	kt := values["kubetre"].(map[string]any)
	if kt["owner"] != "rita@example.com" || kt["workspace"] != "svc-flux" || kt["podSecurity"] != "baseline" {
		t.Fatalf("kubetre values = %v", kt)
	}

	repo := &unstructured.Unstructured{}
	repo.SetGroupVersionKind(ociRepositoryGVK)
	must(t, k8s.Get(ctx, types.NamespacedName{Namespace: ns, Name: "rita-desktop"}, repo))
	if p, _, _ := unstructured.NestedString(repo.Object, "spec", "provider"); p != "azure" {
		t.Fatalf("provider = %q", p)
	}
	if tag, _, _ := unstructured.NestedString(repo.Object, "spec", "ref", "tag"); tag != "0.1.0" {
		t.Fatalf("tag = %q", tag)
	}

	eventually(t, "pending while installing", func() bool { return getSvc(t, ns, "rita-desktop").Status.Phase == treV1.PhasePending })

	// Simulate Flux finishing the install.
	_ = unstructured.SetNestedSlice(hr.Object, []any{map[string]any{
		"type": "Ready", "status": "True", "reason": "InstallSucceeded", "message": "Helm install succeeded",
		"lastTransitionTime": metav1.Now().UTC().Format("2006-01-02T15:04:05Z"),
	}}, "status", "conditions")
	must(t, k8s.Status().Update(ctx, hr))
	eventually(t, "service ready", func() bool { return getSvc(t, ns, "rita-desktop").Status.Phase == treV1.PhaseReady })
}
