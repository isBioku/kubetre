# Architecture

KubeTRE keeps AzureTRE's concepts and user experience, and replaces how they are built. The
biggest change is the composition service: AzureTRE's API, Cosmos DB, Service Bus, resource
processor and Porter are replaced by Kubernetes resources, a controller and GitOps.

## Overview

```mermaid
flowchart LR
  user([Researcher or administrator]) -->|HTTPS 443| fw[Azure Firewall<br/>DNAT]
  fw --> envoy[Envoy Gateway<br/>internal load balancer]
  subgraph aks [AKS cluster]
    envoy -->|/| ui[AzureTRE UI]
    envoy -->|/api| api[KubeTRE API]
    envoy -->|/gateway| broker[Access gateway broker]
    envoy -->|/guacamole| guac[Apache Guacamole]
    guac --> guacd[guacd]
    api -->|Workspace, WorkspaceService| k8s[(Kubernetes API)]
    ctrl[KubeTRE controller] --> k8s
    flux[Flux] --> k8s
    xp[Crossplane] --> k8s
    subgraph ws [Workspace namespace ws-id]
      desk[Desktops, KubeVirt VMs,<br/>services]
    end
    guacd -->|VNC, SSH| desk
  end
  guacd -->|RDP, SSH| vm[Azure VMs in<br/>workspace subnets]
  xp -->|subnets, NSGs, firewall rules, VMs| azure[(Azure Resource Manager)]
  flux -->|charts, images| acr[(Azure Container Registry)]
  broker -.->|sign-in| entra[Microsoft Entra ID]
  ui -.->|sign-in| entra
  aks -->|all egress| fw
```

Everything runs on one AKS cluster with three node pools, plus an optional fourth:

| Node pool | Runs |
|---|---|
| `system` | AKS add-ons and KubeVirt's control components |
| `work` | Flux, Crossplane, Envoy Gateway, CDI, the KubeTRE controller and API, and workspace containers |
| `gateway` | the access gateway only (tainted); its pods use a dedicated subnet that VM security groups trust |
| `kubevirt` | KubeVirt VMs only (tainted, optional) |

## Components

| AzureTRE | KubeTRE | Notes |
|---|---|---|
| TRE API (FastAPI on App Service) | KubeTRE API (Go), `internal/tre` and `internal/api` | Serves AzureTRE's REST API at `/api`, and KubeTRE's own at `/api/v1` |
| Configuration store (Cosmos DB) | Kubernetes API (etcd) | Workspaces and services are custom resources |
| Service Bus | none | The API writes desired state; the controller watches it |
| Resource processor (Python on a VM scale set) and Porter | KubeTRE controller, Flux, Crossplane | See [Provisioning](#provisioning-a-workspace) |
| Porter bundle | Helm chart and template resource | Charts are OCI artifacts in ACR |
| Application Gateway | Azure Firewall DNAT and Envoy Gateway | One public entry point, port 443 |
| Guacamole App Service per workspace | One shared access gateway | OIDC broker, Guacamole and guacd |
| Azure Bastion and jump box | none | Operators use `kubectl` through Entra ID |
| UI (React) | the same UI, vendored | Runtime configuration and workspace roles from the API |

The custom resources are:

| Resource | Scope | Meaning |
|---|---|---|
| `WorkspaceTemplate` | cluster | a kind of workspace: quota, egress, Pod Security, VM network, form schema |
| `ServiceTemplate` | cluster | a kind of workspace service or user resource: chart, values, form schema |
| `Workspace` | cluster | one workspace: template, parameters, members, allowed domains |
| `WorkspaceService` | workspace namespace | one workspace service, or with a parent, one user resource |
| `WorkspaceNetwork`, `ResearchVM` | workspace namespace | Crossplane composite resources for the VM subnet and Azure VMs |

## Provisioning a workspace

AzureTRE sends a message through Service Bus to a resource processor, which runs a Porter
bundle and reports back through another queue. KubeTRE has no queue: the API records what is
wanted, and the controller makes it so.

```mermaid
sequenceDiagram
  participant UI as AzureTRE UI
  participant API as KubeTRE API
  participant K as Kubernetes API
  participant C as KubeTRE controller
  participant X as Crossplane
  participant AZ as Azure
  UI->>API: POST /api/workspaces (template, properties)
  API->>API: check role, validate properties against the template schema
  API->>K: create Workspace
  API-->>UI: 202 with an operation
  C->>K: watch Workspace
  C->>K: namespace, quota, limits, network policies, deployer account
  C->>K: WorkspaceNetwork (VM workspaces only)
  X->>AZ: subnet, security group, route association, firewall rules
  X->>K: WorkspaceNetwork ready
  C->>K: Workspace status: Ready
  UI->>API: GET .../operations/{id} (polling)
  API->>K: read Workspace
  API-->>UI: status deployed
```

Workspace services and user resources follow the same pattern. The controller creates a
Flux `OCIRepository` and `HelmRelease` in the workspace namespace. Flux installs the chart
while impersonating the workspace's `kubetre-deployer` account, which can create workloads,
VMs and disks in that namespace only. A chart cannot change the workspace's network policies,
quota or RBAC.

Properties of this design:

- **Desired state is the store.** There is no separate database to fall out of step.
- **Retries are built in.** Controllers reconcile until the actual state matches.
- **Concurrency is safe.** The API passes the resource version as AzureTRE's `_etag`, so
  conflicting updates fail cleanly.
- **Operations are derived.** The API reports an operation's status from the resource's
  generation and phase, so there is no operation history after a resource is deleted.

## The access gateway

```mermaid
sequenceDiagram
  participant B as Browser
  participant G as Broker (/gateway)
  participant E as Entra ID
  participant Q as Guacamole
  participant D as guacd
  participant V as VM or desktop
  B->>G: GET /gateway/connect/{workspace}/{resource}
  G->>G: same-site check, session cookie
  G->>E: sign in (if needed)
  E-->>G: identity
  G->>G: is the caller the resource's owner? is the target inside the workspace?
  G-->>B: hand-off page with a signed, encrypted, single-use payload
  B->>Q: open the session with the payload
  Q->>D: connect
  D->>V: RDP, SSH or VNC
```

The broker decides every connection from current data. See
[ADR 9](../adr/0009-access-gateway.md) for the full design.

## Further reading

- [Networking](networking.md) and [Azure resources](azure-resources.md).
- Architecture decisions: [ADR 1](../adr/0001-operator-model.md) (operator model),
  [ADR 4](../adr/0004-egress.md) (egress), [ADR 8](../adr/0008-vm-security-model.md) (VMs),
  [ADR 10](../adr/0010-azuretre-ui.md) (the AzureTRE UI).
