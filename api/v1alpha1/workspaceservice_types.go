package v1alpha1

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkspaceServiceSpec is the desired state of a service inside a workspace.
// +kubebuilder:validation:XValidation:rule="self.templateRef == oldSelf.templateRef",message="templateRef is immutable"
// +kubebuilder:validation:XValidation:rule="self.owner == oldSelf.owner",message="owner is immutable"
// +kubebuilder:validation:XValidation:rule="(has(self.parentService) ? self.parentService : '') == (has(oldSelf.parentService) ? oldSelf.parentService : '')",message="parentService is immutable"
type WorkspaceServiceSpec struct {
	// TemplateRef is the name of the ServiceTemplate to install.
	// +kubebuilder:validation:MinLength=1
	TemplateRef string `json:"templateRef"`

	// +kubebuilder:validation:MinLength=1
	DisplayName string `json:"displayName"`

	// Values are user-supplied chart values, validated by the API against the template.
	// +optional
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:validation:Type=object
	// +kubebuilder:pruning:PreserveUnknownFields
	Values *apiextensionsv1.JSON `json:"values,omitempty"`

	// Owner is the researcher a per-user service belongs to. Empty for shared services.
	// +optional
	Owner string `json:"owner"`

	// ParentService is the workspace service a user resource lives under.
	// +optional
	ParentService string `json:"parentService,omitempty"`

	// +optional
	Description string `json:"description,omitempty"`

	// Enabled mirrors AzureTRE: a resource must be disabled before it can be deleted.
	// +optional
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty"`
}

// WorkspaceServiceStatus is the observed state of a WorkspaceService.
type WorkspaceServiceStatus struct {
	// +optional
	Phase string `json:"phase,omitempty"`

	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=wsvc
// +kubebuilder:printcolumn:name="Template",type=string,JSONPath=`.spec.templateRef`
// +kubebuilder:printcolumn:name="Owner",type=string,JSONPath=`.spec.owner`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// WorkspaceService is an installed instance of a ServiceTemplate. It lives in the
// workspace's namespace.
type WorkspaceService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkspaceServiceSpec   `json:"spec,omitempty"`
	Status WorkspaceServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WorkspaceServiceList contains a list of WorkspaceService.
type WorkspaceServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WorkspaceService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WorkspaceService{}, &WorkspaceServiceList{})
}
