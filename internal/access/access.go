// Package access holds the identity and workspace-membership rules shared by the API and
// the access gateway, so both make the same authorization decisions.
package access

import (
	"strings"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
)

// Workspace roles, mirroring AzureTRE.
const (
	RoleOwner          = "WorkspaceOwner"
	RoleResearcher     = "WorkspaceResearcher"
	RoleAirlockManager = "AirlockManager"
)

// Identity is an authenticated user.
type Identity struct {
	Subject string
	Email   string
	Name    string
	Roles   []string
}

// HasRole reports whether the user holds a platform role.
func (id Identity) HasRole(role string) bool {
	for _, r := range id.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// Matches reports whether a membership entry refers to this user.
// Entries are OIDC subjects or email addresses; emails compare case-insensitively.
func (id Identity) Matches(entry string) bool {
	if entry == "" {
		return false
	}
	return entry == id.Subject || (id.Email != "" && strings.EqualFold(entry, id.Email))
}

// Principal is how the user is recorded in membership lists and as a service owner.
func (id Identity) Principal() string {
	if id.Email != "" {
		return strings.ToLower(id.Email)
	}
	return id.Subject
}

// WorkspaceRoles returns the user's roles in a workspace, sorted.
func WorkspaceRoles(ws *treV1.Workspace, id Identity) []string {
	var roles []string
	for _, rm := range []struct {
		role    string
		members []string
	}{
		{RoleAirlockManager, ws.Spec.AirlockManagers},
		{RoleOwner, ws.Spec.Owners},
		{RoleResearcher, ws.Spec.Researchers},
	} {
		for _, m := range rm.members {
			if id.Matches(m) {
				roles = append(roles, rm.role)
				break
			}
		}
	}
	return roles
}

// IsMember reports whether the user holds any role in the workspace.
func IsMember(ws *treV1.Workspace, id Identity) bool { return len(WorkspaceRoles(ws, id)) > 0 }
