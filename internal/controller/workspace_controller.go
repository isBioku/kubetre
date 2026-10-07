// Package controller reconciles KubeTRE workspaces into isolated namespaces.
package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
)

const (
	// Finalizer holds a Workspace until its namespace, and so its data, is gone.
	Finalizer = "kubetre.io/workspace-cleanup"

	templateRefIndex = ".spec.templateRef"
)

var ciliumPolicyGK = schema.GroupKind{Group: "cilium.io", Kind: "CiliumNetworkPolicy"}

// WorkspaceReconciler reconciles a Workspace object.
type WorkspaceReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// APIReader reads straight from the API server. Subnet allocation uses it so a status
	// written moments ago by another reconcile is never missed by a stale cache.
	APIReader client.Reader

	// VMPool is the address range workspace VM subnets are carved from. Nil disables VMs.
	VMPool *VMPool
}

// +kubebuilder:rbac:groups=kubetre.io,resources=workspaces,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=kubetre.io,resources=workspaces/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kubetre.io,resources=workspaces/finalizers,verbs=update
// +kubebuilder:rbac:groups=kubetre.io,resources=workspacetemplates,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=resourcequotas;limitranges,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=bind,resourceNames=kubetre-workspace-deployer
// +kubebuilder:rbac:groups=platform.kubetre.io,resources=workspacenetworks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cilium.io,resources=ciliumnetworkpolicies,verbs=get;list;watch;create;update;patch;delete

// Reconcile drives a Workspace towards its desired state.
func (r *WorkspaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	ws := &treV1.Workspace{}
	if err := r.Get(ctx, req.NamespacedName, ws); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !ws.DeletionTimestamp.IsZero() {
		return r.finalize(ctx, ws)
	}

	if controllerutil.AddFinalizer(ws, Finalizer) {
		if err := r.Update(ctx, ws); err != nil {
			return ctrl.Result{}, err
		}
	}

	original := ws.Status.DeepCopy()
	ws.Status.Namespace = NamespaceFor(ws)
	ws.Status.ObservedGeneration = ws.Generation

	tmpl := &treV1.WorkspaceTemplate{}
	if err := r.Get(ctx, types.NamespacedName{Name: ws.Spec.TemplateRef}, tmpl); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		ws.Status.Phase = treV1.PhaseFailed
		r.setCondition(ws, treV1.ConditionReady, false, "TemplateNotFound",
			fmt.Sprintf("WorkspaceTemplate %q does not exist", ws.Spec.TemplateRef))
		// The template watch requeues this workspace when the template appears.
		return ctrl.Result{}, r.writeStatus(ctx, ws, original)
	}

	if err := r.reconcileNamespace(ctx, ws, tmpl); err != nil {
		ws.Status.Phase = treV1.PhaseFailed
		r.setCondition(ws, treV1.ConditionNamespace, false, "ReconcileError", err.Error())
		r.setCondition(ws, treV1.ConditionReady, false, "NamespaceNotReady", err.Error())
		_ = r.writeStatus(ctx, ws, original)
		return ctrl.Result{}, err
	}
	r.setCondition(ws, treV1.ConditionNamespace, true, "Reconciled", "namespace, quota and limits are in place")

	if err := r.reconcileNetworkPolicies(ctx, ws); err != nil {
		ws.Status.Phase = treV1.PhaseFailed
		r.setCondition(ws, treV1.ConditionNetworkPolicy, false, "ReconcileError", err.Error())
		r.setCondition(ws, treV1.ConditionReady, false, "NetworkPolicyNotReady", err.Error())
		_ = r.writeStatus(ctx, ws, original)
		return ctrl.Result{}, err
	}
	r.setCondition(ws, treV1.ConditionNetworkPolicy, true, "Reconciled", "default-deny network policies are in place")

	if err := r.reconcileEgress(ctx, ws, tmpl); err != nil {
		r.setCondition(ws, treV1.ConditionEgressPolicy, false, "ReconcileError", err.Error())
		_ = r.writeStatus(ctx, ws, original)
		return ctrl.Result{}, err
	}

	if tmpl.Spec.VirtualMachines.Enabled {
		ready, reason, msg, err := r.reconcileVMNetwork(ctx, ws, tmpl)
		if err != nil {
			r.setCondition(ws, treV1.ConditionVMNetwork, false, "ReconcileError", err.Error())
			_ = r.writeStatus(ctx, ws, original)
			return ctrl.Result{}, err
		}
		r.setCondition(ws, treV1.ConditionVMNetwork, ready, reason, msg)
		if !ready {
			ws.Status.Phase = treV1.PhasePending
			if reason == "VMPoolNotConfigured" || reason == "VMPoolExhausted" {
				ws.Status.Phase = treV1.PhaseFailed
			}
			r.setCondition(ws, treV1.ConditionReady, false, reason, msg)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, r.writeStatus(ctx, ws, original)
		}
	}

	ws.Status.Phase = treV1.PhaseReady
	r.setCondition(ws, treV1.ConditionReady, true, "Reconciled", "workspace is ready")
	log.V(1).Info("reconciled workspace", "namespace", ws.Status.Namespace)
	return ctrl.Result{}, r.writeStatus(ctx, ws, original)
}

func (r *WorkspaceReconciler) reconcileNamespace(ctx context.Context, ws *treV1.Workspace, tmpl *treV1.WorkspaceTemplate) error {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: NamespaceFor(ws)}}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, ns, func() error {
		if ns.Labels == nil {
			ns.Labels = map[string]string{}
		}
		if owner := ns.Labels[LabelWorkspace]; ns.CreationTimestamp.Time != (time.Time{}) && owner != ws.Name {
			return fmt.Errorf("namespace %s exists and is not owned by this workspace", ns.Name)
		}
		for k, v := range namespaceLabels(ws, tmpl) {
			ns.Labels[k] = v
		}
		return controllerutil.SetControllerReference(ws, ns, r.Scheme)
	}); err != nil {
		return fmt.Errorf("namespace: %w", err)
	}

	quota := &corev1.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: QuotaName, Namespace: ns.Name}}
	if len(tmpl.Spec.Quota) == 0 {
		if err := r.Delete(ctx, quota); client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("quota: %w", err)
		}
	} else if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, quota, func() error {
		quota.Labels = commonLabels(ws)
		quota.Spec.Hard = tmpl.Spec.Quota.DeepCopy()
		return controllerutil.SetControllerReference(ws, quota, r.Scheme)
	}); err != nil {
		return fmt.Errorf("quota: %w", err)
	}

	if err := r.reconcileDeployer(ctx, ws); err != nil {
		return err
	}

	limits := &corev1.LimitRange{ObjectMeta: metav1.ObjectMeta{Name: LimitRangeName, Namespace: ns.Name}}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, limits, func() error {
		limits.Labels = commonLabels(ws)
		limits.Spec.Limits = defaultLimits()
		return controllerutil.SetControllerReference(ws, limits, r.Scheme)
	}); err != nil {
		return fmt.Errorf("limitrange: %w", err)
	}
	return nil
}

// reconcileDeployer creates the identity Flux uses to install charts in this workspace.
// It is bound to a namespace-scoped role that cannot change network policies, quotas or RBAC,
// so a chart cannot weaken the workspace's isolation.
func (r *WorkspaceReconciler) reconcileDeployer(ctx context.Context, ws *treV1.Workspace) error {
	ns := NamespaceFor(ws)
	sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: DeployerName, Namespace: ns}}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, sa, func() error {
		sa.Labels = commonLabels(ws)
		return controllerutil.SetControllerReference(ws, sa, r.Scheme)
	}); err != nil {
		return fmt.Errorf("deployer serviceaccount: %w", err)
	}
	rb := &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: DeployerName, Namespace: ns}}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, rb, func() error {
		rb.Labels = commonLabels(ws)
		rb.RoleRef = rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: DeployerClusterRole}
		rb.Subjects = []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: DeployerName, Namespace: ns}}
		return controllerutil.SetControllerReference(ws, rb, r.Scheme)
	}); err != nil {
		return fmt.Errorf("deployer rolebinding: %w", err)
	}
	return nil
}

func (r *WorkspaceReconciler) reconcileNetworkPolicies(ctx context.Context, ws *treV1.Workspace) error {
	for _, desired := range desiredNetworkPolicies(ws) {
		np := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: desired.Name, Namespace: desired.Namespace}}
		if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, np, func() error {
			np.Labels = desired.Labels
			np.Spec = desired.Spec
			return controllerutil.SetControllerReference(ws, np, r.Scheme)
		}); err != nil {
			return fmt.Errorf("networkpolicy %s: %w", desired.Name, err)
		}
	}
	return nil
}

// reconcileEgress manages the FQDN allowlist. Without Cilium the allowlist cannot be
// enforced, and the default-deny policy blocks all internet egress: the workspace fails closed.
func (r *WorkspaceReconciler) reconcileEgress(ctx context.Context, ws *treV1.Workspace, tmpl *treV1.WorkspaceTemplate) error {
	fqdns := AllowedFQDNs(tmpl, ws)

	if _, err := r.RESTMapper().RESTMapping(ciliumPolicyGK, "v2"); err != nil {
		if !meta.IsNoMatchError(err) {
			return err
		}
		if len(fqdns) == 0 {
			r.setCondition(ws, treV1.ConditionEgressPolicy, true, "NoEgressRequested", "no internet egress is allowed")
		} else {
			r.setCondition(ws, treV1.ConditionEgressPolicy, false, "CiliumNotInstalled",
				"FQDN allowlist cannot be enforced without Cilium; all internet egress is blocked")
		}
		return nil
	}

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(ciliumPolicyGK.WithVersion("v2"))
	key := types.NamespacedName{Namespace: NamespaceFor(ws), Name: EgressPolicy}

	if len(fqdns) == 0 {
		existing.SetName(key.Name)
		existing.SetNamespace(key.Namespace)
		if err := r.Delete(ctx, existing); client.IgnoreNotFound(err) != nil {
			return err
		}
		r.setCondition(ws, treV1.ConditionEgressPolicy, true, "NoEgressRequested", "no internet egress is allowed")
		return nil
	}

	desired := desiredCiliumPolicy(ws, fqdns)
	if err := controllerutil.SetControllerReference(ws, desired, r.Scheme); err != nil {
		return err
	}
	if err := applySpec(ctx, r.Client, desired); err != nil {
		return err
	}
	r.setCondition(ws, treV1.ConditionEgressPolicy, true, "Enforced",
		fmt.Sprintf("HTTPS egress allowed to %d destinations", len(fqdns)))
	return nil
}

var workspaceNetworkGVK = schema.GroupVersionKind{Group: "platform.kubetre.io", Version: "v1alpha1", Kind: "WorkspaceNetwork"}

// WorkspaceNetworkName is the name of the WorkspaceNetwork in each VM-enabled workspace.
const WorkspaceNetworkName = "vm-network"

// reconcileVMNetwork allocates the workspace's VM subnet once, then asks Crossplane for it.
func (r *WorkspaceReconciler) reconcileVMNetwork(ctx context.Context, ws *treV1.Workspace, tmpl *treV1.WorkspaceTemplate) (bool, string, string, error) {
	if ws.Status.VMNetwork == nil {
		if r.VMPool == nil {
			return false, "VMPoolNotConfigured", "the controller has no --vm-address-pool, so VM workspaces cannot be created", nil
		}
		reader := r.APIReader
		if reader == nil {
			reader = r.Client
		}
		list := &treV1.WorkspaceList{}
		if err := reader.List(ctx, list); err != nil {
			return false, "", "", err
		}
		used := map[string]bool{}
		for _, other := range list.Items {
			if other.Name != ws.Name && other.Status.VMNetwork != nil {
				used[other.Status.VMNetwork.AddressPrefix] = true
			}
		}
		alloc, err := r.VMPool.Allocate(used)
		if err != nil {
			return false, "VMPoolExhausted", err.Error(), nil
		}
		ws.Status.VMNetwork = &treV1.VMNetworkStatus{AddressPrefix: alloc.AddressPrefix, FirewallPriority: alloc.FirewallPriority}
		// Persist the allocation before creating anything that depends on it.
		if err := r.Status().Update(ctx, ws); err != nil {
			return false, "", "", err
		}
	}

	if _, err := r.RESTMapper().RESTMapping(workspaceNetworkGVK.GroupKind(), workspaceNetworkGVK.Version); err != nil {
		if meta.IsNoMatchError(err) {
			return false, "CrossplaneNotInstalled", "Crossplane and the KubeTRE XRDs are required for VM workspaces", nil
		}
		return false, "", "", err
	}

	fqdns := make([]any, 0)
	for _, f := range AllowedFQDNs(tmpl, ws) {
		fqdns = append(fqdns, f)
	}
	xr := &unstructured.Unstructured{}
	xr.SetGroupVersionKind(workspaceNetworkGVK)
	xr.SetName(WorkspaceNetworkName)
	xr.SetNamespace(NamespaceFor(ws))
	xr.SetLabels(commonLabels(ws))
	xr.Object["spec"] = map[string]any{"parameters": map[string]any{
		"addressPrefix":    ws.Status.VMNetwork.AddressPrefix,
		"firewallPriority": int64(ws.Status.VMNetwork.FirewallPriority),
		"allowedFQDNs":     fqdns,
	}}
	if err := applyParameters(ctx, r.Client, xr); err != nil {
		return false, "", "", fmt.Errorf("workspacenetwork: %w", err)
	}

	conds, _, _ := unstructured.NestedSlice(xr.Object, "status", "conditions")
	for _, c := range conds {
		m, _ := c.(map[string]any)
		if m["type"] == "Ready" && m["status"] == "True" {
			return true, "Provisioned", "VM subnet " + ws.Status.VMNetwork.AddressPrefix + " is ready", nil
		}
	}
	return false, "VMNetworkProvisioning", "waiting for the VM subnet " + ws.Status.VMNetwork.AddressPrefix + " to be provisioned", nil
}

func (r *WorkspaceReconciler) finalize(ctx context.Context, ws *treV1.Workspace) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(ws, Finalizer) {
		return ctrl.Result{}, nil
	}
	original := ws.Status.DeepCopy()

	ns := &corev1.Namespace{}
	err := r.Get(ctx, types.NamespacedName{Name: NamespaceFor(ws)}, ns)
	switch {
	case apierrors.IsNotFound(err):
		controllerutil.RemoveFinalizer(ws, Finalizer)
		return ctrl.Result{}, r.Update(ctx, ws)
	case err != nil:
		return ctrl.Result{}, err
	case ns.Labels[LabelWorkspace] != ws.Name:
		// Never delete a namespace this workspace does not own.
		controllerutil.RemoveFinalizer(ws, Finalizer)
		return ctrl.Result{}, r.Update(ctx, ws)
	}

	if ns.DeletionTimestamp.IsZero() {
		if err := r.Delete(ctx, ns); client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
	}
	ws.Status.Phase = treV1.PhaseDeleting
	r.setCondition(ws, treV1.ConditionReady, false, "Deleting", "waiting for the workspace namespace to be removed")
	if err := r.writeStatus(ctx, ws, original); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *WorkspaceReconciler) setCondition(ws *treV1.Workspace, t string, ok bool, reason, msg string) {
	status := metav1.ConditionFalse
	if ok {
		status = metav1.ConditionTrue
	}
	meta.SetStatusCondition(&ws.Status.Conditions, metav1.Condition{
		Type: t, Status: status, Reason: reason, Message: msg, ObservedGeneration: ws.Generation,
	})
}

func (r *WorkspaceReconciler) writeStatus(ctx context.Context, ws *treV1.Workspace, original *treV1.WorkspaceStatus) error {
	if equality.Semantic.DeepEqual(original, &ws.Status) {
		return nil
	}
	return r.Status().Update(ctx, ws)
}

// SetupWithManager registers the controller and its watches.
func (r *WorkspaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &treV1.Workspace{}, templateRefIndex,
		func(o client.Object) []string { return []string{o.(*treV1.Workspace).Spec.TemplateRef} }); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&treV1.Workspace{}).
		Owns(&corev1.Namespace{}).
		Owns(&corev1.ResourceQuota{}).
		Owns(&corev1.LimitRange{}).
		Owns(&networkingv1.NetworkPolicy{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.RoleBinding{}).
		Watches(&treV1.WorkspaceTemplate{}, handler.EnqueueRequestsFromMapFunc(r.workspacesForTemplate)).
		Named("workspace").
		Complete(r)
}

func (r *WorkspaceReconciler) workspacesForTemplate(ctx context.Context, o client.Object) []reconcile.Request {
	list := &treV1.WorkspaceList{}
	if err := r.List(ctx, list, client.MatchingFields{templateRefIndex: o.GetName()}); err != nil {
		return nil
	}
	reqs := make([]reconcile.Request, 0, len(list.Items))
	for _, ws := range list.Items {
		reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{Name: ws.Name}})
	}
	return reqs
}
