package tre

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
	"github.com/isBioku/kubetre/internal/access"
	"github.com/isBioku/kubetre/internal/controller"
)

// AzureTRE operation actions.
const (
	actionInstall   = "install"
	actionUpdate    = "upgrade"
	actionUninstall = "uninstall"
)

// KubeTRE has no operations store: Kubernetes reconciles toward the desired state, so an
// operation is derived from the resource itself. Its ID encodes the resource UID, the action
// and the generation it was issued at, which is enough to tell when it has completed.
type operation struct {
	ID              string  `json:"id"`
	ResourceID      string  `json:"resourceId"`
	ResourcePath    string  `json:"resourcePath"`
	ResourceVersion int64   `json:"resourceVersion"`
	Status          string  `json:"status"`
	Action          string  `json:"action"`
	Message         string  `json:"message"`
	CreatedWhen     float64 `json:"createdWhen"`
	UpdatedWhen     float64 `json:"updatedWhen"`
	User            userRef `json:"user"`
	Steps           []step  `json:"steps"`
}

type step struct {
	TemplateStepID       string  `json:"templateStepId"`
	StepTitle            string  `json:"stepTitle"`
	ResourceID           string  `json:"resourceId"`
	ResourceTemplateName string  `json:"resourceTemplateName"`
	ResourceType         string  `json:"resourceType"`
	ResourceAction       string  `json:"resourceAction"`
	Status               string  `json:"status"`
	Message              string  `json:"message"`
	UpdatedWhen          float64 `json:"updatedWhen"`
}

func operationID(uid types.UID, action string, generation int64) string {
	return fmt.Sprintf("%s_%s_%d", uid, action, generation)
}

func parseOperationID(id string) (uid types.UID, action string, generation int64, ok bool) {
	parts := strings.Split(id, "_")
	if len(parts) != 3 {
		return "", "", 0, false
	}
	g, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return "", "", 0, false
	}
	switch parts[1] {
	case actionInstall, actionUpdate, actionUninstall:
	default:
		return "", "", 0, false
	}
	return types.UID(parts[0]), parts[1], g, true
}

func unix(t time.Time) float64 { return float64(t.UnixMilli()) / 1000 }

func newOperation(uid types.UID, action string, generation int64, path, resourceID, resourceType, templateName,
	status, message string, id access.Identity, now time.Time) operation {
	return buildOperation(uid, action, generation, path, resourceID, resourceType, templateName, status, message,
		userFromIdentity(id), unix(now), unix(now))
}

func buildOperation(uid types.UID, action string, generation int64, path, resourceID, resourceType, templateName,
	status, message string, user userRef, created, updated float64) operation {
	return operation{
		ID: operationID(uid, action, generation), ResourceID: resourceID, ResourcePath: path,
		ResourceVersion: generation, Status: status, Action: action, Message: message,
		CreatedWhen: created, UpdatedWhen: updated, User: user,
		Steps: []step{{
			TemplateStepID: "main", StepTitle: "Main step for " + resourceID, ResourceID: resourceID,
			ResourceTemplateName: templateName, ResourceType: resourceType, ResourceAction: action,
			Status: status, Message: message, UpdatedWhen: updated,
		}},
	}
}

// target is the resource an operations request refers to.
type target struct {
	obj          metav1.Object
	phase        string
	conditions   []metav1.Condition
	observed     int64
	path         string
	resourceType string
	template     string
}

// resolveTarget finds the resource named by the request path and checks the caller may see it.
// found is false when it no longer exists; ok is false when a response has been written.
func (s *Server) resolveTarget(w http.ResponseWriter, r *http.Request, id access.Identity) (t *target, found, ok bool) {
	wsName, svcName, urName := r.PathValue("ws"), r.PathValue("svc"), r.PathValue("ur")
	ws := &treV1.Workspace{}
	if err := s.Client.Get(r.Context(), types.NamespacedName{Name: wsName}, ws); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, false, true
		}
		s.internal(w, "get workspace", err)
		return nil, false, false
	}
	roles := rolesOf(ws, id)
	if len(roles) == 0 && !s.isAdmin(id) {
		writeError(w, http.StatusNotFound, "workspace not found")
		return nil, false, false
	}
	if svcName == "" {
		return &target{obj: ws, phase: ws.Status.Phase, conditions: ws.Status.Conditions, observed: ws.Status.ObservedGeneration,
			path: workspacePath(ws.Name), resourceType: typeWorkspace, template: ws.Spec.TemplateRef}, true, true
	}
	name := svcName
	if urName != "" {
		name = urName
	}
	svc := &treV1.WorkspaceService{}
	if err := s.Client.Get(r.Context(), types.NamespacedName{Namespace: controller.NamespaceFor(ws), Name: name}, svc); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, false, true
		}
		s.internal(w, "get service", err)
		return nil, false, false
	}
	if urName != "" && svc.Spec.ParentService != svcName {
		return nil, false, true
	}
	if urName != "" && !canSeeUserResource(roles, s.isAdmin(id), id, svc) {
		writeError(w, http.StatusNotFound, "user resource not found")
		return nil, false, false
	}
	t = &target{obj: svc, phase: svc.Status.Phase, conditions: svc.Status.Conditions, observed: svc.Status.ObservedGeneration,
		template: svc.Spec.TemplateRef}
	if urName != "" {
		t.path, t.resourceType = userResourcePath(ws.Name, svcName, svc.Name), typeUserResource
	} else {
		t.path, t.resourceType = servicePath(ws.Name, svc.Name), typeWorkspaceService
	}
	return t, true, true
}

// statusFor reports where an operation has got to.
func statusFor(t *target, found bool, uid types.UID, action string, generation int64) string {
	if !found || t.obj.GetUID() != uid {
		return "deleted"
	}
	if !t.obj.GetDeletionTimestamp().IsZero() || t.phase == treV1.PhaseDeleting {
		return "deleting"
	}
	switch action {
	case actionUninstall:
		return "deleting"
	case actionUpdate:
		if t.observed < generation {
			return "updating"
		}
		switch t.phase {
		case treV1.PhaseReady:
			return "updated"
		case treV1.PhaseFailed:
			return "updating_failed"
		}
		return "updating"
	default:
		switch t.phase {
		case treV1.PhaseReady:
			return "deployed"
		case treV1.PhaseFailed:
			return "deployment_failed"
		}
		return "deploying"
	}
}

func (s *Server) getOperation(w http.ResponseWriter, r *http.Request, id access.Identity) {
	uid, action, gen, ok := parseOperationID(r.PathValue("op"))
	if !ok {
		writeError(w, http.StatusNotFound, "operation not found")
		return
	}
	t, found, ok := s.resolveTarget(w, r, id)
	if !ok {
		return
	}
	status := statusFor(t, found, uid, action, gen)
	path := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api"), "/operations/"+r.PathValue("op"))
	resourceID, resourceType, template, message := lastSegment(path), "", "", ""
	created, updated := unix(s.now()), unix(s.now())
	user := userOf("")
	if found {
		resourceType, template = t.resourceType, t.template
		message = readyMessage(t.conditions)
		created, updated = unix(t.obj.GetCreationTimestamp().Time), updatedWhen(t.obj, t.conditions)
		user = createdBy(t.obj)
	}
	writeJSON(w, http.StatusOK, map[string]any{"operation": buildOperation(uid, action, gen, path, resourceID,
		resourceType, template, status, message, user, created, updated)})
}

// targetOperations lists the operations implied by a resource's current state.
func targetOperations(t *target) []operation {
	obj := t.obj
	created := unix(obj.GetCreationTimestamp().Time)
	updated := updatedWhen(obj, t.conditions)
	msg := readyMessage(t.conditions)
	user := createdBy(obj)
	ops := []operation{buildOperation(obj.GetUID(), actionInstall, 1, t.path, obj.GetName(), t.resourceType, t.template,
		statusFor(t, true, obj.GetUID(), actionInstall, 1), msg, user, created, updated)}
	if g := obj.GetGeneration(); g > 1 {
		ops = append(ops, buildOperation(obj.GetUID(), actionUpdate, g, t.path, obj.GetName(), t.resourceType, t.template,
			statusFor(t, true, obj.GetUID(), actionUpdate, g), msg, user, updated, updated))
	}
	if !obj.GetDeletionTimestamp().IsZero() {
		at := unix(obj.GetDeletionTimestamp().Time)
		ops = append(ops, buildOperation(obj.GetUID(), actionUninstall, obj.GetGeneration(), t.path, obj.GetName(),
			t.resourceType, t.template, "deleting", msg, user, at, at))
	}
	// Newest first, as AzureTRE returns them.
	for i, j := 0, len(ops)-1; i < j; i, j = i+1, j-1 {
		ops[i], ops[j] = ops[j], ops[i]
	}
	return ops
}

func (s *Server) listOperations(w http.ResponseWriter, r *http.Request, id access.Identity) {
	t, found, ok := s.resolveTarget(w, r, id)
	if !ok {
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "resource not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"operations": targetOperations(t)})
}

// myOperations returns the caller's operations that are still in progress, which the UI
// uses to restore its notifications after a page reload.
func (s *Server) myOperations(w http.ResponseWriter, r *http.Request, id access.Identity) {
	out := []operation{}
	wsList := &treV1.WorkspaceList{}
	if err := s.Client.List(r.Context(), wsList); err != nil {
		s.internal(w, "list workspaces", err)
		return
	}
	mine := func(obj metav1.Object) bool { return id.Matches(obj.GetAnnotations()[annCreatedBy]) }
	inProgress := func(ops []operation) {
		for _, op := range ops {
			switch op.Status {
			case "deploying", "updating", "deleting":
				out = append(out, op)
			}
		}
	}
	for i := range wsList.Items {
		ws := &wsList.Items[i]
		roles := rolesOf(ws, id)
		if len(roles) == 0 && !s.isAdmin(id) {
			continue
		}
		if mine(ws) {
			inProgress(targetOperations(&target{obj: ws, phase: ws.Status.Phase, conditions: ws.Status.Conditions,
				observed: ws.Status.ObservedGeneration, path: workspacePath(ws.Name), resourceType: typeWorkspace,
				template: ws.Spec.TemplateRef})[:1])
		}
		svcs := &treV1.WorkspaceServiceList{}
		if err := s.Client.List(r.Context(), svcs, client.InNamespace(controller.NamespaceFor(ws))); err != nil {
			continue
		}
		for j := range svcs.Items {
			svc := &svcs.Items[j]
			if !mine(svc) {
				continue
			}
			t := &target{obj: svc, phase: svc.Status.Phase, conditions: svc.Status.Conditions,
				observed: svc.Status.ObservedGeneration, template: svc.Spec.TemplateRef}
			if svc.Spec.ParentService != "" {
				t.path, t.resourceType = userResourcePath(ws.Name, svc.Spec.ParentService, svc.Name), typeUserResource
			} else {
				t.path, t.resourceType = servicePath(ws.Name, svc.Name), typeWorkspaceService
			}
			inProgress(targetOperations(t)[:1])
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"operations": out})
}

// KubeTRE keeps no resource history; AzureTRE's UI shows an empty list.
func (s *Server) getHistory(w http.ResponseWriter, r *http.Request, id access.Identity) {
	if _, found, ok := s.resolveTarget(w, r, id); !ok {
		return
	} else if !found {
		writeError(w, http.StatusNotFound, "resource not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"resource_history": []any{}})
}

func lastSegment(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

func rolesOf(ws *treV1.Workspace, id access.Identity) map[string]bool {
	roles := map[string]bool{}
	for _, role := range access.WorkspaceRoles(ws, id) {
		roles[role] = true
	}
	return roles
}
