package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Airlock request types and statuses, as AzureTRE names them.
const (
	AirlockImport = "import"
	AirlockExport = "export"

	AirlockDraft               = "draft"
	AirlockSubmitted           = "submitted"
	AirlockInReview            = "in_review"
	AirlockApprovalInProgress  = "approval_in_progress"
	AirlockApproved            = "approved"
	AirlockRejectionInProgress = "rejection_in_progress"
	AirlockRejected            = "rejected"
	AirlockCancelled           = "cancelled"
	AirlockBlockingInProgress  = "blocking_in_progress"
	AirlockBlockedByScan       = "blocked_by_scan"
	AirlockFailed              = "failed"
	AirlockRevoked             = "revoked"
)

// AirlockUser identifies who acted. ID is the Entra object ID, which AzureTRE's UI compares
// with the signed-in account.
type AirlockUser struct {
	ID    string `json:"id"`
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
}

// AirlockRequestSpec is what the requester asked for. It does not change.
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="an airlock request's spec is immutable"
type AirlockRequestSpec struct {
	// +kubebuilder:validation:Enum=import;export
	Type string `json:"type"`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=200
	Title string `json:"title"`
	// +kubebuilder:validation:MaxLength=4000
	BusinessJustification string      `json:"businessJustification,omitempty"`
	CreatedBy             AirlockUser `json:"createdBy"`
}

// AirlockFile is a file in a request.
type AirlockFile struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// AirlockReview is one review decision, or a revocation.
type AirlockReview struct {
	ID string `json:"id"`
	// +kubebuilder:validation:Enum=approved;rejected;revoked
	Decision    string      `json:"decision"`
	Explanation string      `json:"explanation,omitempty"`
	Reviewer    AirlockUser `json:"reviewer"`
	When        metav1.Time `json:"when"`
}

// AirlockReviewResource is a review VM created for one reviewer.
type AirlockReviewResource struct {
	Workspace    string `json:"workspace"`
	Service      string `json:"service"`
	UserResource string `json:"userResource"`
}

// AirlockHistoryItem records a previous state of the request.
type AirlockHistoryItem struct {
	Version        int64       `json:"version"`
	When           metav1.Time `json:"when"`
	UpdatedBy      AirlockUser `json:"updatedBy"`
	PreviousStatus string      `json:"previousStatus"`
}

// AirlockScanResult is the malware scanner's verdict on the request's file.
type AirlockScanResult struct {
	Clean   bool   `json:"clean"`
	Message string `json:"message,omitempty"`
}

// AirlockRequestStatus is the request's progress. The API records user actions; the
// controller moves the data and completes the in-progress statuses.
type AirlockRequestStatus struct {
	// +optional
	Status string `json:"status,omitempty"`
	// +optional
	StatusMessage string `json:"statusMessage,omitempty"`
	// +optional
	Files []AirlockFile `json:"files,omitempty"`
	// +optional
	ScanResult *AirlockScanResult `json:"scanResult,omitempty"`
	// +optional
	Reviews []AirlockReview `json:"reviews,omitempty"`
	// ReviewResources are review VMs, keyed by the reviewer's object ID.
	// +optional
	ReviewResources map[string]AirlockReviewResource `json:"reviewResources,omitempty"`
	// +optional
	History []AirlockHistoryItem `json:"history,omitempty"`
	// Version counts updates, as AzureTRE's resourceVersion does.
	// +optional
	Version int64 `json:"version,omitempty"`
	// +optional
	UpdatedBy AirlockUser `json:"updatedBy,omitempty"`
	// +optional
	UpdatedWhen *metav1.Time `json:"updatedWhen,omitempty"`
	// NotifiedStatus is the last status notifications were sent for.
	// +optional
	NotifiedStatus string `json:"notifiedStatus,omitempty"`
}

// AirlockRequest is a request to move one file into (import) or out of (export) a workspace.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=airlock
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`
// +kubebuilder:printcolumn:name="Title",type=string,JSONPath=`.spec.title`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type AirlockRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AirlockRequestSpec   `json:"spec,omitempty"`
	Status AirlockRequestStatus `json:"status,omitempty"`
}

// AirlockRequestList contains a list of AirlockRequest.
// +kubebuilder:object:root=true
type AirlockRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AirlockRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AirlockRequest{}, &AirlockRequestList{})
}
