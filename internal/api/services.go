package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
	"github.com/isBioku/kubetre/internal/controller"
)

// maxServiceName keeps Helm release names (53 characters) and derived names well within limits.
const maxServiceName = 40

type serviceTemplateView struct {
	Name                string          `json:"name"`
	DisplayName         string          `json:"displayName"`
	Description         string          `json:"description,omitempty"`
	Version             string          `json:"version"`
	PerUser             bool            `json:"perUser"`
	RequiredPodSecurity string          `json:"requiredPodSecurity"`
	ValuesSchema        json.RawMessage `json:"valuesSchema,omitempty"`
}

type serviceView struct {
	Name        string             `json:"name"`
	Workspace   string             `json:"workspace"`
	TemplateRef string             `json:"templateRef"`
	DisplayName string             `json:"displayName"`
	Owner       string             `json:"owner,omitempty"`
	Values      json.RawMessage    `json:"values,omitempty"`
	Phase       string             `json:"phase"`
	Conditions  []metav1.Condition `json:"conditions,omitempty"`
}

func toServiceView(ws string, s *treV1.WorkspaceService) serviceView {
	v := serviceView{
		Name: s.Name, Workspace: ws, TemplateRef: s.Spec.TemplateRef, DisplayName: s.Spec.DisplayName,
		Owner: s.Spec.Owner, Phase: s.Status.Phase, Conditions: s.Status.Conditions,
	}
	if v.Phase == "" {
		v.Phase = treV1.PhasePending
	}
	if s.Spec.Values != nil {
		v.Values = s.Spec.Values.Raw
	}
	return v
}

func (s *Server) listServiceTemplates(w http.ResponseWriter, r *http.Request, _ Identity) {
	list := &treV1.ServiceTemplateList{}
	if err := s.Client.List(r.Context(), list); err != nil {
		s.internal(w, "list service templates", err)
		return
	}
	out := make([]serviceTemplateView, 0, len(list.Items))
	for _, t := range list.Items {
		v := serviceTemplateView{
			Name: t.Name, DisplayName: t.Spec.DisplayName, Description: t.Spec.Description,
			Version: t.Spec.Chart.Version, PerUser: t.Spec.PerUser,
			RequiredPodSecurity: string(t.Spec.RequiredPodSecurity.OrDefault()),
		}
		if t.Spec.ValuesSchema != nil {
			v.ValuesSchema = t.Spec.ValuesSchema.Raw
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"serviceTemplates": out})
}

// workspaceAccess loads a workspace and the caller's roles in it. Non-members get 404.
func (s *Server) workspaceAccess(w http.ResponseWriter, r *http.Request, id Identity) (*treV1.Workspace, map[string]bool, bool) {
	ws, ok := s.loadWorkspace(w, r)
	if !ok {
		return nil, nil, false
	}
	roles := map[string]bool{}
	for _, role := range workspaceRoles(ws, id) {
		roles[role] = true
	}
	if s.isAdmin(id) {
		roles[DefaultAdminRole] = true
	}
	if len(roles) == 0 {
		writeError(w, http.StatusNotFound, "workspace not found")
		return nil, nil, false
	}
	return ws, roles, true
}

// canSeeService: owners and admins see everything; others see shared services and their own.
func canSeeService(roles map[string]bool, id Identity, svc *treV1.WorkspaceService) bool {
	return roles[RoleOwner] || roles[DefaultAdminRole] || svc.Spec.Owner == "" || id.Matches(svc.Spec.Owner)
}

func (s *Server) listServices(w http.ResponseWriter, r *http.Request, id Identity) {
	ws, roles, ok := s.workspaceAccess(w, r, id)
	if !ok {
		return
	}
	list := &treV1.WorkspaceServiceList{}
	if err := s.Client.List(r.Context(), list, client.InNamespace(controller.NamespaceFor(ws))); err != nil {
		s.internal(w, "list services", err)
		return
	}
	out := []serviceView{}
	for i := range list.Items {
		if canSeeService(roles, id, &list.Items[i]) {
			out = append(out, toServiceView(ws.Name, &list.Items[i]))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"services": out})
}

func (s *Server) getService(w http.ResponseWriter, r *http.Request, id Identity) {
	ws, roles, ok := s.workspaceAccess(w, r, id)
	if !ok {
		return
	}
	svc, ok := s.loadService(w, r, ws)
	if !ok {
		return
	}
	if !canSeeService(roles, id, svc) {
		writeError(w, http.StatusNotFound, "service not found")
		return
	}
	writeJSON(w, http.StatusOK, toServiceView(ws.Name, svc))
}

type createServiceRequest struct {
	Name        string          `json:"name"`
	TemplateRef string          `json:"templateRef"`
	DisplayName string          `json:"displayName"`
	Values      json.RawMessage `json:"values"`
}

// createService: owners create shared services; any member creates their own per-user services.
func (s *Server) createService(w http.ResponseWriter, r *http.Request, id Identity) {
	ws, roles, ok := s.workspaceAccess(w, r, id)
	if !ok {
		return
	}
	var req createServiceRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	var problems []string
	if errs := validation.IsDNS1123Label(req.Name); len(errs) > 0 {
		problems = append(problems, "name: "+strings.Join(errs, "; "))
	} else if len(req.Name) > maxServiceName {
		problems = append(problems, fmt.Sprintf("name: must be at most %d characters", maxServiceName))
	}
	if strings.TrimSpace(req.DisplayName) == "" {
		problems = append(problems, "displayName: required")
	}
	if len(problems) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "invalid service", "details": problems})
		return
	}
	if ws.Status.Phase != treV1.PhaseReady {
		writeError(w, http.StatusConflict, "workspace is not ready")
		return
	}

	tmpl := &treV1.ServiceTemplate{}
	if err := s.Client.Get(r.Context(), types.NamespacedName{Name: req.TemplateRef}, tmpl); err != nil {
		if apierrors.IsNotFound(err) {
			writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("service template %q does not exist", req.TemplateRef))
			return
		}
		s.internal(w, "get service template", err)
		return
	}

	owner := ""
	if tmpl.Spec.PerUser {
		if !roles[RoleOwner] && !roles[RoleResearcher] {
			writeError(w, http.StatusForbidden, "only workspace owners and researchers can create personal services")
			return
		}
		owner = id.Principal()
	} else if !roles[RoleOwner] {
		writeError(w, http.StatusForbidden, "only workspace owners can create shared services")
		return
	}

	values := normalizeObject(req.Values)
	if err := validateAgainstSchema("service-"+tmpl.Name, rawOrNil(tmpl.Spec.ValuesSchema), values); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "values do not match the template schema", "details": []string{err.Error()}})
		return
	}

	svc := &treV1.WorkspaceService{
		ObjectMeta: metav1.ObjectMeta{
			Name: req.Name, Namespace: controller.NamespaceFor(ws),
			Annotations: map[string]string{"kubetre.io/created-by": id.Principal()},
		},
		Spec: treV1.WorkspaceServiceSpec{
			TemplateRef: req.TemplateRef, DisplayName: req.DisplayName, Owner: owner,
			Values: &apiextensionsv1.JSON{Raw: values},
		},
	}
	if err := s.Client.Create(r.Context(), svc); err != nil {
		switch {
		case apierrors.IsAlreadyExists(err):
			writeError(w, http.StatusConflict, fmt.Sprintf("service %q already exists", req.Name))
		case apierrors.IsInvalid(err):
			writeError(w, http.StatusUnprocessableEntity, err.Error())
		default:
			s.internal(w, "create service", err)
		}
		return
	}
	s.logger().Info("service created", "workspace", ws.Name, "service", svc.Name, "template", tmpl.Name, "by", id.Principal())
	w.Header().Set("Location", fmt.Sprintf("/api/v1/workspaces/%s/services/%s", ws.Name, svc.Name))
	writeJSON(w, http.StatusAccepted, toServiceView(ws.Name, svc))
}

// deleteService: a personal service's owner, workspace owners and admins may delete.
func (s *Server) deleteService(w http.ResponseWriter, r *http.Request, id Identity) {
	ws, roles, ok := s.workspaceAccess(w, r, id)
	if !ok {
		return
	}
	svc, ok := s.loadService(w, r, ws)
	if !ok {
		return
	}
	if !canSeeService(roles, id, svc) {
		writeError(w, http.StatusNotFound, "service not found")
		return
	}
	personalOwner := svc.Spec.Owner != "" && id.Matches(svc.Spec.Owner)
	if !roles[RoleOwner] && !roles[DefaultAdminRole] && !personalOwner {
		writeError(w, http.StatusForbidden, "you cannot delete this service")
		return
	}
	if err := s.Client.Delete(r.Context(), svc); client.IgnoreNotFound(err) != nil {
		s.internal(w, "delete service", err)
		return
	}
	s.logger().Info("service deletion requested", "workspace", ws.Name, "service", svc.Name, "by", id.Principal())
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) loadService(w http.ResponseWriter, r *http.Request, ws *treV1.Workspace) (*treV1.WorkspaceService, bool) {
	svc := &treV1.WorkspaceService{}
	err := s.Client.Get(r.Context(), types.NamespacedName{Namespace: controller.NamespaceFor(ws), Name: r.PathValue("service")}, svc)
	switch {
	case apierrors.IsNotFound(err):
		writeError(w, http.StatusNotFound, "service not found")
		return nil, false
	case err != nil:
		s.internal(w, "get service", err)
		return nil, false
	}
	return svc, true
}

func rawOrNil(j *apiextensionsv1.JSON) []byte {
	if j == nil {
		return nil
	}
	return j.Raw
}

var errNotObject = errors.New("must be a JSON object")
