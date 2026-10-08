package tre

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
	"github.com/isBioku/kubetre/internal/access"
	"github.com/isBioku/kubetre/internal/schema"
)

// Workspace properties that map onto the Workspace spec or annotations rather than onto the
// template's parameters. They appear in the workspace template's form.
var workspaceSystemProps = map[string]bool{
	"display_name": true, "description": true, "overview": true,
	"owners": true, "researchers": true, "airlock_managers": true, "allowed_fqdns": true,
}

// Properties KubeTRE derives and returns, never accepted as input.
var workspaceDerivedProps = map[string]bool{
	"scope_id": true, "enable_airlock": true, "vm_subnet": true, "kubernetes_namespace": true,
}

var fqdnPattern = regexp.MustCompile(`^(\*\.)?[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)

func workspacePath(name string) string { return "/workspaces/" + name }

func (s *Server) workspaceJSON(ws *treV1.Workspace, templateVersion string, id access.Identity) resource {
	props := rawMap(rawOf(ws.Spec.Parameters))
	props["display_name"] = ws.Spec.DisplayName
	props["description"] = ws.Spec.Description
	if o := ws.Annotations[annOverview]; o != "" {
		props["overview"] = o
	}
	props["owners"] = toAny(ws.Spec.Owners)
	props["researchers"] = toAny(ws.Spec.Researchers)
	props["airlock_managers"] = toAny(ws.Spec.AirlockManagers)
	props["allowed_fqdns"] = toAny(ws.Spec.Egress.AllowedFQDNs)
	props["scope_id"] = s.APIScope
	props["enable_airlock"] = false
	if ws.Status.Namespace != "" {
		props["kubernetes_namespace"] = ws.Status.Namespace
	}
	if ws.Status.VMNetwork != nil {
		props["vm_subnet"] = ws.Status.VMNetwork.AddressPrefix
	}
	roles := access.WorkspaceRoles(ws, id)
	if roles == nil {
		roles = []string{}
	}
	return resource{
		ID: ws.Name, IsEnabled: enabled(ws.Spec.Enabled), ResourcePath: workspacePath(ws.Name),
		ResourceVersion: ws.Generation, ResourceType: typeWorkspace, TemplateName: ws.Spec.TemplateRef,
		TemplateVersion: templateVersion, AvailableUpgrades: []any{},
		DeploymentStatus: deploymentStatus(ws, ws.Status.Phase, ws.Generation),
		UpdatedWhen:      updatedWhen(ws, ws.Status.Conditions), User: createdBy(ws),
		Etag: ws.ResourceVersion, Properties: props, UserRoles: roles,
	}
}

func rawOf(j *apiextensionsv1.JSON) []byte {
	if j == nil {
		return nil
	}
	return j.Raw
}

func (s *Server) workspaceTemplateVersions(r *http.Request) map[string]string {
	list := &treV1.WorkspaceTemplateList{}
	out := map[string]string{}
	if err := s.Client.List(r.Context(), list); err == nil {
		for _, t := range list.Items {
			out[t.Name] = t.Spec.Version
		}
	}
	return out
}

// loadWorkspace returns the workspace and the caller's roles in it. Callers who are neither
// members nor TRE administrators get 404, so workspace names do not leak.
func (s *Server) loadWorkspace(w http.ResponseWriter, r *http.Request, id access.Identity) (*treV1.Workspace, map[string]bool, bool) {
	ws := &treV1.Workspace{}
	err := s.Client.Get(r.Context(), types.NamespacedName{Name: r.PathValue("ws")}, ws)
	if apierrors.IsNotFound(err) {
		writeError(w, http.StatusNotFound, "workspace not found")
		return nil, nil, false
	}
	if err != nil {
		s.internal(w, "get workspace", err)
		return nil, nil, false
	}
	roles := map[string]bool{}
	for _, role := range access.WorkspaceRoles(ws, id) {
		roles[role] = true
	}
	if len(roles) == 0 && !s.isAdmin(id) {
		writeError(w, http.StatusNotFound, "workspace not found")
		return nil, nil, false
	}
	return ws, roles, true
}

func (s *Server) listWorkspaces(w http.ResponseWriter, r *http.Request, id access.Identity) {
	list := &treV1.WorkspaceList{}
	if err := s.Client.List(r.Context(), list); err != nil {
		s.internal(w, "list workspaces", err)
		return
	}
	versions := s.workspaceTemplateVersions(r)
	admin := s.isAdmin(id)
	out := []resource{}
	for i := range list.Items {
		ws := &list.Items[i]
		if admin || access.IsMember(ws, id) {
			out = append(out, s.workspaceJSON(ws, versions[ws.Spec.TemplateRef], id))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspaces": out})
}

func (s *Server) getWorkspace(w http.ResponseWriter, r *http.Request, id access.Identity) {
	ws, _, ok := s.loadWorkspace(w, r, id)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspace": s.workspaceJSON(ws, s.workspaceTemplateVersions(r)[ws.Spec.TemplateRef], id)})
}

func (s *Server) getScopeID(w http.ResponseWriter, r *http.Request, id access.Identity) {
	if _, _, ok := s.loadWorkspace(w, r, id); !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspaceAuth": map[string]any{"scopeId": s.APIScope}})
}

// splitWorkspaceProps separates spec-level properties from template parameters.
func splitWorkspaceProps(props map[string]any) (system map[string]any, params map[string]any) {
	system, params = map[string]any{}, map[string]any{}
	for k, v := range props {
		switch {
		case workspaceSystemProps[k]:
			system[k] = v
		case workspaceDerivedProps[k]:
		default:
			params[k] = v
		}
	}
	return system, params
}

func validFQDNs(list []string) error {
	for _, f := range list {
		if !fqdnPattern.MatchString(strings.ToLower(f)) {
			return fmt.Errorf("allowed_fqdns: %q is not a hostname", f)
		}
	}
	return nil
}

func (s *Server) createWorkspace(w http.ResponseWriter, r *http.Request, id access.Identity) {
	if !s.isAdmin(id) {
		writeError(w, http.StatusForbidden, "only TRE administrators can create workspaces")
		return
	}
	b, ok := readBody(w, r)
	if !ok {
		return
	}
	tmpl := &treV1.WorkspaceTemplate{}
	if err := s.Client.Get(r.Context(), types.NamespacedName{Name: b.TemplateName}, tmpl); err != nil {
		if apierrors.IsNotFound(err) {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("template %q does not exist", b.TemplateName))
			return
		}
		s.internal(w, "get template", err)
		return
	}
	system, params := splitWorkspaceProps(b.Properties)
	display := strings.TrimSpace(str(system["display_name"]))
	if display == "" {
		writeError(w, http.StatusUnprocessableEntity, "display_name is required")
		return
	}
	rawParams, _ := json.Marshal(params)
	if err := schema.Validate("workspace-"+tmpl.Name, rawOf(tmpl.Spec.ParametersSchema), rawParams); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "properties do not match the template: "+err.Error())
		return
	}
	fqdns := stringList(system["allowed_fqdns"])
	if err := validFQDNs(fqdns); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	owners := stringList(system["owners"])
	if len(owners) == 0 {
		owners = []string{id.Principal()}
	}
	ws := &treV1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: slug(display, 40),
			Annotations:  map[string]string{annCreatedBy: id.Principal()},
		},
		Spec: treV1.WorkspaceSpec{
			DisplayName: display, Description: str(system["description"]), TemplateRef: tmpl.Name,
			Parameters: &apiextensionsv1.JSON{Raw: rawParams}, Owners: owners,
			Researchers: stringList(system["researchers"]), AirlockManagers: stringList(system["airlock_managers"]),
			Egress: treV1.EgressSpec{AllowedFQDNs: fqdns},
		},
	}
	if o := str(system["overview"]); o != "" {
		ws.Annotations[annOverview] = o
	}
	if err := s.Client.Create(r.Context(), ws); err != nil {
		if apierrors.IsInvalid(err) {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		s.internal(w, "create workspace", err)
		return
	}
	s.logger().Info("workspace created", "workspace", ws.Name, "by", id.Principal())
	writeJSON(w, http.StatusAccepted, map[string]any{"operation": newOperation(ws.UID, actionInstall, 1,
		workspacePath(ws.Name), ws.Name, typeWorkspace, tmpl.Name, "awaiting_deployment", "", id, s.now())})
}

func (s *Server) patchWorkspace(w http.ResponseWriter, r *http.Request, id access.Identity) {
	if !s.isAdmin(id) {
		writeError(w, http.StatusForbidden, "only TRE administrators can update workspaces")
		return
	}
	ws, _, ok := s.loadWorkspace(w, r, id)
	if !ok {
		return
	}
	if !etagMatches(r, ws.ResourceVersion) {
		writeError(w, http.StatusConflict, "the workspace was changed by someone else; refresh and try again")
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
		ws.Spec.Enabled = &v
	}
	if len(b.Properties) > 0 {
		system, params := splitWorkspaceProps(b.Properties)
		if v, ok := system["display_name"]; ok && strings.TrimSpace(str(v)) != "" {
			ws.Spec.DisplayName = strings.TrimSpace(str(v))
		}
		if v, ok := system["description"]; ok {
			ws.Spec.Description = str(v)
		}
		if v, ok := system["overview"]; ok {
			if ws.Annotations == nil {
				ws.Annotations = map[string]string{}
			}
			ws.Annotations[annOverview] = str(v)
		}
		if v, ok := system["owners"]; ok {
			if owners := stringList(v); len(owners) > 0 {
				ws.Spec.Owners = owners
			}
		}
		if v, ok := system["researchers"]; ok {
			ws.Spec.Researchers = stringList(v)
		}
		if v, ok := system["airlock_managers"]; ok {
			ws.Spec.AirlockManagers = stringList(v)
		}
		if v, ok := system["allowed_fqdns"]; ok {
			fqdns := stringList(v)
			if err := validFQDNs(fqdns); err != nil {
				writeError(w, http.StatusUnprocessableEntity, err.Error())
				return
			}
			ws.Spec.Egress.AllowedFQDNs = fqdns
		}
		merged := rawMap(rawOf(ws.Spec.Parameters))
		for k, v := range params {
			merged[k] = v
		}
		raw, _ := json.Marshal(merged)
		tmpl := &treV1.WorkspaceTemplate{}
		if err := s.Client.Get(r.Context(), types.NamespacedName{Name: ws.Spec.TemplateRef}, tmpl); err == nil {
			if err := schema.Validate("workspace-"+tmpl.Name, rawOf(tmpl.Spec.ParametersSchema), raw); err != nil {
				writeError(w, http.StatusUnprocessableEntity, "properties do not match the template: "+err.Error())
				return
			}
		}
		ws.Spec.Parameters = &apiextensionsv1.JSON{Raw: raw}
	}
	if err := s.Client.Update(r.Context(), ws); err != nil {
		if apierrors.IsConflict(err) {
			writeError(w, http.StatusConflict, "the workspace was changed by someone else; refresh and try again")
			return
		}
		s.internal(w, "update workspace", err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"operation": newOperation(ws.UID, actionUpdate, ws.Generation,
		workspacePath(ws.Name), ws.Name, typeWorkspace, ws.Spec.TemplateRef, "updating", "", id, s.now())})
}

func (s *Server) deleteWorkspace(w http.ResponseWriter, r *http.Request, id access.Identity) {
	if !s.isAdmin(id) {
		writeError(w, http.StatusForbidden, "only TRE administrators can delete workspaces")
		return
	}
	ws, _, ok := s.loadWorkspace(w, r, id)
	if !ok {
		return
	}
	if enabled(ws.Spec.Enabled) {
		writeError(w, http.StatusBadRequest, "the workspace must be disabled before it can be deleted")
		return
	}
	uid := ws.UID
	if err := s.Client.Delete(r.Context(), ws, client.Preconditions{UID: &uid}); client.IgnoreNotFound(err) != nil {
		s.internal(w, "delete workspace", err)
		return
	}
	s.logger().Info("workspace deletion requested", "workspace", ws.Name, "by", id.Principal())
	writeJSON(w, http.StatusAccepted, map[string]any{"operation": newOperation(uid, actionUninstall, ws.Generation,
		workspacePath(ws.Name), ws.Name, typeWorkspace, ws.Spec.TemplateRef, "deleting", "", id, s.now())})
}
