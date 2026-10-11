package controller

import (
	"context"
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
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

	// The workspace's VMs reach only pods marked for them, before the network is even ready.
	np := &networkingv1.NetworkPolicy{}
	must(t, k8s.Get(ctx, types.NamespacedName{Namespace: "ws-vm-one", Name: "kubetre-allow-from-vms"}, np))
	if len(np.Spec.Ingress) != 1 || np.Spec.Ingress[0].From[0].IPBlock.CIDR != one.AddressPrefix ||
		np.Spec.PodSelector.MatchLabels[LabelVMAccess] != "true" {
		t.Fatalf("VM ingress policy = %+v", np.Spec)
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

// A service can widen its workspace's egress and take extra subnets from the VM pool, which
// are released when it goes.
func TestServicesWidenEgressAndGetSubnets(t *testing.T) {
	ctx := context.Background()
	tmpl := newTemplate("svcnet-tpl", "pypi.org")
	tmpl.Spec.VirtualMachines.Enabled = true
	must(t, k8s.Create(ctx, tmpl))
	must(t, k8s.Create(ctx, newWorkspace("svcnet", "svcnet-tpl")))
	eventually(t, "VM subnet allocated", func() bool { return getWS(t, "svcnet").Status.VMNetwork != nil })
	must(t, k8s.Create(ctx, &treV1.ServiceTemplate{ObjectMeta: metav1.ObjectMeta{Name: "bricks"}, Spec: treV1.ServiceTemplateSpec{
		DisplayName: "Bricks", AllowedFQDNs: []string{"login.microsoftonline.com"}, Subnets: 2}}))
	must(t, k8s.Create(ctx, &treV1.WorkspaceService{ObjectMeta: metav1.ObjectMeta{Name: "bricks", Namespace: "ws-svcnet"},
		Spec: treV1.WorkspaceServiceSpec{TemplateRef: "bricks", DisplayName: "Bricks"}}))

	eventually(t, "service subnets and egress", func() bool {
		ws := getWS(t, "svcnet")
		params, _, _ := unstructured.NestedMap(getNetwork(t, "svcnet").Object, "spec", "parameters")
		fq, _ := params["allowedFQDNs"].([]any)
		return len(ws.Status.ServiceSubnets) == 1 && len(fq) == 2 && fq[0] == "login.microsoftonline.com"
	})
	ws := getWS(t, "svcnet")
	got := ws.Status.ServiceSubnets[0]
	if got.Service != "bricks" || len(got.AddressPrefixes) != 2 || got.FirewallPriority == 0 ||
		got.AddressPrefixes[0] == ws.Status.VMNetwork.AddressPrefix || got.AddressPrefixes[0] == got.AddressPrefixes[1] {
		t.Fatalf("service subnets = %+v, VM subnet %s", got, ws.Status.VMNetwork.AddressPrefix)
	}
	all := &treV1.WorkspaceList{}
	must(t, k8s.List(ctx, all))
	for _, other := range all.Items {
		if o := other.Status.VMNetwork; o != nil && (o.AddressPrefix == got.AddressPrefixes[0] || o.AddressPrefix == got.AddressPrefixes[1]) {
			t.Fatalf("service subnet collides with %s", other.Name)
		}
	}

	must(t, k8s.Delete(ctx, &treV1.WorkspaceService{ObjectMeta: metav1.ObjectMeta{Name: "bricks", Namespace: "ws-svcnet"}}))
	eventually(t, "subnets released and egress narrowed", func() bool {
		params, _, _ := unstructured.NestedMap(getNetwork(t, "svcnet").Object, "spec", "parameters")
		fq, _ := params["allowedFQDNs"].([]any)
		return len(getWS(t, "svcnet").Status.ServiceSubnets) == 0 && len(fq) == 1
	})
}
