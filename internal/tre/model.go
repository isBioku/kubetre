package tre

import (
	"encoding/json"
	"math"
	"regexp"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
	"github.com/isBioku/kubetre/internal/access"
)

// AzureTRE resource types.
const (
	typeWorkspace        = "workspace"
	typeWorkspaceService = "workspace-service"
	typeUserResource     = "user-resource"
)

// Annotations KubeTRE keeps for AzureTRE fields that have no place in the spec.
const (
	annCreatedBy = "kubetre.io/created-by"
	annOverview  = "kubetre.io/overview"
)

// userRef is AzureTRE's user object.
type userRef struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Email           string   `json:"email"`
	Roles           []string `json:"roles"`
	RoleAssignments []any    `json:"roleAssignments"`
}

func userOf(principal string) userRef {
	return userRef{ID: principal, Name: principal, Email: principal, Roles: []string{}, RoleAssignments: []any{}}
}

func userFromIdentity(id access.Identity) userRef {
	u := userOf(id.Principal())
	u.ID = userID(id)
	if id.Name != "" {
		u.Name = id.Name
	}
	return u
}

// resource is AzureTRE's resource JSON.
type resource struct {
	ID                       string         `json:"id"`
	IsEnabled                bool           `json:"isEnabled"`
	ResourcePath             string         `json:"resourcePath"`
	ResourceVersion          int64          `json:"resourceVersion"`
	ResourceType             string         `json:"resourceType"`
	TemplateName             string         `json:"templateName"`
	TemplateVersion          string         `json:"templateVersion"`
	AvailableUpgrades        []any          `json:"availableUpgrades"`
	DeploymentStatus         string         `json:"deploymentStatus"`
	UpdatedWhen              float64        `json:"updatedWhen"`
	User                     userRef        `json:"user"`
	Etag                     string         `json:"_etag"`
	Properties               map[string]any `json:"properties"`
	WorkspaceID              string         `json:"workspaceId,omitempty"`
	ParentWorkspaceServiceID string         `json:"parentWorkspaceServiceId,omitempty"`
	OwnerID                  string         `json:"ownerId,omitempty"`
	// UserRoles is a KubeTRE extension: the caller's roles in a workspace, which AzureTRE reads
	// from a per-workspace token instead.
	UserRoles []string `json:"userRoles,omitempty"`
}

func enabled(b *bool) bool { return b == nil || *b }

// deploymentStatus maps a KubeTRE phase onto AzureTRE's deployment statuses.
func deploymentStatus(obj metav1.Object, phase string, generation int64) string {
	if !obj.GetDeletionTimestamp().IsZero() || phase == treV1.PhaseDeleting {
		return "deleting"
	}
	switch phase {
	case treV1.PhaseReady:
		if generation > 1 {
			return "updated"
		}
		return "deployed"
	case treV1.PhaseFailed:
		if generation > 1 {
			return "updating_failed"
		}
		return "deployment_failed"
	case treV1.PhasePending:
		if generation > 1 {
			return "updating"
		}
		return "deploying"
	default:
		return "awaiting_deployment"
	}
}

func readyMessage(conds []metav1.Condition) string {
	for _, c := range conds {
		if c.Type == treV1.ConditionReady {
			return c.Message
		}
	}
	return ""
}

// updatedWhen is the latest of creation and condition transitions, in AzureTRE's unix seconds.
func updatedWhen(obj metav1.Object, conds []metav1.Condition) float64 {
	t := obj.GetCreationTimestamp().Time
	for _, c := range conds {
		if c.LastTransitionTime.After(t) {
			t = c.LastTransitionTime.Time
		}
	}
	if t.IsZero() {
		t = time.Now()
	}
	return math.Floor(float64(t.UnixMilli())) / 1000
}

func createdBy(obj metav1.Object) userRef {
	return userOf(obj.GetAnnotations()[annCreatedBy])
}

func rawMap(j []byte) map[string]any {
	m := map[string]any{}
	if len(j) > 0 {
		_ = json.Unmarshal(j, &m)
	}
	return m
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// slug turns a display name into a DNS label prefix for generateName.
func slug(name string, max int) string {
	s := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(s) > max {
		s = strings.Trim(s[:max], "-")
	}
	if s == "" || s[0] < 'a' || s[0] > 'z' {
		s = "r" + s
		if len(s) > max {
			s = s[:max]
		}
	}
	return s + "-"
}

func stringList(v any) []string {
	var out []string
	switch t := v.(type) {
	case []any:
		for _, e := range t {
			if s, ok := e.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
	case []string:
		for _, s := range t {
			if strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
	case string:
		for _, s := range strings.Split(t, ",") {
			if strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
	}
	return out
}

func toAny(in []string) []any {
	out := make([]any, 0, len(in))
	for _, s := range in {
		out = append(out, s)
	}
	return out
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// annOwnerID records the owner's Entra object ID, which AzureTRE's UI compares with the
// signed-in account to tell "my" user resources apart.
const annOwnerID = "kubetre.io/owner-id"

// userID is the ID AzureTRE's UI knows the caller by: the Entra object ID when present.
func userID(id access.Identity) string {
	if id.ObjectID != "" {
		return id.ObjectID
	}
	return id.Principal()
}
