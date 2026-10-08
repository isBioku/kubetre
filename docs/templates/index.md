# Templates

Everything a researcher can create comes from a template, as in AzureTRE. A template is a
Kubernetes resource that Flux registers from Git:

| AzureTRE | KubeTRE | Defined in |
|---|---|---|
| Workspace template (Porter bundle) | `WorkspaceTemplate` | `config/templates/workspacetemplate-*.yaml` |
| Workspace service template | `ServiceTemplate` with `kind: WorkspaceService` | `config/templates/servicetemplate-*.yaml` |
| User resource template | `ServiceTemplate` with `kind: UserResource` and `parentTemplate` | `config/templates`, `config/kubevirt` |
| Shared service template | not available yet | |

A template's JSON Schema becomes the form in AzureTRE's UI. KubeTRE adds a name, a
description and an overview field to every form, as AzureTRE does. Fields marked
`updateable: true` can be changed after creation; all others are read-only on the update form.

## Registered templates

| Template | Type | Page |
|---|---|---|
| Base workspace | Workspace | [Base workspace](workspaces/base.md) |
| Workspace with virtual machines | Workspace | [Workspace with virtual machines](workspaces/vm-workspace.md) |
| Virtual Desktops | Workspace service | [Virtual Desktops](workspace-services/virtual-desktops.md) |
| Windows VM (Windows Server 2022) | User resource | [Windows VM](user-resources/windows-vm.md) |
| Linux VM (Ubuntu 24.04) | User resource | [Linux VM](user-resources/linux-vm.md) |
| Linux desktop | User resource | [Linux desktop](user-resources/linux-desktop.md) |
| Linux VM on KubeVirt (Ubuntu 24.04) | User resource, optional | [Linux VM on KubeVirt](user-resources/linux-vm-kubevirt.md) |

## Not available yet

AzureTRE ships many more templates. KubeTRE has no equivalent yet for these, so they do not
appear in the UI:

- **Workspaces:** Unrestricted, Airlock Import Review.
- **Workspace services:** Azure ML, Azure Databricks, Gitea, Health Services, OHDSI, MySQL,
  Azure SQL, Azure OpenAI.
- **Shared services:** Gitea and Nexus mirrors, CycleCloud, Airlock Notifier.
- **User resources:** import and export review VMs.

To add your own, see [Authoring templates](../developers/authoring-templates.md).
