// Package tre serves AzureTRE's REST API over KubeTRE's Kubernetes resources, so that
// AzureTRE's own user interface runs unchanged against KubeTRE. Paths, payloads and
// envelopes follow AzureTRE's API (/api/workspaces, /workspace-services, /user-resources,
// templates and operations); authorization follows AzureTRE's roles.
package tre

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/isBioku/kubetre/internal/access"
	"github.com/isBioku/kubetre/internal/api"
)

// Server is the AzureTRE-compatible API.
type Server struct {
	Client    client.Client
	Auth      api.Authenticator
	AdminRole string
	// APIScope is the API's application ID URI (api://<client-id>). It is returned as every
	// workspace's scope_id: KubeTRE uses one audience for all workspaces.
	APIScope string
	// GatewayURL is the access gateway's base URL, used to build connection_uri.
	GatewayURL string
	Version    string
	Log        *slog.Logger
	Now        func() time.Time
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Server) logger() *slog.Logger {
	if s.Log == nil {
		return slog.Default()
	}
	return s.Log
}

func (s *Server) isAdmin(id access.Identity) bool {
	role := s.AdminRole
	if role == "" {
		role = api.DefaultAdminRole
	}
	return id.HasRole(role)
}

// Handler returns AzureTRE's API routes.
func (s *Server) Handler() http.Handler {
	m := http.NewServeMux()
	h := func(pattern string, fn authed) { m.HandleFunc(pattern, s.authed(fn)) }

	m.HandleFunc("GET /api/.metadata", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"api_version": s.Version})
	})
	m.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"services": []map[string]string{
			{"service": "Kubernetes API", "status": "OK", "message": ""},
		}})
	})

	// Workspaces.
	h("GET /api/workspaces", s.listWorkspaces)
	h("POST /api/workspaces", s.createWorkspace)
	h("GET /api/workspaces/{ws}", s.getWorkspace)
	h("PATCH /api/workspaces/{ws}", s.patchWorkspace)
	h("DELETE /api/workspaces/{ws}", s.deleteWorkspace)
	h("GET /api/workspaces/{ws}/scopeid", s.getScopeID)

	// Workspace services.
	h("GET /api/workspaces/{ws}/workspace-services", s.listWorkspaceServices)
	h("POST /api/workspaces/{ws}/workspace-services", s.createWorkspaceService)
	h("GET /api/workspaces/{ws}/workspace-services/{svc}", s.getWorkspaceService)
	h("PATCH /api/workspaces/{ws}/workspace-services/{svc}", s.patchWorkspaceService)
	h("DELETE /api/workspaces/{ws}/workspace-services/{svc}", s.deleteWorkspaceService)

	// User resources.
	base := "/api/workspaces/{ws}/workspace-services/{svc}/user-resources"
	h("GET "+base, s.listUserResources)
	h("POST "+base, s.createUserResource)
	h("GET "+base+"/{ur}", s.getUserResource)
	h("PATCH "+base+"/{ur}", s.patchUserResource)
	h("DELETE "+base+"/{ur}", s.deleteUserResource)

	// Operations and history, at every level of the hierarchy.
	for _, p := range []string{"/api/workspaces/{ws}", "/api/workspaces/{ws}/workspace-services/{svc}", base + "/{ur}"} {
		h("GET "+p+"/operations", s.listOperations)
		h("GET "+p+"/operations/{op}", s.getOperation)
		h("GET "+p+"/history", s.getHistory)
	}
	h("GET /api/operations", s.myOperations)

	// Templates.
	h("GET /api/workspace-templates", s.listWorkspaceTemplates)
	h("GET /api/workspace-templates/{name}", s.getWorkspaceTemplate)
	h("GET /api/workspace-service-templates", s.listServiceTemplates)
	h("GET /api/workspace-service-templates/{name}", s.getServiceTemplate)
	h("GET /api/workspaces/{ws}/workspace-service-templates/{parent}/user-resource-templates", s.listUserResourceTemplates)
	h("GET /api/workspace-service-templates/{parent}/user-resource-templates/{name}", s.getUserResourceTemplate)
	h("GET /api/shared-service-templates", s.emptyTemplates)

	// Features KubeTRE does not have yet. The UI already handles these responses.
	h("GET /api/shared-services", func(w http.ResponseWriter, _ *http.Request, _ access.Identity) {
		writeJSON(w, http.StatusOK, map[string]any{"sharedServices": []any{}})
	})
	h("GET /api/costs", notSupported)
	h("GET /api/workspaces/{ws}/costs", notSupported)
	h("GET /api/requests", func(w http.ResponseWriter, _ *http.Request, _ access.Identity) {
		writeJSON(w, http.StatusOK, []any{})
	})
	h("GET /api/workspaces/{ws}/requests", s.listAirlockRequests)
	h("GET /api/workspaces/{ws}/users", s.listWorkspaceUsers)
	h("GET /api/workspaces/{ws}/roles", s.listWorkspaceRoles)
	return m
}

type authed func(w http.ResponseWriter, r *http.Request, id access.Identity)

func (s *Server) authed(fn authed) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := s.Auth.Authenticate(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		fn(w, r, id)
	}
}

func notSupported(w http.ResponseWriter, _ *http.Request, _ access.Identity) {
	writeError(w, http.StatusNotFound, "not supported by KubeTRE yet")
}

func (s *Server) internal(w http.ResponseWriter, op string, err error) {
	s.logger().Error("request failed", "op", op, "error", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

// writeError uses AzureTRE's error body ({"detail": ...}).
func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"detail": msg})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// requestBody is AzureTRE's create and update payload.
type requestBody struct {
	TemplateName    string         `json:"templateName"`
	TemplateVersion string         `json:"templateVersion"`
	Properties      map[string]any `json:"properties"`
	IsEnabled       *bool          `json:"isEnabled"`
}

func readBody(w http.ResponseWriter, r *http.Request) (*requestBody, bool) {
	var b requestBody
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return nil, false
	}
	if b.Properties == nil {
		b.Properties = map[string]any{}
	}
	return &b, true
}

// etagMatches implements AzureTRE's optimistic concurrency: an empty etag header is accepted.
func etagMatches(r *http.Request, resourceVersion string) bool {
	e := strings.Trim(r.Header.Get("etag"), `"`)
	return e == "" || e == resourceVersion
}
