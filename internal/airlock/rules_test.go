package airlock

import (
	"errors"
	"slices"
	"testing"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
	"github.com/isBioku/kubetre/internal/access"
)

func roles(r ...string) map[string]bool {
	m := map[string]bool{}
	for _, x := range r {
		m[x] = true
	}
	return m
}

func TestTransitionsFollowAzureTRE(t *testing.T) {
	legal := [][2]string{
		{"draft", "submitted"}, {"draft", "cancelled"}, {"submitted", "in_review"}, {"submitted", "blocking_in_progress"},
		{"in_review", "approval_in_progress"}, {"in_review", "rejection_in_progress"}, {"in_review", "cancelled"},
		{"approval_in_progress", "approved"}, {"rejection_in_progress", "rejected"}, {"blocking_in_progress", "blocked_by_scan"},
		{"approved", "revoked"}, {"submitted", "failed"},
	}
	for _, l := range legal {
		if !CanTransition(l[0], l[1]) {
			t.Errorf("%s -> %s should be legal", l[0], l[1])
		}
	}
	for _, l := range [][2]string{{"draft", "approved"}, {"submitted", "cancelled"}, {"rejected", "approved"}, {"approved", "cancelled"}, {"revoked", "approved"}} {
		if CanTransition(l[0], l[1]) {
			t.Errorf("%s -> %s should be illegal", l[0], l[1])
		}
	}
}

func TestAllowedActions(t *testing.T) {
	for _, c := range []struct {
		status string
		roles  map[string]bool
		want   []string
	}{
		{"draft", roles(access.RoleResearcher), []string{"cancel", "submit"}},
		{"draft", roles(access.RoleOwner), []string{"cancel", "submit"}},
		{"in_review", roles(access.RoleResearcher), []string{"cancel"}},
		{"in_review", roles(access.RoleAirlockManager), []string{"review"}},
		{"in_review", roles(access.RoleAirlockManager, access.RoleResearcher), []string{"review", "cancel"}},
		{"approved", roles(access.RoleAirlockManager), []string{"revoke"}},
		{"approved", roles(access.RoleResearcher), []string{}},
		{"submitted", roles(access.RoleOwner, access.RoleAirlockManager), []string{}},
	} {
		if got := AllowedActions(c.status, c.roles); !slices.Equal(got, c.want) {
			t.Errorf("%s %v: got %v, want %v", c.status, c.roles, got, c.want)
		}
	}
}

func TestLocations(t *testing.T) {
	for _, c := range []struct {
		typ, status string
		acct        Account
		container   string
	}{
		{"import", "draft", External, "r1-draft"},
		{"import", "in_review", External, "r1"},
		{"import", "approved", Internal, "r1"},
		{"export", "draft", Internal, "r1-draft"},
		{"export", "in_review", Internal, "r1"},
		{"export", "approved", External, "r1"},
	} {
		a, ctr := Location(c.typ, c.status, "r1")
		if a != c.acct || ctr != c.container {
			t.Errorf("%s %s: got %s/%s, want %s/%s", c.typ, c.status, a, ctr, c.acct, c.container)
		}
	}
}

func TestLinkPermissions(t *testing.T) {
	if p, err := LinkPermissions(treV1.AirlockDraft, roles(access.RoleResearcher)); err != nil || !p.Write || !p.Delete {
		t.Errorf("draft: %+v %v", p, err)
	}
	if p, err := LinkPermissions(treV1.AirlockApproved, roles(access.RoleResearcher)); err != nil || p.Write || !p.Read {
		t.Errorf("approved: %+v %v", p, err)
	}
	if _, err := LinkPermissions(treV1.AirlockInReview, roles(access.RoleResearcher)); !errors.Is(err, ErrLinkForbidden) {
		t.Errorf("researchers do not read during review: %v", err)
	}
	if _, err := LinkPermissions(treV1.AirlockInReview, roles(access.RoleAirlockManager)); err != nil {
		t.Errorf("managers read during review: %v", err)
	}
	if _, err := LinkPermissions(treV1.AirlockDraft, roles(access.RoleAirlockManager)); !errors.Is(err, ErrLinkForbidden) {
		t.Errorf("managers do not get draft links: %v", err)
	}
	for status, want := range map[string]error{"approval_in_progress": ErrLinkProcessing, "cancelled": ErrLinkCancelled, "revoked": ErrLinkCancelled, "rejected": ErrLinkInvalid, "blocked_by_scan": ErrLinkInvalid} {
		if _, err := LinkPermissions(status, roles(access.RoleOwner, access.RoleAirlockManager)); !errors.Is(err, want) {
			t.Errorf("%s: got %v, want %v", status, err, want)
		}
	}
}
