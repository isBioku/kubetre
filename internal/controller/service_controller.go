package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
)

// Flux kinds the service controller drives.
var (
	ociRepositoryGVK = schema.GroupVersionKind{Group: "source.toolkit.fluxcd.io", Version: "v1", Kind: "OCIRepository"}
	helmReleaseGVK   = schema.GroupVersionKind{Group: "helm.toolkit.fluxcd.io", Version: "v2", Kind: "HelmRelease"}
)

const (
	serviceTemplateIndex = ".spec.templateRef"
	pollInterval         = 15 * time.Second
	helmChartMediaType   = "application/vnd.cncf.helm.chart.content.v1.tar+gzip"
)

// WorkspaceServiceReconciler installs ServiceTemplate charts into workspaces through Flux.
type WorkspaceServiceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=kubetre.io,resources=workspaceservices,verbs=get;list;watch
// +kubebuilder:rbac:groups=kubetre.io,resources=workspaceservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kubetre.io,resources=servicetemplates,verbs=get;list;watch
// +kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=ocirepositories,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=helm.toolkit.fluxcd.io,resources=helmreleases,verbs=get;list;watch;create;update;patch;delete

// Reconcile drives a WorkspaceService towards an installed Helm release.
func (r *WorkspaceServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	svc := &treV1.WorkspaceService{}
	if err := r.Get(ctx, req.NamespacedName, svc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !svc.DeletionTimestamp.IsZero() {
		// Owner references delete the HelmRelease, and Flux uninstalls the chart.
		return ctrl.Result{}, nil
	}
	original := svc.Status.DeepCopy()
	svc.Status.ObservedGeneration = svc.Generation

	fail := func(reason, msg string, requeue time.Duration) (ctrl.Result, error) {
		svc.Status.Phase = treV1.PhaseFailed
		setReady(&svc.Status.Conditions, svc.Generation, false, reason, msg)
		return ctrl.Result{RequeueAfter: requeue}, r.writeStatus(ctx, svc, original)
	}

	ns := &corev1.Namespace{}
	if err := r.Get(ctx, types.NamespacedName{Name: svc.Namespace}, ns); err != nil {
		return ctrl.Result{}, err
	}
	workspace := ns.Labels[LabelWorkspace]
	if workspace == "" {
		return fail("NotInWorkspace", "services can only be installed in workspace namespaces", 0)
	}

	tmpl := &treV1.ServiceTemplate{}
	if err := r.Get(ctx, types.NamespacedName{Name: svc.Spec.TemplateRef}, tmpl); err != nil {
		if apierrors.IsNotFound(err) {
			return fail("TemplateNotFound", fmt.Sprintf("ServiceTemplate %q does not exist", svc.Spec.TemplateRef), 0)
		}
		return ctrl.Result{}, err
	}

	enforced := treV1.PodSecurityLevel(ns.Labels[LabelPodSecurity])
	required := tmpl.Spec.RequiredPodSecurity.OrDefault()
	if enforced.Rank() < required.Rank() {
		return fail("PodSecurityTooStrict", fmt.Sprintf(
			"this service needs pod security %q but the workspace enforces %q; use a workspace template that allows it",
			required, enforced.OrDefault()), 0)
	}

	if _, err := r.RESTMapper().RESTMapping(helmReleaseGVK.GroupKind(), helmReleaseGVK.Version); err != nil {
		if meta.IsNoMatchError(err) {
			return fail("FluxNotInstalled", "Flux source and helm controllers are required to install services", time.Minute)
		}
		return ctrl.Result{}, err
	}

	values, err := mergedValues(tmpl, svc, workspace, enforced.OrDefault())
	if err != nil {
		return fail("InvalidValues", err.Error(), 0)
	}

	repo := desiredOCIRepository(svc, tmpl)
	if err := controllerutil.SetControllerReference(svc, repo, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	if err := applySpec(ctx, r.Client, repo); err != nil {
		return ctrl.Result{}, fmt.Errorf("ocirepository: %w", err)
	}

	release := desiredHelmRelease(svc, workspace, values)
	if err := controllerutil.SetControllerReference(svc, release, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	if err := applySpec(ctx, r.Client, release); err != nil {
		return ctrl.Result{}, fmt.Errorf("helmrelease: %w", err)
	}

	phase, ok, reason, msg := helmReleaseState(release)
	svc.Status.Phase = phase
	setReady(&svc.Status.Conditions, svc.Generation, ok, reason, msg)
	if err := r.writeStatus(ctx, svc, original); err != nil {
		return ctrl.Result{}, err
	}
	if phase != treV1.PhaseReady {
		return ctrl.Result{RequeueAfter: pollInterval}, nil
	}
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// mergedValues layers admin values, then user values, then KubeTRE's own values.
// The "kubetre" key is always KubeTRE's, so users cannot spoof the owner or workspace.
func mergedValues(tmpl *treV1.ServiceTemplate, svc *treV1.WorkspaceService, workspace string, level treV1.PodSecurityLevel) (map[string]any, error) {
	out := map[string]any{}
	for _, src := range []*struct {
		name string
		raw  []byte
	}{{"template values", rawOf(tmpl.Spec.Values)}, {"service values", rawOf(svc.Spec.Values)}} {
		if len(src.raw) == 0 {
			continue
		}
		layer := map[string]any{}
		if err := json.Unmarshal(src.raw, &layer); err != nil {
			return nil, fmt.Errorf("%s are not a JSON object: %w", src.name, err)
		}
		deepMerge(out, layer)
	}
	out["kubetre"] = map[string]any{
		"workspace":   workspace,
		"service":     svc.Name,
		"owner":       svc.Spec.Owner,
		"podSecurity": string(level),
	}
	return out, nil
}

func rawOf(j *apiextensionsv1.JSON) []byte {
	if j == nil {
		return nil
	}
	return j.Raw
}

func deepMerge(dst, src map[string]any) {
	for k, v := range src {
		if sv, ok := v.(map[string]any); ok {
			if dv, ok := dst[k].(map[string]any); ok {
				deepMerge(dv, sv)
				continue
			}
		}
		dst[k] = v
	}
}

func serviceLabels(svc *treV1.WorkspaceService, workspace string) map[string]string {
	return map[string]string{LabelWorkspace: workspace, LabelManagedBy: ManagedByValue, "kubetre.io/service": svc.Name}
}

func desiredOCIRepository(svc *treV1.WorkspaceService, tmpl *treV1.ServiceTemplate) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(ociRepositoryGVK)
	u.SetName(svc.Name)
	u.SetNamespace(svc.Namespace)
	u.SetLabels(map[string]string{LabelManagedBy: ManagedByValue, "kubetre.io/service": svc.Name})
	provider := tmpl.Spec.Chart.Provider
	if provider == "" {
		provider = "generic"
	}
	u.Object["spec"] = map[string]any{
		"interval": "10m",
		"url":      tmpl.Spec.Chart.URL,
		"ref":      map[string]any{"tag": tmpl.Spec.Chart.Version},
		"provider": provider,
		"layerSelector": map[string]any{
			"mediaType": helmChartMediaType,
			"operation": "copy",
		},
	}
	return u
}

func desiredHelmRelease(svc *treV1.WorkspaceService, workspace string, values map[string]any) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(helmReleaseGVK)
	u.SetName(svc.Name)
	u.SetNamespace(svc.Namespace)
	u.SetLabels(serviceLabels(svc, workspace))
	u.Object["spec"] = map[string]any{
		"interval":    "10m",
		"releaseName": svc.Name,
		"chartRef":    map[string]any{"kind": "OCIRepository", "name": svc.Name},
		// Flux impersonates the workspace deployer, so the chart is confined to this namespace.
		"serviceAccountName": DeployerName,
		"install":            map[string]any{"remediation": map[string]any{"retries": int64(3)}},
		"upgrade":            map[string]any{"remediation": map[string]any{"retries": int64(3), "remediateLastFailure": true}},
		"values":             toUnstructuredJSON(values),
	}
	return u
}

// toUnstructuredJSON round-trips through JSON so numbers are int64/float64 as the
// unstructured converter expects.
func toUnstructuredJSON(v map[string]any) map[string]any {
	raw, _ := json.Marshal(v)
	out := map[string]any{}
	dec := json.NewDecoder(bytesReader(raw))
	dec.UseNumber()
	_ = dec.Decode(&out)
	return normalizeNumbers(out).(map[string]any)
}

func normalizeNumbers(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, e := range t {
			t[k] = normalizeNumbers(e)
		}
		return t
	case []any:
		for i, e := range t {
			t[i] = normalizeNumbers(e)
		}
		return t
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return i
		}
		f, _ := t.Float64()
		return f
	default:
		return v
	}
}

// helmReleaseState maps the HelmRelease Ready condition onto a KubeTRE phase.
func helmReleaseState(release *unstructured.Unstructured) (phase string, ok bool, reason, msg string) {
	conds, _, _ := unstructured.NestedSlice(release.Object, "status", "conditions")
	for _, c := range conds {
		m, _ := c.(map[string]any)
		if m["type"] != "Ready" {
			continue
		}
		reason, _ = m["reason"].(string)
		msg, _ = m["message"].(string)
		switch m["status"] {
		case "True":
			return treV1.PhaseReady, true, reason, msg
		case "False":
			if reason == "Progressing" || reason == "DependencyNotReady" {
				return treV1.PhasePending, false, reason, msg
			}
			return treV1.PhaseFailed, false, reason, msg
		}
		return treV1.PhasePending, false, reason, msg
	}
	return treV1.PhasePending, false, "Installing", "waiting for Flux to install the chart"
}

func setReady(conds *[]metav1.Condition, gen int64, ok bool, reason, msg string) {
	status := metav1.ConditionFalse
	if ok {
		status = metav1.ConditionTrue
	}
	if reason == "" {
		reason = "Unknown"
	}
	meta.SetStatusCondition(conds, metav1.Condition{
		Type: treV1.ConditionReady, Status: status, Reason: reason, Message: msg, ObservedGeneration: gen,
	})
}

func (r *WorkspaceServiceReconciler) writeStatus(ctx context.Context, svc *treV1.WorkspaceService, original *treV1.WorkspaceServiceStatus) error {
	if equality.Semantic.DeepEqual(original, &svc.Status) {
		return nil
	}
	return r.Status().Update(ctx, svc)
}

// SetupWithManager registers the controller. Flux objects are polled rather than watched,
// so the controller starts even before Flux is installed.
func (r *WorkspaceServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &treV1.WorkspaceService{}, serviceTemplateIndex,
		func(o client.Object) []string { return []string{o.(*treV1.WorkspaceService).Spec.TemplateRef} }); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&treV1.WorkspaceService{}).
		Watches(&treV1.ServiceTemplate{}, handler.EnqueueRequestsFromMapFunc(r.servicesForTemplate)).
		Named("workspaceservice").
		Complete(r)
}

func (r *WorkspaceServiceReconciler) servicesForTemplate(ctx context.Context, o client.Object) []reconcile.Request {
	list := &treV1.WorkspaceServiceList{}
	if err := r.List(ctx, list, client.MatchingFields{serviceTemplateIndex: o.GetName()}); err != nil {
		return nil
	}
	reqs := make([]reconcile.Request, 0, len(list.Items))
	for _, s := range list.Items {
		reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: s.Namespace, Name: s.Name}})
	}
	return reqs
}
