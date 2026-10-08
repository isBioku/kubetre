# KubeTRE and AzureTRE

KubeTRE keeps what users see and replaces what operators run.

## The same

- **User interface.** Microsoft's AzureTRE UI, with its branding, layout and wording.
- **API.** AzureTRE's REST API for workspaces, workspace services, user resources, templates
  and operations, with the same payloads and envelopes.
- **Concepts.** Workspaces, workspace services, user resources and templates, each created
  from a form generated from the template's JSON Schema.
- **Roles.** `TREAdmin`, `TREUser`, `WorkspaceOwner`, `WorkspaceResearcher` and
  `AirlockManager`, with AzureTRE's rules:
  - Only TRE administrators create workspaces, and owners add workspace services.
  - Owners and researchers create user resources. Researchers see only their own, while owners
    see all of them.
- **Behaviour.** Disable before delete, optimistic concurrency with `_etag`, read-only fields
  on update unless marked `updateable`, and operations followed in the notification panel.

## Different

| Area | AzureTRE | KubeTRE |
|---|---|---|
| Composition | Cosmos DB, Service Bus, resource processor, Porter | Kubernetes resources, controller, Flux, Crossplane |
| Templates | Porter bundles registered through the API | `WorkspaceTemplate` and `ServiceTemplate` resources committed to Git, with Helm charts |
| Workspace boundary | a VNet per workspace | a namespace per workspace, plus a subnet per VM workspace |
| Ingress | Application Gateway | Azure Firewall DNAT to Envoy Gateway |
| Remote access | Guacamole App Service per workspace | one shared access gateway that authorises by ownership |
| Workspace roles | Entra application per workspace | members on the `Workspace` resource |
| Entra applications | API, UX, application admin, automation, one per workspace | API, UI, gateway |
| Configuration | `config.yaml` and `make` targets | Terraform variables and Flux |
| Deployment | `make all` or GitHub Actions | `terraform apply`, image builds in ACR, `git push` |
| Machines | Windows and Linux Azure VMs | Windows and Linux Azure VMs, Linux container desktops, optional Linux VMs on KubeVirt |
| Clipboard and files | configurable per Guacamole service | always off |

## Not available yet

- **Data movement:** the airlock, review VMs and the airlock notifier.
- **Shared services:** the Nexus and Gitea mirrors, CycleCloud and certificates.
- **Workspace services:** Azure ML, Databricks, Gitea, Health Data Services, OHDSI, MySQL,
  Azure SQL and Azure OpenAI.
- **Workspace types:** the Unrestricted and Airlock Import Review workspaces.
- **Costs and power:** cost reporting, start and stop for the TRE and for VMs, and VM
  password reset.
- **Lifecycle:** template version upgrades of existing resources, resource history, pipeline
  templates and custom actions.
- **Identity:** the Users page backed by Microsoft Graph, and automation identities. API roles
  can only be assigned to users.
- **Azure features:** customer-managed keys, DNS security policy, forced tunnelling to an
  external firewall, Azure Monitor private link and Azure US Government.
- **Tooling:** the CLI, the Swagger UI, CI/CD workflows and Let's Encrypt automation.

## Only in KubeTRE

- **Linux desktops** that run as containers and start in about a minute.
- **Linux VMs on KubeVirt**, chosen per VM.
- **Two layers of egress control:** Cilium FQDN policies for pods, and Azure Firewall rules
  for VMs.
- **No per-workspace storage accounts**, so AzureTRE's 32-workspace soft limit does not apply.
- **Tests for every platform layer**, run against a real Kubernetes API server (envtest),
  including the API under its own RBAC.
