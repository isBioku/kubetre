package airlock

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
)

// Touch records an update as AzureTRE does: a history item snapshots the previous version,
// time, actor and status, and the version increases. by is nil when the platform, not a
// person, made the change; the last person to act stays as updatedBy.
func Touch(req *treV1.AirlockRequest, by *treV1.AirlockUser, now time.Time) {
	prevWhen := req.CreationTimestamp
	if req.Status.UpdatedWhen != nil {
		prevWhen = *req.Status.UpdatedWhen
	}
	req.Status.History = append(req.Status.History, treV1.AirlockHistoryItem{
		Version: req.Status.Version, When: prevWhen, UpdatedBy: req.Status.UpdatedBy, PreviousStatus: req.Status.Status,
	})
	req.Status.Version++
	t := metav1.NewTime(now)
	req.Status.UpdatedWhen = &t
	if by != nil {
		req.Status.UpdatedBy = *by
	}
}

// Transition moves a request to a new status, or returns ErrIllegalTransition.
func Transition(req *treV1.AirlockRequest, to string, by *treV1.AirlockUser, now time.Time) error {
	if !CanTransition(req.Status.Status, to) {
		return ErrIllegalTransition
	}
	Touch(req, by, now)
	req.Status.Status = to
	return nil
}
