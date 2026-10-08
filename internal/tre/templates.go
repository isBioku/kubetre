package tre

import (
	"encoding/json"
	"net/http"
	"sort"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
	"github.com/isBioku/kubetre/internal/access"
)

// templateJSON is AzureTRE's resource template: a JSON Schema plus metadata, which the UI
// renders as the create and update form.
type templateJSON struct {
	Schema           string         `json:"$schema"`
	SchemaID         string         `json:"$id"`
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Type             string         `json:"type"`
	Title            string         `json:"title"`
	Description      string         `json:"description"`
	Version          string         `json:"version"`
	ResourceType     string         `json:"resourceType"`
	Current          bool           `json:"current"`
	Required         []string       `json:"required"`
	Properties       map[string]any `json:"properties"`
	SystemProperties map[string]any `json:"system_properties"`
	Actions          []any          `json:"actions"`
	CustomActions    []any          `json:"customActions"`
	UISchema         map[string]any `json:"uiSchema"`
	Pipeline         any            `json:"pipeline"`
	ParentTemplate   string         `json:"parentWorkspaceService,omitempty"`
}

// templateSummary is an entry in AzureTRE's template lists.
type templateSummary struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	ResourceType string `json:"resourceType"`
	Version      string `json:"version"`
	Current      bool   `json:"current"`
}

func prop(title, description string, extra map[string]any) map[string]any {
	p := map[string]any{"type": "string", "title": title, "description": description, "updateable": true}
	for k, v := range extra {
		p[k] = v
	}
	return p
}

func stringArray(title, description string) map[string]any {
	return map[string]any{"type": "array", "title": title, "description": description, "updateable": true,
		"items": map[string]any{"type": "string"}}
}

// Workspace fields every KubeTRE workspace template has. AzureTRE assigns workspace roles
// through Entra app roles; KubeTRE keeps them on the workspace, so they are part of the form.
func workspaceSystemSchema() (map[string]any, []string) {
	return map[string]any{
		"display_name": prop("Name for the workspace", "The name of the workspace to be displayed to users", nil),
		"description":  prop("Description of the workspace", "Description of the workspace", nil),
		"overview": prop("Workspace Overview",
			"Long form description of the workspace, in markdown syntax. Displayed on the workspace overview page.", nil),
		"owners": stringArray("Workspace owners",
			"Email addresses of the people who manage this workspace. Defaults to you."),
		"researchers": stringArray("Workspace researchers",
			"Email addresses of the people who work in this workspace."),
		"allowed_fqdns": stringArray("Allowed internet domains",
			"Extra hostnames this workspace may reach on HTTPS, for example pypi.org. A leading *. allows subdomains."),
	}, []string{"display_name", "description"}
}

func serviceSystemSchema(kind string) (map[string]any, []string) {
	noun := "workspace service"
	if kind == typeUserResource {
		noun = "resource"
	}
	return map[string]any{
		"display_name": prop("Name for the "+noun, "The name of the "+noun+" to be displayed to users", nil),
		"description":  prop("Description of the "+noun, "Description of the "+noun, nil),
		"overview":     prop("Overview", "Long form description of the "+noun+", in markdown syntax.", nil),
	}, []string{"display_name", "description"}
}

// buildTemplate merges the system fields with the parameters schema. For an update form,
// fields not marked "updateable" are read-only, as in AzureTRE.
func buildTemplate(name, title, description, version, resourceType string, params []byte,
	system map[string]any, required []string, isUpdate bool) templateJSON {
	props := map[string]any{}
	for k, v := range system {
		props[k] = v
	}
	var sch struct {
		Properties map[string]map[string]any `json:"properties"`
		Required   []string                  `json:"required"`
	}
	if len(params) > 0 {
		_ = json.Unmarshal(params, &sch)
	}
	for k, v := range sch.Properties {
		if isUpdate {
			if u, _ := v["updateable"].(bool); !u {
				v["readOnly"] = true
			}
		}
		props[k] = v
	}
	order := []string{"display_name", "description", "overview"}
	var rest []string
	for k := range props {
		if k != "display_name" && k != "description" && k != "overview" {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	return templateJSON{
		Schema: "http://json-schema.org/draft-07/schema", SchemaID: "https://kubetre.io/templates/" + name + ".json",
		ID: name, Name: name, Type: "object", Title: title, Description: description, Version: version,
		ResourceType: resourceType, Current: true, Required: append(required, sch.Required...), Properties: props,
		SystemProperties: map[string]any{}, Actions: []any{}, CustomActions: []any{},
		UISchema: map[string]any{"ui:order": append(order, rest...)},
	}
}

func isUpdate(r *http.Request) bool { return r.URL.Query().Get("is_update") == "true" }

func workspaceTemplateJSON(t *treV1.WorkspaceTemplate, update bool) templateJSON {
	system, required := workspaceSystemSchema()
	return buildTemplate(t.Name, t.Spec.DisplayName, t.Spec.Description, t.Spec.Version, typeWorkspace,
		rawOf(t.Spec.ParametersSchema), system, required, update)
}

func serviceTemplateJSON(t *treV1.ServiceTemplate, update bool) templateJSON {
	kind := typeWorkspaceService
	if t.IsUserResource() {
		kind = typeUserResource
	}
	system, required := serviceSystemSchema(kind)
	out := buildTemplate(t.Name, t.Spec.DisplayName, t.Spec.Description, templateVersion(t), kind,
		rawOf(t.Spec.ValuesSchema), system, required, update)
	out.ParentTemplate = t.Spec.ParentTemplate
	return out
}

// updateable reports whether a template parameter may change after creation.
func updateable(t *treV1.ServiceTemplate, key string) bool {
	var sch struct {
		Properties map[string]map[string]any `json:"properties"`
	}
	_ = json.Unmarshal(rawOf(t.Spec.ValuesSchema), &sch)
	u, _ := sch.Properties[key]["updateable"].(bool)
	return u
}

func (s *Server) listWorkspaceTemplates(w http.ResponseWriter, r *http.Request, _ access.Identity) {
	list := &treV1.WorkspaceTemplateList{}
	if err := s.Client.List(r.Context(), list); err != nil {
		s.internal(w, "list templates", err)
		return
	}
	out := []templateSummary{}
	for _, t := range list.Items {
		out = append(out, templateSummary{ID: t.Name, Name: t.Name, Title: t.Spec.DisplayName, Description: t.Spec.Description,
			ResourceType: typeWorkspace, Version: t.Spec.Version, Current: true})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	writeJSON(w, http.StatusOK, map[string]any{"templates": out})
}

func (s *Server) getWorkspaceTemplate(w http.ResponseWriter, r *http.Request, _ access.Identity) {
	t := &treV1.WorkspaceTemplate{}
	if err := s.Client.Get(r.Context(), types.NamespacedName{Name: r.PathValue("name")}, t); err != nil {
		if apierrors.IsNotFound(err) {
			writeError(w, http.StatusNotFound, "template not found")
			return
		}
		s.internal(w, "get template", err)
		return
	}
	writeJSON(w, http.StatusOK, workspaceTemplateJSON(t, isUpdate(r)))
}

func (s *Server) serviceTemplateList(r *http.Request, keep func(*treV1.ServiceTemplate) bool) ([]templateSummary, error) {
	list := &treV1.ServiceTemplateList{}
	if err := s.Client.List(r.Context(), list); err != nil {
		return nil, err
	}
	out := []templateSummary{}
	for i := range list.Items {
		t := &list.Items[i]
		if !keep(t) {
			continue
		}
		kind := typeWorkspaceService
		if t.IsUserResource() {
			kind = typeUserResource
		}
		out = append(out, templateSummary{ID: t.Name, Name: t.Name, Title: t.Spec.DisplayName, Description: t.Spec.Description,
			ResourceType: kind, Version: templateVersion(t), Current: true})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out, nil
}

func (s *Server) listServiceTemplates(w http.ResponseWriter, r *http.Request, _ access.Identity) {
	out, err := s.serviceTemplateList(r, func(t *treV1.ServiceTemplate) bool { return !t.IsUserResource() })
	if err != nil {
		s.internal(w, "list templates", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": out})
}

func (s *Server) getServiceTemplate(w http.ResponseWriter, r *http.Request, _ access.Identity) {
	t, ok := s.loadServiceTemplate(w, r, r.PathValue("name"))
	if !ok {
		return
	}
	if t.IsUserResource() {
		writeError(w, http.StatusNotFound, "template not found")
		return
	}
	writeJSON(w, http.StatusOK, serviceTemplateJSON(t, isUpdate(r)))
}

// listUserResourceTemplates lists the user resource templates of a workspace service
// template that can be used in this workspace.
func (s *Server) listUserResourceTemplates(w http.ResponseWriter, r *http.Request, id access.Identity) {
	ws, _, ok := s.loadWorkspace(w, r, id)
	if !ok {
		return
	}
	parent := r.PathValue("parent")
	vms := ws.Status.VMNetwork != nil
	out, err := s.serviceTemplateList(r, func(t *treV1.ServiceTemplate) bool {
		return t.IsUserResource() && t.Spec.ParentTemplate == parent && (vms || !t.Spec.RequiresVirtualMachines)
	})
	if err != nil {
		s.internal(w, "list templates", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": out})
}

func (s *Server) getUserResourceTemplate(w http.ResponseWriter, r *http.Request, _ access.Identity) {
	t, ok := s.loadServiceTemplate(w, r, r.PathValue("name"))
	if !ok {
		return
	}
	if !t.IsUserResource() || t.Spec.ParentTemplate != r.PathValue("parent") {
		writeError(w, http.StatusNotFound, "template not found")
		return
	}
	writeJSON(w, http.StatusOK, serviceTemplateJSON(t, isUpdate(r)))
}

func (s *Server) loadServiceTemplate(w http.ResponseWriter, r *http.Request, name string) (*treV1.ServiceTemplate, bool) {
	t := &treV1.ServiceTemplate{}
	if err := s.Client.Get(r.Context(), types.NamespacedName{Name: name}, t); err != nil {
		if apierrors.IsNotFound(err) {
			writeError(w, http.StatusNotFound, "template not found")
			return nil, false
		}
		s.internal(w, "get template", err)
		return nil, false
	}
	return t, true
}

func (s *Server) emptyTemplates(w http.ResponseWriter, _ *http.Request, _ access.Identity) {
	writeJSON(w, http.StatusOK, map[string]any{"templates": []any{}})
}
