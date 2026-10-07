package controller

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
)

func getNetwork(t *testing.T, ws string) *unstructured.Unstructured {
	t.Helper()
	xr := &unstructured.Unstructured{}
	xr.SetGroupVersionKind(workspaceNetworkGVK)
	if err := k8s.Get(context.Background(), types.NamespacedName{Namespace: "ws-" + ws, Name: WorkspaceNetworkName}, xr); err != nil {
		return nil
	}
	return xr
}

func TestVMWorkspacesGetDistinctSubnetsAndWaitForNetwork(t *testing.T) {
	ctx := context.Background()
	tmpl := newTemplate("vm-tpl", "pypi.org")
	tmpl.Spec.VirtualMachines.Enabled = true
	must(t, k8s.Create(ctx, tmpl))
	// Created back to back, so allocation must not rely on a possibly stale cache.
	must(t, k8s.Create(ctx, newWorkspace("vm-one", "vm-tpl", "github.com")))
	must(t, k8s.Create(ctx, newWorkspace("vm-two", "vm-tpl")))

	for _, name := range []string{"vm-one", "vm-two"} {
		eventually(t, name+" waits for its VM network", func() bool {
			c := condition(getWS(t, name), treV1.ConditionVMNetwork)
			return c != nil && c.Reason == "VMNetworkProvisioning" && getNetwork(t, name) != nil
		})
		if getWS(t, name).Status.Phase != treV1.PhasePending {
			t.Fatalf("%s phase = %s, want Pending", name, getWS(t, name).Status.Phase)
		}
	}

	one, two := getWS(t, "vm-one").Status.VMNetwork, getWS(t, "vm-two").Status.VMNetwork
	if one.AddressPrefix == two.AddressPrefix || one.FirewallPriority == two.FirewallPriority {
		t.Fatalf("allocations collide: %+v %+v", one, two)
	}

	params, _, _ := unstructured.NestedMap(getNetwork(t, "vm-one").Object, "spec", "parameters")
	if params["addressPrefix"] != one.AddressPrefix || params["firewallPriority"] != int64(one.FirewallPriority) {
		t.Fatalf("parameters = %v, status = %+v", params, one)
	}
	if fq := params["allowedFQDNs"].([]any); len(fq) != 2 || fq[0] != "github.com" || fq[1] != "pypi.org" {
		t.Fatalf("allowedFQDNs = %v", fq)
	}

	// Simulate Crossplane reporting the Azure resources ready.
	xr := getNetwork(t, "vm-one")
	_ = unstructured.SetNestedSlice(xr.Object, []any{map[string]any{
		"type": "Ready", "status": "True", "reason": "Available",
		"lastTransitionTime": metav1.Now().UTC().Format("2006-01-02T15:04:05Z"),
	}}, "status", "conditions")
	must(t, k8s.Status().Update(ctx, xr))
	// Nudge a reconcile instead of waiting for the 30 second requeue.
	ws := getWS(t, "vm-one")
	ws.Spec.Description = "ready now"
	must(t, k8s.Update(ctx, ws))
	eventually(t, "vm-one ready", func() bool { return getWS(t, "vm-one").Status.Phase == treV1.PhaseReady })

	// The allocation never changes once made.
	if got := getWS(t, "vm-one").Status.VMNetwork; *got != *one {
		t.Fatalf("allocation changed: %+v -> %+v", one, got)
	}
}
