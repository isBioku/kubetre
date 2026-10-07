package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EgressSpec lists the internet destinations a workspace may reach.
// Everything not listed is denied.
type EgressSpec struct {
	// AllowedFQDNs are hostnames reachable on TCP 443. A leading "*." matches subdomains.
	// +optional
	// +listType=set
	AllowedFQDNs []string `json:"allowedFQDNs,omitempty"`
}

// WorkspaceTemplateSpec defines a kind of workspace that administrators can create.
type WorkspaceTemplateSpec struct {
	// +kubebuilder:validation:MinLength=1
	DisplayName string `json:"displayName"`

	// +optional
	Description string `json:"description,omitempty"`

	// Version is the semantic version of this template.
	// +kubebuilder:validation:Pattern=`^[0-9]+\.[0-9]+\.[0-9]+$`
	Version string `json:"version"`

	// ParametersSchema is a JSON Schema that workspace parameters must satisfy.
	// The UI renders a form from it and the API validates against it.
	// +optional
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:validation:Type=object
	// +kubebuilder:pruning:PreserveUnknownFields
	ParametersSchema *apiextensionsv1.JSON `json:"parametersSchema,omitempty"`

	// Quota is applied as the hard limit of the workspace's ResourceQuota.
	// +optional
	Quota corev1.ResourceList `json:"quota,omitempty"`

	// Egress is the default internet allowlist for workspaces of this template.
	// +optional
	Egress EgressSpec `json:"egress,omitempty"`

	// PodSecurity is the Pod Security Standard enforced in workspaces of this template.
	// Keep "restricted" unless a registered service genuinely needs more.
	// +optional
	// +kubebuilder:default=restricted
	PodSecurity PodSecurityLevel `json:"podSecurity,omitempty"`

	// VirtualMachines gives each workspace a private cloud subnet for research VMs,
	// isolated from other workspaces and egressing through the firewall.
	// +optional
	VirtualMachines VirtualMachinesSpec `json:"virtualMachines,omitempty"`
}

// VirtualMachinesSpec controls cloud VM support for a workspace template.
type VirtualMachinesSpec struct {
	// +optional
	Enabled bool `json:"enabled,omitempty"`
}

// PodSecurityLevel is a Kubernetes Pod Security Standard level.
// +kubebuilder:validation:Enum=restricted;baseline;privileged
type PodSecurityLevel string

// Pod Security Standard levels, from strictest to loosest.
const (
	PodSecurityRestricted PodSecurityLevel = "restricted"
	PodSecurityBaseline   PodSecurityLevel = "baseline"
	PodSecurityPrivileged PodSecurityLevel = "privileged"
)

// Rank orders levels so 0 is strictest. Unknown or empty values count as restricted.
func (l PodSecurityLevel) Rank() int {
	switch l {
	case PodSecurityBaseline:
		return 1
	case PodSecurityPrivileged:
		return 2
	default:
		return 0
	}
}

// OrDefault returns restricted when the level is unset.
func (l PodSecurityLevel) OrDefault() PodSecurityLevel {
	if l == "" {
		return PodSecurityRestricted
	}
	return l
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=wstpl
// +kubebuilder:printcolumn:name="Display Name",type=string,JSONPath=`.spec.displayName`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.spec.version`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// WorkspaceTemplate is a registered kind of workspace.
type WorkspaceTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec WorkspaceTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// WorkspaceTemplateList contains a list of WorkspaceTemplate.
type WorkspaceTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WorkspaceTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WorkspaceTemplate{}, &WorkspaceTemplateList{})
}
