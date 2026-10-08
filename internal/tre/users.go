package tre

import (
	"net/http"
	"sort"

	"github.com/isBioku/kubetre/internal/access"
)

type roleJSON struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

var workspaceRoleList = []roleJSON{
	{ID: access.RoleOwner, DisplayName: "Workspace Owner"},
	{ID: access.RoleResearcher, DisplayName: "Workspace Researcher"},
	{ID: access.RoleAirlockManager, DisplayName: "Airlock Manager"},
}

// listWorkspaceUsers returns the workspace's members in AzureTRE's shape. Members are
// recorded by email address, so that is also their ID here.
func (s *Server) listWorkspaceUsers(w http.ResponseWriter, r *http.Request, id access.Identity) {
	ws, roles, ok := s.loadWorkspace(w, r, id)
	if !ok {
		return
	}
	if !roles[access.RoleOwner] && !s.isAdmin(id) {
		writeError(w, http.StatusForbidden, "only workspace owners can list users")
		return
	}
	type user struct {
		ID                string     `json:"id"`
		DisplayName       string     `json:"displayName"`
		UserPrincipalName string     `json:"userPrincipalName"`
		Email             string     `json:"email"`
		Roles             []roleJSON `json:"roles"`
	}
	byID := map[string]*user{}
	add := func(members []string, role roleJSON) {
		for _, m := range members {
			u, ok := byID[m]
			if !ok {
				u = &user{ID: m, DisplayName: m, UserPrincipalName: m, Email: m}
				byID[m] = u
			}
			u.Roles = append(u.Roles, role)
		}
	}
	add(ws.Spec.Owners, workspaceRoleList[0])
	add(ws.Spec.Researchers, workspaceRoleList[1])
	add(ws.Spec.AirlockManagers, workspaceRoleList[2])
	out := make([]user, 0, len(byID))
	for _, u := range byID {
		out = append(out, *u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	writeJSON(w, http.StatusOK, map[string]any{"users": out})
}

func (s *Server) listWorkspaceRoles(w http.ResponseWriter, r *http.Request, id access.Identity) {
	if _, _, ok := s.loadWorkspace(w, r, id); !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"roles": workspaceRoleList})
}

// Airlock is not implemented in KubeTRE yet; workspaces report enable_airlock=false, so the
// UI hides it, and any direct request sees an empty list.
func (s *Server) listAirlockRequests(w http.ResponseWriter, r *http.Request, id access.Identity) {
	if _, _, ok := s.loadWorkspace(w, r, id); !ok {
		return
	}
	writeJSON(w, http.StatusOK, []any{})
}
