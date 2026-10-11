package v1alpha1

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Workspace phases.
const (
	PhasePending  = "Pending"
	PhaseReady    = "Ready"
	PhaseFailed   = "Failed"
	PhaseDeleting = "Deleting"
)

// Condition types reported on a Workspace.
const (
	ConditionReady         = "Ready"
	ConditionNamespace     = "NamespaceReady"
	ConditionNetworkPolicy = "NetworkPolicyReady"
	ConditionEgressPolicy  = "EgressPolicyReady"
	ConditionVMNetwork     = "VMNetworkReady"
)

// WorkspaceSpec is the desired state of a Workspace.
// +kubebuilder:validation:XValidation:rule="self.templateRef == oldSelf.templateRef",message="templateRef is immutable"
type WorkspaceSpec struct {
	// +kubebuilder:validation:MinLength=1
	DisplayName string `json:"displayName"`

	// +optional
	Description string `json:"description,omitempty"`

	// TemplateRef is the name of the WorkspaceTemplate this workspace instantiates.
	// +kubebuilder:validation:MinLength=1
	TemplateRef string `json:"templateRef"`

	// Parameters are template-specific values, validated by the API against the template schema.
	// +optional
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:validation:Type=object
	// +kubebuilder:pruning:PreserveUnknownFields
	Parameters *apiextensionsv1.JSON `json:"parameters,omitempty"`

	// Owners manage the workspace. Entries are OIDC subjects or email addresses.
	// +kubebuilder:validation:MinItems=1
	// +listType=set
	Owners []string `json:"owners"`

	// Researchers use the workspace.
	// +optional
	// +listType=set
	Researchers []string `json:"researchers,omitempty"`

	// AirlockManagers review data entering and leaving the workspace.
	// +optional
	// +listType=set
	AirlockManagers []string `json:"airlockManagers,omitempty"`

	// Egress adds destinations to the template's internet allowlist.
	// +optional
	Egress EgressSpec `json:"egress,omitempty"`

	// Enabled mirrors AzureTRE: a workspace must be disabled before it can be deleted.
	// +optional
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty"`

	// Airlock configures moving files into and out of the workspace.
	// +optional
	Airlock AirlockSpec `json:"airlock,omitempty"`
}

// AirlockSpec turns the airlock on for a workspace and says where review VMs are created.
type AirlockSpec struct {
	// Enabled lets members create import and export requests.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Review says where airlock managers' review VMs are created, per request type.
	// +optional
	Review AirlockReviewSpec `json:"review,omitempty"`
}

// AirlockReviewSpec mirrors AzureTRE's airlock_review_config.
type AirlockReviewSpec struct {
	// Import review VMs run in a separate import review workspace, so unapproved data never
	// enters the research workspace.
	// +optional
	Import *AirlockReviewTarget `json:"import,omitempty"`
	// Export review VMs run in the request's own workspace. Workspace is ignored.
	// +optional
	Export *AirlockReviewTarget `json:"export,omitempty"`
}

// AirlockReviewTarget is a workspace service and the user resource template to create in it.
type AirlockReviewTarget struct {
	// +optional
	Workspace string `json:"workspace,omitempty"`
	// +kubebuilder:validation:MinLength=1
	Service string `json:"service"`
	// +kubebuilder:validation:MinLength=1
	Template string `json:"template"`
}

// WorkspaceStatus is the observed state of a Workspace.
type WorkspaceStatus struct {
	// +optional
	Phase string `json:"phase,omitempty"`

	// Namespace is the Kubernetes namespace that holds the workspace's workloads.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// VMNetwork is the cloud subnet allocated to this workspace's VMs. Once set it never changes.
	// +optional
	VMNetwork *VMNetworkStatus `json:"vmNetwork,omitempty"`

	// ServiceSubnets are extra subnets allocated to the workspace's services.
	// +optional
	// +listType=map
	// +listMapKey=service
	ServiceSubnets []ServiceSubnetStatus `json:"serviceSubnets,omitempty"`

	// MemberIDs maps members' email addresses to their Entra ID object IDs, recorded by the API
	// when they use the TRE. Cloud services grant roles to these IDs.
	// +optional
	MemberIDs map[string]string `json:"memberIDs,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ServiceSubnetStatus is a service's share of the VM address pool. It is released when the
// service is deleted.
type ServiceSubnetStatus struct {
	Service         string   `json:"service"`
	AddressPrefixes []string `json:"addressPrefixes"`

	// FirewallPriority orders the service's own egress rule collection; unique like a
	// workspace's.
	FirewallPriority int32 `json:"firewallPriority"`
}

// VMNetworkStatus records a workspace's VM subnet allocation.
type VMNetworkStatus struct {
	// AddressPrefix is the subnet CIDR, allocated from the controller's VM address pool.
	AddressPrefix string `json:"addressPrefix"`

	// FirewallPriority orders this workspace's egress rule collection; unique per workspace.
	FirewallPriority int32 `json:"firewallPriority"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=ws
// +kubebuilder:printcolumn:name="Template",type=string,JSONPath=`.spec.templateRef`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Namespace",type=string,JSONPath=`.status.namespace`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Workspace is an isolated research environment backed by its own namespace.
type Workspace struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkspaceSpec   `json:"spec,omitempty"`
	Status WorkspaceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WorkspaceList contains a list of Workspace.
type WorkspaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Workspace `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Workspace{}, &WorkspaceList{})
}
