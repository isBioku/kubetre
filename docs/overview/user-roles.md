# User roles

KubeTRE keeps AzureTRE's roles and personas. One person can hold several.

## Roles the TRE enforces

| Role | Scope | Where it is held | Can |
|---|---|---|---|
| TRE administrator (`TREAdmin`) | whole TRE | Entra app role on the API app | create, update and delete workspaces; see every workspace |
| TRE user (`TREUser`) | whole TRE | Entra app role on the API app | sign in to the UI |
| Workspace owner (`WorkspaceOwner`) | one workspace | owners list on the `Workspace` | add, change and remove workspace services; see every user resource; everything a researcher can |
| Researcher (`WorkspaceResearcher`) | one workspace | researchers list on the `Workspace` | create and manage their own user resources; connect to their own machines |
| Airlock manager (`AirlockManager`) | one workspace | airlock managers list on the `Workspace` | nothing yet: KubeTRE has no airlock |

How this differs from AzureTRE:

- **Workspace roles are not Entra app roles.** AzureTRE registers an Entra application for
  each workspace and assigns workspace roles on it. KubeTRE keeps the members of each workspace
  on the `Workspace` resource, by email address, and the API returns the caller's roles to the
  UI. Creating a workspace creates nothing in Entra ID.
- **Changes apply at once.** Workspace role changes take effect on the next page load.
  TRE-wide roles come from the sign-in token, so a change needs the user to sign out and in.
- **Owners cannot open researchers' machines.** Owners see every VM in the UI, but the access
  gateway only connects a machine's owner.
- **Non-members see nothing.** A user who is neither a member nor a TRE administrator gets
  "not found" for a workspace, so workspace names do not leak.

## Personas

| Persona | In KubeTRE |
|---|---|
| Azure administrator | Deploys and upgrades the TRE with Terraform and Flux, manages TRE administrators in Entra ID, troubleshoots the cluster and Azure resources. Skills: Azure, Terraform, Kubernetes, Git. |
| TRE administrator | Creates and manages workspaces and their members in the UI. Needs no Azure or Kubernetes access. |
| Workspace owner | Adds workspace services and oversees the workspace's machines. |
| Researcher | Creates their own machines and works in them. Installs packages from allowed domains. |
| Airlock manager | Will review imports and exports once the airlock exists. |
| TRE service integrator | Adds new tools as templates: a Helm chart plus a `ServiceTemplate`, and Crossplane compositions for cloud resources. See [Authoring templates](../developers/authoring-templates.md). |
| KubeTRE developer | Changes the controller, API, gateway or UI. See [Developers](../developers/index.md). |
| Data engineer | Builds pipelines between the data platform and the TRE. KubeTRE has no data ingress path yet. |
| Information security officer | Reviews the security model ([ADR 8](../adr/0008-vm-security-model.md), [ADR 9](../adr/0009-access-gateway.md)) and the [security checks](../quickstart/verify.md) before sensitive data is used. |

## Seeing your roles

In the UI, open the user menu at the top right and select **Your access**. It lists your TRE
roles and, inside a workspace, your workspace roles.
