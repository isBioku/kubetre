package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/santhosh-tekuri/jsonschema/v6"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
	"github.com/isBioku/kubetre/internal/access"
	"github.com/isBioku/kubetre/internal/controller"
)

// DefaultAdminRole matches the TRE administrator role name used by AzureTRE.
const DefaultAdminRole = "TREAdmin"

const maxBodyBytes = 1 << 20

// Server serves the KubeTRE API. Kubernetes is the system of record: the API
// validates intent and writes custom resources; controllers do the work.
type Server struct {
	Client    client.Client
	Auth      Authenticator
	AdminRole string
	Log       *slog.Logger
}

// Handler returns the HTTP routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /api/v1/me", s.authed(s.me))
	mux.HandleFunc("GET /api/v1/templates", s.authed(s.listTemplates))
	mux.HandleFunc("GET /api/v1/workspaces", s.authed(s.listWorkspaces))
	mux.HandleFunc("POST /api/v1/workspaces", s.authed(s.createWorkspace))
	mux.HandleFunc("GET /api/v1/workspaces/{name}", s.authed(s.getWorkspace))
	mux.HandleFunc("DELETE /api/v1/workspaces/{name}", s.authed(s.deleteWorkspace))
	mux.HandleFunc("GET /api/v1/service-templates", s.authed(s.listServiceTemplates))
	mux.HandleFunc("GET /api/v1/workspaces/{name}/services", s.authed(s.listServices))
	mux.HandleFunc("POST /api/v1/workspaces/{name}/services", s.authed(s.createService))
	mux.HandleFunc("GET /api/v1/workspaces/{name}/services/{service}", s.authed(s.getService))
	mux.HandleFunc("DELETE /api/v1/workspaces/{name}/services/{service}", s.authed(s.deleteService))
	return mux
}

type authedHandler func(w http.ResponseWriter, r *http.Request, id Identity)

func (s *Server) authed(h authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := s.Auth.Authenticate(r)
		if err != nil {
			s.logger().Debug("authentication failed", "error", err)
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		h(w, r, id)
	}
}

func (s *Server) logger() *slog.Logger {
	if s.Log == nil {
		return slog.Default()
	}
	return s.Log
}

func (s *Server) isAdmin(id Identity) bool {
	role := s.AdminRole
	if role == "" {
		role = DefaultAdminRole
	}
	return id.HasRole(role)
}

// Workspace roles, mirroring AzureTRE.
const (
	RoleOwner          = access.RoleOwner
	RoleResearcher     = access.RoleResearcher
	RoleAirlockManager = access.RoleAirlockManager
)

func workspaceRoles(ws *treV1.Workspace, id Identity) []string { return access.WorkspaceRoles(ws, id) }

// ---- views ----

type templateView struct {
	Name             string          `json:"name"`
	DisplayName      string          `json:"displayName"`
	Description      string          `json:"description,omitempty"`
	Version          string          `json:"version"`
	ParametersSchema json.RawMessage `json:"parametersSchema,omitempty"`
}

type workspaceView struct {
	Name            string             `json:"name"`
	DisplayName     string             `json:"displayName"`
	Description     string             `json:"description,omitempty"`
	TemplateRef     string             `json:"templateRef"`
	Parameters      json.RawMessage    `json:"parameters,omitempty"`
	Owners          []string           `json:"owners"`
	Researchers     []string           `json:"researchers,omitempty"`
	AirlockManagers []string           `json:"airlockManagers,omitempty"`
	AllowedFQDNs    []string           `json:"allowedFQDNs,omitempty"`
	Phase           string             `json:"phase"`
	Namespace       string             `json:"namespace,omitempty"`
	Conditions      []metav1.Condition `json:"conditions,omitempty"`
	YourRoles       []string           `json:"yourRoles"`
}

func toWorkspaceView(ws *treV1.Workspace, id Identity) workspaceView {
	v := workspaceView{
		Name: ws.Name, DisplayName: ws.Spec.DisplayName, Description: ws.Spec.Description,
		TemplateRef: ws.Spec.TemplateRef, Owners: ws.Spec.Owners, Researchers: ws.Spec.Researchers,
		AirlockManagers: ws.Spec.AirlockManagers, AllowedFQDNs: ws.Spec.Egress.AllowedFQDNs,
		Phase: ws.Status.Phase, Namespace: ws.Status.Namespace, Conditions: ws.Status.Conditions,
		YourRoles: workspaceRoles(ws, id),
	}
	if v.Phase == "" {
		v.Phase = treV1.PhasePending
	}
	if v.YourRoles == nil {
		v.YourRoles = []string{}
	}
	if ws.Spec.Parameters != nil {
		v.Parameters = ws.Spec.Parameters.Raw
	}
	return v
}

// ---- handlers ----

func (s *Server) me(w http.ResponseWriter, _ *http.Request, id Identity) {
	writeJSON(w, http.StatusOK, map[string]any{
		"subject": id.Subject, "email": id.Email, "name": id.Name, "roles": id.Roles, "isAdmin": s.isAdmin(id),
	})
}

func (s *Server) listTemplates(w http.ResponseWriter, r *http.Request, _ Identity) {
	list := &treV1.WorkspaceTemplateList{}
	if err := s.Client.List(r.Context(), list); err != nil {
		s.internal(w, "list templates", err)
		return
	}
	out := make([]templateView, 0, len(list.Items))
	for _, t := range list.Items {
		v := templateView{Name: t.Name, DisplayName: t.Spec.DisplayName, Description: t.Spec.Description, Version: t.Spec.Version}
		if t.Spec.ParametersSchema != nil {
			v.ParametersSchema = t.Spec.ParametersSchema.Raw
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": out})
}

func (s *Server) listWorkspaces(w http.ResponseWriter, r *http.Request, id Identity) {
	list := &treV1.WorkspaceList{}
	if err := s.Client.List(r.Context(), list); err != nil {
		s.internal(w, "list workspaces", err)
		return
	}
	admin := s.isAdmin(id)
	out := []workspaceView{}
	for i := range list.Items {
		ws := &list.Items[i]
		if admin || len(workspaceRoles(ws, id)) > 0 {
			out = append(out, toWorkspaceView(ws, id))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspaces": out})
}

// getWorkspace returns 404 to non-members so workspace names do not leak.
func (s *Server) getWorkspace(w http.ResponseWriter, r *http.Request, id Identity) {
	ws, ok := s.loadWorkspace(w, r)
	if !ok {
		return
	}
	if !s.isAdmin(id) && len(workspaceRoles(ws, id)) == 0 {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	writeJSON(w, http.StatusOK, toWorkspaceView(ws, id))
}

type createWorkspaceRequest struct {
	Name            string          `json:"name"`
	DisplayName     string          `json:"displayName"`
	Description     string          `json:"description"`
	TemplateRef     string          `json:"templateRef"`
	Parameters      json.RawMessage `json:"parameters"`
	Owners          []string        `json:"owners"`
	Researchers     []string        `json:"researchers"`
	AirlockManagers []string        `json:"airlockManagers"`
	AllowedFQDNs    []string        `json:"allowedFQDNs"`
}

func (s *Server) createWorkspace(w http.ResponseWriter, r *http.Request, id Identity) {
	if !s.isAdmin(id) {
		writeError(w, http.StatusForbidden, "only TRE administrators can create workspaces")
		return
	}
	var req createWorkspaceRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if problems := validateCreate(&req); len(problems) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "invalid workspace", "details": problems})
		return
	}
	if len(req.Owners) == 0 {
		req.Owners = []string{id.Principal()}
	}

	tmpl := &treV1.WorkspaceTemplate{}
	if err := s.Client.Get(r.Context(), types.NamespacedName{Name: req.TemplateRef}, tmpl); err != nil {
		if apierrors.IsNotFound(err) {
			writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("template %q does not exist", req.TemplateRef))
			return
		}
		s.internal(w, "get template", err)
		return
	}

	params := normalizeObject(req.Parameters)
	if err := validateParameters(tmpl, params); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "parameters do not match the template schema", "details": []string{err.Error()}})
		return
	}

	ws := &treV1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Name,
			Annotations: map[string]string{"kubetre.io/created-by": id.Principal()},
		},
		Spec: treV1.WorkspaceSpec{
			DisplayName: req.DisplayName, Description: req.Description, TemplateRef: req.TemplateRef,
			Parameters: &apiextensionsv1.JSON{Raw: params}, Owners: dedupe(req.Owners),
			Researchers: dedupe(req.Researchers), AirlockManagers: dedupe(req.AirlockManagers),
			Egress: treV1.EgressSpec{AllowedFQDNs: dedupe(req.AllowedFQDNs)},
		},
	}
	if err := s.Client.Create(r.Context(), ws); err != nil {
		switch {
		case apierrors.IsAlreadyExists(err):
			writeError(w, http.StatusConflict, fmt.Sprintf("workspace %q already exists", req.Name))
		case apierrors.IsInvalid(err):
			writeError(w, http.StatusUnprocessableEntity, err.Error())
		default:
			s.internal(w, "create workspace", err)
		}
		return
	}
	s.logger().Info("workspace created", "workspace", ws.Name, "by", id.Principal())
	w.Header().Set("Location", "/api/v1/workspaces/"+ws.Name)
	writeJSON(w, http.StatusAccepted, toWorkspaceView(ws, id))
}

func (s *Server) deleteWorkspace(w http.ResponseWriter, r *http.Request, id Identity) {
	if !s.isAdmin(id) {
		writeError(w, http.StatusForbidden, "only TRE administrators can delete workspaces")
		return
	}
	ws, ok := s.loadWorkspace(w, r)
	if !ok {
		return
	}
	if err := s.Client.Delete(r.Context(), ws); client.IgnoreNotFound(err) != nil {
		s.internal(w, "delete workspace", err)
		return
	}
	s.logger().Info("workspace deletion requested", "workspace", ws.Name, "by", id.Principal())
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) loadWorkspace(w http.ResponseWriter, r *http.Request) (*treV1.Workspace, bool) {
	ws := &treV1.Workspace{}
	err := s.Client.Get(r.Context(), types.NamespacedName{Name: r.PathValue("name")}, ws)
	switch {
	case apierrors.IsNotFound(err):
		writeError(w, http.StatusNotFound, "workspace not found")
		return nil, false
	case err != nil:
		s.internal(w, "get workspace", err)
		return nil, false
	}
	return ws, true
}

// ---- validation ----

func validateCreate(req *createWorkspaceRequest) []string {
	var problems []string
	maxName := validation.DNS1123LabelMaxLength - len(controller.NamespacePrefix)
	if errs := validation.IsDNS1123Label(req.Name); len(errs) > 0 {
		problems = append(problems, "name: "+strings.Join(errs, "; "))
	} else if len(req.Name) > maxName {
		problems = append(problems, fmt.Sprintf("name: must be at most %d characters", maxName))
	}
	if strings.TrimSpace(req.DisplayName) == "" {
		problems = append(problems, "displayName: required")
	}
	if strings.TrimSpace(req.TemplateRef) == "" {
		problems = append(problems, "templateRef: required")
	}
	for _, f := range req.AllowedFQDNs {
		host := strings.TrimPrefix(f, "*.")
		if errs := validation.IsDNS1123Subdomain(strings.ToLower(host)); len(errs) > 0 || !strings.Contains(host, ".") {
			problems = append(problems, fmt.Sprintf("allowedFQDNs: %q is not a hostname", f))
		}
	}
	return problems
}

// validateParameters checks workspace parameters against the template's JSON Schema.
func validateParameters(tmpl *treV1.WorkspaceTemplate, params json.RawMessage) error {
	var schema []byte
	if tmpl.Spec.ParametersSchema != nil {
		schema = tmpl.Spec.ParametersSchema.Raw
	}
	return validateAgainstSchema("workspace-"+tmpl.Name, schema, params)
}

// validateAgainstSchema validates a JSON object against an optional JSON Schema.
// Remote and file references are disabled so a schema cannot make the API fetch anything.
func validateAgainstSchema(id string, schema []byte, doc json.RawMessage) error {
	var obj map[string]any
	if err := json.Unmarshal(doc, &obj); err != nil || obj == nil {
		return errNotObject
	}
	if len(bytes.TrimSpace(schema)) == 0 {
		return nil
	}
	schemaDoc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema))
	if err != nil {
		return fmt.Errorf("template schema is not valid JSON: %w", err)
	}
	c := jsonschema.NewCompiler()
	c.UseLoader(jsonschema.SchemeURLLoader{})
	url := "kubetre://templates/" + id + ".json"
	if err := c.AddResource(url, schemaDoc); err != nil {
		return fmt.Errorf("template schema: %w", err)
	}
	sch, err := c.Compile(url)
	if err != nil {
		return fmt.Errorf("template schema does not compile: %w", err)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(doc))
	if err != nil {
		return errors.New("must be valid JSON")
	}
	return sch.Validate(inst)
}

// normalizeObject turns an empty or null body field into an empty JSON object.
func normalizeObject(raw json.RawMessage) json.RawMessage {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 || string(t) == "null" {
		return json.RawMessage(`{}`)
	}
	return raw
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v != "" && !seen[strings.ToLower(v)] {
			seen[strings.ToLower(v)] = true
			out = append(out, v)
		}
	}
	return out
}

// ---- responses ----

func (s *Server) internal(w http.ResponseWriter, op string, err error) {
	s.logger().Error("request failed", "op", op, "error", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
