package v1alpha1

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ChartRef points at a Helm chart published to an OCI registry.
type ChartRef struct {
	// URL is the chart's OCI repository, for example oci://ghcr.io/org/charts/linux-desktop.
	// +kubebuilder:validation:Pattern=`^oci://.+`
	URL string `json:"url"`

	// Version is the exact chart version to install.
	// +kubebuilder:validation:MinLength=1
	Version string `json:"version"`

	// Provider selects how Flux authenticates to the registry. "azure", "aws" and "gcp"
	// use the cluster's workload identity with ACR, ECR or Artifact Registry.
	// +optional
	// +kubebuilder:default=generic
	// +kubebuilder:validation:Enum=generic;azure;aws;gcp
	Provider string `json:"provider,omitempty"`
}

// Service kinds, mirroring AzureTRE's resource hierarchy.
const (
	// KindWorkspaceService is installed once per workspace by a workspace owner.
	KindWorkspaceService = "WorkspaceService"
	// KindUserResource belongs to one researcher and lives under a workspace service.
	KindUserResource = "UserResource"
)

// ServiceTemplateSpec defines a service that can be installed into workspaces.
// +kubebuilder:validation:XValidation:rule="self.kind != 'UserResource' || (has(self.parentTemplate) && self.parentTemplate != ”)",message="user resource templates need a parentTemplate"
type ServiceTemplateSpec struct {
	// Kind is WorkspaceService or UserResource. User resources are personal and are created
	// under a workspace service of their parentTemplate.
	// +optional
	// +kubebuilder:default=WorkspaceService
	// +kubebuilder:validation:Enum=WorkspaceService;UserResource
	Kind string `json:"kind,omitempty"`

	// ParentTemplate is the workspace service template a user resource is created under.
	// +optional
	ParentTemplate string `json:"parentTemplate,omitempty"`

	// +kubebuilder:validation:MinLength=1
	DisplayName string `json:"displayName"`

	// +optional
	Description string `json:"description,omitempty"`

	// Chart is installed for each instance. Omit it for a workspace service that only groups
	// user resources, such as the virtual desktops service.
	// +optional
	Chart *ChartRef `json:"chart,omitempty"`

	// ValuesSchema is the JSON Schema for values users may set. Keep
	// additionalProperties false so users cannot override chart internals.
	// +optional
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:validation:Type=object
	// +kubebuilder:pruning:PreserveUnknownFields
	ValuesSchema *apiextensionsv1.JSON `json:"valuesSchema,omitempty"`

	// Values are fixed chart values set by the administrator. User values are
	// applied on top, then KubeTRE's own values under the "kubetre" key.
	// +optional
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:validation:Type=object
	// +kubebuilder:pruning:PreserveUnknownFields
	Values *apiextensionsv1.JSON `json:"values,omitempty"`

	// PerUser services belong to one researcher, like a personal desktop. User resources are
	// always per user; this flag remains for templates without a parent service.
	// +optional
	PerUser bool `json:"perUser,omitempty"`

	// RequiredPodSecurity is the loosest Pod Security level the chart's pods need.
	// Installation is refused in workspaces that enforce a stricter level.
	// +optional
	// +kubebuilder:default=restricted
	RequiredPodSecurity PodSecurityLevel `json:"requiredPodSecurity,omitempty"`

	// RequiresVirtualMachines marks templates that create Azure VMs. They can only be used in
	// workspaces whose template enables virtual machines, which gives them a VM subnet.
	// +optional
	RequiresVirtualMachines bool `json:"requiresVirtualMachines,omitempty"`

	// OwnerAccount marks templates whose chart creates a login account for its owner, named
	// by the chart value "username". KubeTRE sets it from the owner's email at creation.
	// +optional
	OwnerAccount bool `json:"ownerAccount,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=svctpl
// +kubebuilder:printcolumn:name="Display Name",type=string,JSONPath=`.spec.displayName`
// +kubebuilder:printcolumn:name="Kind",type=string,JSONPath=`.spec.kind`
// +kubebuilder:printcolumn:name="Chart",type=string,JSONPath=`.spec.chart.url`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.spec.chart.version`

// ServiceTemplate is a registered Helm chart that can run inside workspaces.
type ServiceTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ServiceTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// ServiceTemplateList contains a list of ServiceTemplate.
type ServiceTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServiceTemplate{}, &ServiceTemplateList{})
}

// IsUserResource reports whether instances belong to one researcher.
func (t *ServiceTemplate) IsUserResource() bool {
	return t.Spec.Kind == KindUserResource || t.Spec.PerUser
}
