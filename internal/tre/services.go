package tre

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
	"github.com/isBioku/kubetre/internal/access"
	"github.com/isBioku/kubetre/internal/controller"
	"github.com/isBioku/kubetre/internal/schema"
)

// Service properties held outside the template's values.
var serviceSystemProps = map[string]bool{"display_name": true, "description": true, "overview": true}

// Service properties KubeTRE derives and returns, never accepted as input.
var serviceDerivedProps = map[string]bool{"connection_uri": true, "is_exposed_externally": true, "owner_id": true,
	access.VMUsernameValue: true}

func servicePath(ws, svc string) string { return workspacePath(ws) + "/workspace-services/" + svc }

func userResourcePath(ws, svc, ur string) string {
	return servicePath(ws, svc) + "/user-resources/" + ur
}

// canSeeUserResource follows AzureTRE: workspace owners see every user resource,
// researchers only their own.
func canSeeUserResource(roles map[string]bool, admin bool, id access.Identity, ur *treV1.WorkspaceService) bool {
	return roles[access.RoleOwner] || admin || id.Matches(ur.Spec.Owner)
}

func (s *Server) serviceJSON(ws *treV1.Workspace, svc *treV1.WorkspaceService, tmpl *treV1.ServiceTemplate) resource {
	props := rawMap(rawOf(svc.Spec.Values))
	props["display_name"] = svc.Spec.DisplayName
	props["description"] = svc.Spec.Description
	if o := svc.Annotations[annOverview]; o != "" {
		props["overview"] = o
	}
	version := ""
	if tmpl != nil {
		version = templateVersion(tmpl)
	}
	res := resource{
		ID: svc.Name, IsEnabled: enabled(svc.Spec.Enabled), ResourceVersion: svc.Generation,
		TemplateName: svc.Spec.TemplateRef, TemplateVersion: version, AvailableUpgrades: []any{},
		DeploymentStatus: deploymentStatus(svc, svc.Status.Phase, svc.Generation),
		UpdatedWhen:      updatedWhen(svc, svc.Status.Conditions), User: createdBy(svc),
		Etag: svc.ResourceVersion, Properties: props, WorkspaceID: ws.Name,
	}
	if res.User.ID == "" && svc.Spec.Owner != "" {
		res.User = userOf(svc.Spec.Owner)
	}
	if svc.Spec.ParentService != "" {
		res.ResourceType = typeUserResource
		res.ResourcePath = userResourcePath(ws.Name, svc.Spec.ParentService, svc.Name)
		res.ParentWorkspaceServiceID = svc.Spec.ParentService
		res.OwnerID = svc.Annotations[annOwnerID]
		if res.OwnerID == "" {
			res.OwnerID = svc.Spec.Owner
		}
		props["owner_id"] = res.OwnerID
	} else {
		res.ResourceType = typeWorkspaceService
		res.ResourcePath = servicePath(ws.Name, svc.Name)
	}
	if tmpl != nil && tmpl.Spec.Chart != nil && tmpl.IsUserResource() && s.GatewayURL != "" {
		props["connection_uri"] = strings.TrimSuffix(s.GatewayURL, "/") + "/connect/" + ws.Name + "/" + svc.Name
		props["is_exposed_externally"] = true
	}
	return res
}

func templateVersion(t *treV1.ServiceTemplate) string {
	if t.Spec.Chart != nil && t.Spec.Chart.Version != "" {
		return t.Spec.Chart.Version
	}
	return "0.1.0"
}

func (s *Server) serviceTemplates(r *http.Request) map[string]*treV1.ServiceTemplate {
	list := &treV1.ServiceTemplateList{}
	out := map[string]*treV1.ServiceTemplate{}
	if err := s.Client.List(r.Context(), list); err == nil {
		for i := range list.Items {
			out[list.Items[i].Name] = &list.Items[i]
		}
	}
	return out
}

// loadService fetches a workspace service (no parent) or, when parent is set, a user resource under it.
func (s *Server) loadService(w http.ResponseWriter, r *http.Request, ws *treV1.Workspace, name, parent string) (*treV1.WorkspaceService, bool) {
	svc := &treV1.WorkspaceService{}
	err := s.Client.Get(r.Context(), types.NamespacedName{Namespace: controller.NamespaceFor(ws), Name: name}, svc)
	if err == nil && svc.Spec.ParentService != parent {
		err = apierrors.NewNotFound(treV1.GroupVersion.WithResource("workspaceservices").GroupResource(), name)
	}
	if apierrors.IsNotFound(err) {
		if parent == "" {
			writeError(w, http.StatusNotFound, "workspace service not found")
		} else {
			writeError(w, http.StatusNotFound, "user resource not found")
		}
		return nil, false
	}
	if err != nil {
		s.internal(w, "get service", err)
		return nil, false
	}
	return svc, true
}

func (s *Server) listChildren(r *http.Request, ws *treV1.Workspace) ([]treV1.WorkspaceService, error) {
	list := &treV1.WorkspaceServiceList{}
	if err := s.Client.List(r.Context(), list, client.InNamespace(controller.NamespaceFor(ws))); err != nil {
		return nil, err
	}
	return list.Items, nil
}

// ---- workspace services ----

func (s *Server) listWorkspaceServices(w http.ResponseWriter, r *http.Request, id access.Identity) {
	ws, _, ok := s.loadWorkspace(w, r, id)
	if !ok {
		return
	}
	items, err := s.listChildren(r, ws)
	if err != nil {
		s.internal(w, "list services", err)
		return
	}
	tmpls := s.serviceTemplates(r)
	out := []resource{}
	for i := range items {
		if items[i].Spec.ParentService == "" {
			out = append(out, s.serviceJSON(ws, &items[i], tmpls[items[i].Spec.TemplateRef]))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspaceServices": out})
}

func (s *Server) getWorkspaceService(w http.ResponseWriter, r *http.Request, id access.Identity) {
	ws, _, ok := s.loadWorkspace(w, r, id)
	if !ok {
		return
	}
	svc, ok := s.loadService(w, r, ws, r.PathValue("svc"), "")
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspaceService": s.serviceJSON(ws, svc, s.serviceTemplates(r)[svc.Spec.TemplateRef])})
}

func (s *Server) createWorkspaceService(w http.ResponseWriter, r *http.Request, id access.Identity) {
	ws, roles, ok := s.loadWorkspace(w, r, id)
	if !ok {
		return
	}
	if !roles[access.RoleOwner] && !s.isAdmin(id) {
		writeError(w, http.StatusForbidden, "only workspace owners can add workspace services")
		return
	}
	s.create(w, r, id, ws, nil)
}

func (s *Server) patchWorkspaceService(w http.ResponseWriter, r *http.Request, id access.Identity) {
	ws, roles, ok := s.loadWorkspace(w, r, id)
	if !ok {
		return
	}
	if !roles[access.RoleOwner] && !s.isAdmin(id) {
		writeError(w, http.StatusForbidden, "only workspace owners can update workspace services")
		return
	}
	svc, ok := s.loadService(w, r, ws, r.PathValue("svc"), "")
	if !ok {
		return
	}
	s.patch(w, r, id, ws, svc, servicePath(ws.Name, svc.Name), typeWorkspaceService)
}

func (s *Server) deleteWorkspaceService(w http.ResponseWriter, r *http.Request, id access.Identity) {
	ws, roles, ok := s.loadWorkspace(w, r, id)
	if !ok {
		return
	}
	if !roles[access.RoleOwner] && !s.isAdmin(id) {
		writeError(w, http.StatusForbidden, "only workspace owners can delete workspace services")
		return
	}
	svc, ok := s.loadService(w, r, ws, r.PathValue("svc"), "")
	if !ok {
		return
	}
	items, err := s.listChildren(r, ws)
	if err != nil {
		s.internal(w, "list services", err)
		return
	}
	for _, c := range items {
		if c.Spec.ParentService == svc.Name {
			writeError(w, http.StatusBadRequest, "the workspace service has user resources; delete them first")
			return
		}
	}
	s.remove(w, r, id, svc, servicePath(ws.Name, svc.Name), typeWorkspaceService)
}

// ---- user resources ----

func (s *Server) listUserResources(w http.ResponseWriter, r *http.Request, id access.Identity) {
	ws, roles, ok := s.loadWorkspace(w, r, id)
	if !ok {
		return
	}
	parent, ok := s.loadService(w, r, ws, r.PathValue("svc"), "")
	if !ok {
		return
	}
	items, err := s.listChildren(r, ws)
	if err != nil {
		s.internal(w, "list services", err)
		return
	}
	tmpls := s.serviceTemplates(r)
	admin := s.isAdmin(id)
	out := []resource{}
	for i := range items {
		c := &items[i]
		if c.Spec.ParentService == parent.Name && canSeeUserResource(roles, admin, id, c) {
			out = append(out, s.serviceJSON(ws, c, tmpls[c.Spec.TemplateRef]))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"userResources": out})
}

func (s *Server) loadUserResource(w http.ResponseWriter, r *http.Request, id access.Identity) (*treV1.Workspace, map[string]bool, *treV1.WorkspaceService, bool) {
	ws, roles, ok := s.loadWorkspace(w, r, id)
	if !ok {
		return nil, nil, nil, false
	}
	ur, ok := s.loadService(w, r, ws, r.PathValue("ur"), r.PathValue("svc"))
	if !ok {
		return nil, nil, nil, false
	}
	if !canSeeUserResource(roles, s.isAdmin(id), id, ur) {
		writeError(w, http.StatusNotFound, "user resource not found")
		return nil, nil, nil, false
	}
	return ws, roles, ur, true
}

func (s *Server) getUserResource(w http.ResponseWriter, r *http.Request, id access.Identity) {
	ws, _, ur, ok := s.loadUserResource(w, r, id)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"userResource": s.serviceJSON(ws, ur, s.serviceTemplates(r)[ur.Spec.TemplateRef])})
}

func (s *Server) createUserResource(w http.ResponseWriter, r *http.Request, id access.Identity) {
	ws, roles, ok := s.loadWorkspace(w, r, id)
	if !ok {
		return
	}
	if !roles[access.RoleOwner] && !roles[access.RoleResearcher] {
		writeError(w, http.StatusForbidden, "only workspace owners and researchers can create user resources")
		return
	}
	parent, ok := s.loadService(w, r, ws, r.PathValue("svc"), "")
	if !ok {
		return
	}
	if !enabled(parent.Spec.Enabled) || parent.Status.Phase != treV1.PhaseReady || !parent.DeletionTimestamp.IsZero() {
		writeError(w, http.StatusConflict, "the workspace service must be deployed and enabled first")
		return
	}
	s.create(w, r, id, ws, parent)
}

func (s *Server) patchUserResource(w http.ResponseWriter, r *http.Request, id access.Identity) {
	ws, roles, ur, ok := s.loadUserResource(w, r, id)
	if !ok {
		return
	}
	if !roles[access.RoleOwner] && !id.Matches(ur.Spec.Owner) {
		writeError(w, http.StatusForbidden, "only the owner of this resource or a workspace owner can update it")
		return
	}
	s.patch(w, r, id, ws, ur, userResourcePath(ws.Name, ur.Spec.ParentService, ur.Name), typeUserResource)
}

func (s *Server) deleteUserResource(w http.ResponseWriter, r *http.Request, id access.Identity) {
	ws, roles, ur, ok := s.loadUserResource(w, r, id)
	if !ok {
		return
	}
	if !roles[access.RoleOwner] && !s.isAdmin(id) && !id.Matches(ur.Spec.Owner) {
		writeError(w, http.StatusForbidden, "only the owner of this resource or a workspace owner can delete it")
		return
	}
	s.remove(w, r, id, ur, userResourcePath(ws.Name, ur.Spec.ParentService, ur.Name), typeUserResource)
}

// ---- shared create, update and delete ----

// splitServiceProps separates spec-level properties from chart values.
func splitServiceProps(props map[string]any) (system, values map[string]any) {
	system, values = map[string]any{}, map[string]any{}
	for k, v := range props {
		switch {
		case serviceSystemProps[k]:
			system[k] = v
		case serviceDerivedProps[k]:
		default:
			values[k] = v
		}
	}
	return system, values
}

func (s *Server) create(w http.ResponseWriter, r *http.Request, id access.Identity, ws *treV1.Workspace, parent *treV1.WorkspaceService) {
	b, ok := readBody(w, r)
	if !ok {
		return
	}
	tmpl := &treV1.ServiceTemplate{}
	if err := s.Client.Get(r.Context(), types.NamespacedName{Name: b.TemplateName}, tmpl); err != nil {
		if apierrors.IsNotFound(err) {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("template %q does not exist", b.TemplateName))
			return
		}
		s.internal(w, "get template", err)
		return
	}
	if parent == nil && tmpl.IsUserResource() {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("%q is a user resource template", tmpl.Name))
		return
	}
	if parent != nil && (!tmpl.IsUserResource() || tmpl.Spec.ParentTemplate != parent.Spec.TemplateRef) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("%q is not a user resource template of %q", tmpl.Name, parent.Spec.TemplateRef))
		return
	}
	system, values := splitServiceProps(b.Properties)
	display := strings.TrimSpace(str(system["display_name"]))
	if display == "" {
		writeError(w, http.StatusUnprocessableEntity, "display_name is required")
		return
	}
	raw, _ := json.Marshal(values)
	if err := schema.Validate("service-"+tmpl.Name, rawOf(tmpl.Spec.ValuesSchema), raw); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "properties do not match the template: "+err.Error())
		return
	}
	if parent != nil && tmpl.Spec.OwnerAccount {
		// The VM's account is named after its owner, as people expect at the Windows or
		// Linux login, instead of a shared name.
		var err error
		if raw, err = access.WithVMUsername(raw, id.Principal()); err != nil {
			s.internal(w, "set VM username", err)
			return
		}
	}
	svc := &treV1.WorkspaceService{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: slug(display, 30), Namespace: controller.NamespaceFor(ws),
			Annotations: map[string]string{annCreatedBy: id.Principal()},
		},
		Spec: treV1.WorkspaceServiceSpec{
			TemplateRef: tmpl.Name, DisplayName: display, Description: str(system["description"]),
			Values: &apiextensionsv1.JSON{Raw: raw},
		},
	}
	if o := str(system["overview"]); o != "" {
		svc.Annotations[annOverview] = o
	}
	path, kind := "", typeWorkspaceService
	if parent != nil {
		svc.Spec.ParentService = parent.Name
		svc.Spec.Owner = id.Principal()
		svc.Annotations[annOwnerID] = userID(id)
		kind = typeUserResource
	}
	if err := s.Client.Create(r.Context(), svc); err != nil {
		switch {
		case apierrors.IsInvalid(err):
			writeError(w, http.StatusUnprocessableEntity, err.Error())
		case apierrors.IsForbidden(err):
			// A ResourceQuota (for example the workspace's VM count) refused the object.
			writeError(w, http.StatusForbidden, err.Error())
		default:
			s.internal(w, "create service", err)
		}
		return
	}
	if parent != nil {
		path = userResourcePath(ws.Name, parent.Name, svc.Name)
	} else {
		path = servicePath(ws.Name, svc.Name)
	}
	s.logger().Info("resource created", "type", kind, "workspace", ws.Name, "name", svc.Name, "template", tmpl.Name, "by", id.Principal())
	writeJSON(w, http.StatusAccepted, map[string]any{"operation": newOperation(svc.UID, actionInstall, 1, path, svc.Name,
		kind, tmpl.Name, "awaiting_deployment", "", id, s.now())})
}

func (s *Server) patch(w http.ResponseWriter, r *http.Request, id access.Identity, ws *treV1.Workspace, svc *treV1.WorkspaceService, path, kind string) {
	if !etagMatches(r, svc.ResourceVersion) {
		writeError(w, http.StatusConflict, "the resource was changed by someone else; refresh and try again")
		return
	}
	b, ok := readBody(w, r)
	if !ok {
		return
	}
	if b.TemplateVersion != "" {
		writeError(w, http.StatusBadRequest, "template upgrades are not supported by KubeTRE yet")
		return
	}
	if b.IsEnabled != nil {
		v := *b.IsEnabled
		svc.Spec.Enabled = &v
	}
	if len(b.Properties) > 0 {
		system, values := splitServiceProps(b.Properties)
		if v, ok := system["display_name"]; ok && strings.TrimSpace(str(v)) != "" {
			svc.Spec.DisplayName = strings.TrimSpace(str(v))
		}
		if v, ok := system["description"]; ok {
			svc.Spec.Description = str(v)
		}
		if v, ok := system["overview"]; ok {
			if svc.Annotations == nil {
				svc.Annotations = map[string]string{}
			}
			svc.Annotations[annOverview] = str(v)
		}
		if len(values) > 0 {
			tmpl := &treV1.ServiceTemplate{}
			if err := s.Client.Get(r.Context(), types.NamespacedName{Name: svc.Spec.TemplateRef}, tmpl); err != nil {
				s.internal(w, "get template", err)
				return
			}
			current := rawMap(rawOf(svc.Spec.Values))
			for k, v := range values {
				if !updateable(tmpl, k) && fmt.Sprint(current[k]) != fmt.Sprint(v) {
					writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("%s cannot be changed after creation", k))
					return
				}
				current[k] = v
			}
			raw, _ := json.Marshal(current)
			// Values KubeTRE manages itself are not part of the template's schema.
			chosen := map[string]any{}
			for k, v := range current {
				if !serviceDerivedProps[k] {
					chosen[k] = v
				}
			}
			chosenRaw, _ := json.Marshal(chosen)
			if err := schema.Validate("service-"+tmpl.Name, rawOf(tmpl.Spec.ValuesSchema), chosenRaw); err != nil {
				writeError(w, http.StatusUnprocessableEntity, "properties do not match the template: "+err.Error())
				return
			}
			svc.Spec.Values = &apiextensionsv1.JSON{Raw: raw}
		}
	}
	if err := s.Client.Update(r.Context(), svc); err != nil {
		if apierrors.IsConflict(err) {
			writeError(w, http.StatusConflict, "the resource was changed by someone else; refresh and try again")
			return
		}
		if apierrors.IsInvalid(err) {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		s.internal(w, "update service", err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"operation": newOperation(svc.UID, actionUpdate, svc.Generation, path,
		svc.Name, kind, svc.Spec.TemplateRef, "updating", "", id, s.now())})
}

func (s *Server) remove(w http.ResponseWriter, r *http.Request, id access.Identity, svc *treV1.WorkspaceService, path, kind string) {
	if enabled(svc.Spec.Enabled) {
		writeError(w, http.StatusBadRequest, "the resource must be disabled before it can be deleted")
		return
	}
	uid := svc.UID
	if err := s.Client.Delete(r.Context(), svc, client.Preconditions{UID: &uid}); client.IgnoreNotFound(err) != nil {
		s.internal(w, "delete service", err)
		return
	}
	s.logger().Info("resource deletion requested", "type", kind, "name", svc.Name, "by", id.Principal())
	writeJSON(w, http.StatusAccepted, map[string]any{"operation": newOperation(uid, actionUninstall, svc.Generation, path,
		svc.Name, kind, svc.Spec.TemplateRef, "deleting", "", id, s.now())})
}
