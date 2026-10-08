# Terms and definitions

KubeTRE keeps AzureTRE's vocabulary. Each term maps to a Kubernetes resource.

**TRE.** One deployment of KubeTRE: one AKS cluster with its Azure network, firewall and
registry. An organisation usually runs one TRE, or one per department.

**Composition service.** The part that turns requests into running resources. In AzureTRE it
is the API, Cosmos DB, Service Bus, the resource processor and Porter. In KubeTRE it is the API
and the KubeTRE controller, with Kubernetes itself as the store of desired state.

**Workspace.** A security boundary for one project: inbound access only for authorised users,
outbound access only to defined destinations, and free movement of data inside. In KubeTRE a
workspace is a `Workspace` resource. It produces a namespace `ws-<id>` and, for VM
workspaces, an Azure subnet with its own security group and firewall rules.

**Workspace owner and researcher.** A workspace has one or more owners and any number of
researchers. Owners are also researchers. See [User roles](user-roles.md).

**Workspace service.** A building block added to a workspace, such as Virtual Desktops. It is a
`WorkspaceService` resource, installed from a `ServiceTemplate` of kind `WorkspaceService`.

**User resource.** A resource that belongs to one researcher, such as their VM. It is a
`WorkspaceService` resource with a parent service, installed from a `ServiceTemplate` of kind
`UserResource`. It can only be created under a compatible workspace service.

**Shared service.** A service for the whole TRE, such as a package mirror. Not available yet.

**Template.** The definition of everything above: a `WorkspaceTemplate` or `ServiceTemplate`
resource with a JSON Schema for its form, and for services a Helm chart. Templates are
registered by committing them to Git; Flux applies them. See [Templates](../templates/index.md).

**Operation.** A create, update or delete request on a resource, which the UI follows in its
notification panel. KubeTRE derives operations from the resource's state rather than storing
them.

**Access gateway.** The single entry point for remote sessions: an OIDC broker, Apache
Guacamole and guacd, shared by all workspaces.

## Limits

AzureTRE has a soft limit of 32 workspaces because each workspace creates storage accounts.
KubeTRE creates no storage account per workspace. Its limits are the cluster's capacity and
the VM address pool: with the defaults, 1,024 workspaces with VMs.
