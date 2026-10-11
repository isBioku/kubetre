// Package airlock implements AzureTRE's airlock rules: the request state machine, who may do
// what, which storage holds each stage, and the storage and notification back ends.
package airlock

import (
	"errors"
	"slices"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
	"github.com/isBioku/kubetre/internal/access"
)

// ErrIllegalTransition is AzureTRE's "Airlock request status change was illegal".
var ErrIllegalTransition = errors.New("Airlock request status change was illegal")

var transitions = map[string][]string{
	treV1.AirlockDraft:               {treV1.AirlockSubmitted, treV1.AirlockCancelled, treV1.AirlockFailed},
	treV1.AirlockSubmitted:           {treV1.AirlockInReview, treV1.AirlockBlockingInProgress, treV1.AirlockFailed},
	treV1.AirlockInReview:            {treV1.AirlockApprovalInProgress, treV1.AirlockRejectionInProgress, treV1.AirlockCancelled, treV1.AirlockFailed},
	treV1.AirlockApprovalInProgress:  {treV1.AirlockApproved, treV1.AirlockFailed},
	treV1.AirlockRejectionInProgress: {treV1.AirlockRejected, treV1.AirlockFailed},
	treV1.AirlockBlockingInProgress:  {treV1.AirlockBlockedByScan, treV1.AirlockFailed},
	treV1.AirlockApproved:            {treV1.AirlockRevoked},
}

// CanTransition reports whether a request may move from one status to another.
func CanTransition(from, to string) bool { return slices.Contains(transitions[from], to) }

// IsFinal reports whether no further transition is possible, apart from revoking an approval.
func IsFinal(status string) bool {
	switch status {
	case treV1.AirlockApproved, treV1.AirlockRejected, treV1.AirlockBlockedByScan,
		treV1.AirlockCancelled, treV1.AirlockFailed, treV1.AirlockRevoked:
		return true
	}
	return false
}

// Actions AzureTRE's UI offers.
const (
	ActionReview = "review"
	ActionCancel = "cancel"
	ActionSubmit = "submit"
	ActionRevoke = "revoke"
)

// AllowedActions follows AzureTRE: it depends on the status and the caller's workspace roles
// only, not on who created the request.
func AllowedActions(status string, roles map[string]bool) []string {
	member := roles[access.RoleOwner] || roles[access.RoleResearcher]
	out := []string{}
	if status == treV1.AirlockInReview && roles[access.RoleAirlockManager] {
		out = append(out, ActionReview)
	}
	if (status == treV1.AirlockDraft || status == treV1.AirlockInReview) && member {
		out = append(out, ActionCancel)
	}
	if status == treV1.AirlockDraft && member {
		out = append(out, ActionSubmit)
	}
	if status == treV1.AirlockApproved && roles[access.RoleAirlockManager] {
		out = append(out, ActionRevoke)
	}
	return out
}

// Account is one of the two airlock storage accounts.
type Account string

const (
	// External is reachable from the internet: import drafts and approved exports.
	External Account = "external"
	// Internal is reachable only inside the TRE: export drafts and approved imports.
	Internal Account = "internal"
)

// DraftContainer and SealedContainer name a request's containers. The draft is writable by the
// requester; on submission its file is copied into the sealed container and the draft deleted,
// so links issued for the draft stop working.
func DraftContainer(id string) string  { return id + "-draft" }
func SealedContainer(id string) string { return id }

// Location says where a request's data is for a given status: account and container.
func Location(reqType, status, id string) (Account, string) {
	approved := status == treV1.AirlockApproved || status == treV1.AirlockApprovalInProgress
	container := SealedContainer(id)
	if status == treV1.AirlockDraft {
		container = DraftContainer(id)
	}
	switch {
	case reqType == treV1.AirlockImport && approved:
		return Internal, container
	case reqType == treV1.AirlockImport:
		return External, container
	case approved:
		return External, container
	default:
		return Internal, container
	}
}

// Link errors, with AzureTRE's messages.
var (
	ErrLinkForbidden  = errors.New("User is unauthorized to access airlock request files in its current status.")
	ErrLinkProcessing = errors.New("Airlock request is being processed, please try again later.")
	ErrLinkCancelled  = errors.New("Airlock request was cancelled.")
	ErrLinkInvalid    = errors.New("Airlock request is in invalid status: rejected, blocked or failed.")
)

// Permissions of a container link.
type Permissions struct{ Read, Write, List, Delete bool }

// LinkPermissions decides whether the caller gets a link to the request's files, and with which
// permissions. During review only airlock managers and owners may read; otherwise researchers
// and owners. Drafts are writable.
func LinkPermissions(status string, roles map[string]bool) (Permissions, error) {
	allowed := roles[access.RoleResearcher] || roles[access.RoleOwner]
	if status == treV1.AirlockInReview {
		allowed = roles[access.RoleAirlockManager] || roles[access.RoleOwner]
	}
	if !allowed {
		return Permissions{}, ErrLinkForbidden
	}
	switch status {
	case treV1.AirlockApprovalInProgress, treV1.AirlockRejectionInProgress, treV1.AirlockBlockingInProgress:
		return Permissions{}, ErrLinkProcessing
	case treV1.AirlockCancelled, treV1.AirlockRevoked:
		return Permissions{}, ErrLinkCancelled
	case treV1.AirlockFailed, treV1.AirlockRejected, treV1.AirlockBlockedByScan:
		return Permissions{}, ErrLinkInvalid
	case treV1.AirlockDraft:
		return Permissions{Read: true, Write: true, List: true, Delete: true}, nil
	}
	return Permissions{Read: true, List: true}, nil
}
