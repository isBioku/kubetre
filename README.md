# KubeTRE

An experimental cloud-native port of Microsoft's
[Azure Trusted Research Environment (AzureTRE)](https://github.com/microsoft/AzureTRE) to
Kubernetes. It keeps AzureTRE's user interface and API, and replaces the back end with
Kubernetes operators, Helm, Flux and Crossplane.

A TRE administrator creates **workspaces** from **templates**. Each workspace is an isolated
namespace with a quota, a default-deny network, and an internet allowlist. Researchers see only
the workspaces they belong to.

## AzureTRE's user interface

Everything is created from the user interface of Microsoft's
[Azure Trusted Research Environment](https://github.com/microsoft/AzureTRE), which KubeTRE runs
unchanged in look and behaviour. Workspaces, workspace services such as Virtual Desktops, and
researchers' own VMs and desktops are created there, and **Connect** opens a session through
Apache Guacamole. AzureTRE and its UI are Microsoft's work, used here under their MIT licence.
The UI is vendored in [ui/](ui/) with Microsoft's copyright notice; [ui/README.md](ui/README.md)
lists the few lines changed so it can run against KubeTRE.

KubeTRE serves AzureTRE's REST API (`/api/workspaces`, `/workspace-services`, `/user-resources`,
templates and operations) from Kubernetes resources instead of Cosmos DB, Service Bus and Porter.
This is an experimental, community port to test whether AzureTRE can be cloud-native. It is not
an official Microsoft release.

## Status

| Milestone | What exists |
|---|---|
| 1. Control plane | `Workspace` and `WorkspaceTemplate` CRDs; controller creating an isolated namespace with Pod Security, quota, default-deny NetworkPolicies and Cilium FQDN egress; HTTP API with single-audience OIDC |
| 2. Services and VMs (in progress) | `ServiceTemplate` and `WorkspaceService` CRDs; controller installing Helm charts through Flux as a confined per-workspace identity; API for shared and personal services; Linux container desktop; Windows and Linux Azure VMs through Crossplane, each workspace on its own firewalled subnet |

| 3. Azure infrastructure | Terraform for AKS, VNet, Azure Firewall, ACR, Log Analytics, Crossplane identity and Flux, with mocked-provider security tests; Flux GitOps tree that installs Crossplane and KubeTRE |

| 4. Access gateway | OIDC broker plus stock Guacamole: owner-only sessions, targets confined to the workspace, clipboard and file transfer off; Envoy Gateway behind a single firewall DNAT rule |

| 5. AzureTRE UI | Microsoft's AzureTRE UI served at `/`, backed by an AzureTRE-compatible API; workspaces, Virtual Desktops and personal VMs created from the UI |

Hosting is cloud-only: AKS first, EKS and GKE later ([ADR 7](docs/adr/0007-cloud-targets.md)).
Design decisions are in [docs/adr](docs/adr).

## Layout

```text
api/v1alpha1/        CRD Go types (source of truth for the CRDs)
internal/controller/ Workspace reconciler
internal/api/        KubeTRE's own HTTP API (/api/v1) and authentication
internal/tre/        AzureTRE-compatible API (/api) for AzureTRE's UI
ui/                  AzureTRE's UI (Microsoft, MIT), vendored with minimal changes
internal/gateway/    access gateway broker: OIDC sign-in, session authorization, Guacamole hand-off
internal/access/     identity and workspace membership shared by the API and the gateway
cmd/                 controller and api entrypoints
charts/              linux-desktop and research-vm Helm charts
crossplane/          XRDs, Azure compositions and package installs for workspace networks and VMs
infra/azure/         Terraform for one environment: AKS, network, firewall, ACR, identities, Flux
deploy/azure/        Flux stages: Crossplane, its packages, then the KubeTRE platform
images/              linux-desktop container image
config/              CRDs, RBAC, deployments, the access gateway, and registered templates (Kustomize)
test/e2e/            API + controller against a real API server
test/charts/         rendered charts checked against Pod Security admission and the VM XRD
test/crossplane/     compositions rendered and checked against the real Azure provider schemas
test/deploy/         GitOps tree checked against Terraform outputs and real Flux/Crossplane schemas
docs/                deployment guide and architecture decision records (docs/adr)
hack/                Entra ID setup and CRD refresh scripts
```

## Develop

Needs Go 1.27. No cluster or Docker is required for tests.

```sh
make test        # installs controller-gen and envtest binaries on first run
make generate    # after changing api/v1alpha1 or RBAC markers
make build
```

Infrastructure checks, also without an Azure account:

```sh
make infra-test     # terraform validate plus security tests against a mocked provider
```

To deploy an environment, follow the [deployment guide](docs/deployment-guide.md). It goes from
an empty Azure subscription to a researcher connected to a Windows VM, and covers upgrades,
teardown and troubleshooting. The VM security model and its open gaps are in
[ADR 8](docs/adr/0008-vm-security-model.md).

## Roadmap

1. **Control plane skeleton.** Done.
2. **Services and VMs.** Done in code, untested in Azure.
3. **AKS infrastructure.** Done in code; first `terraform apply` pending.
4. **Access gateway.** Done in code; certificate automation and session recording pending.
5. **Shared services.** Package mirror (Nexus) and git mirror in `kubetre-shared`.
6. **UI.** Template-driven forms from the template schemas.
7. **Airlock.** PostgreSQL state, storage per stage, presigned URLs, malware scanning, review VMs.
8. **Hardening.** Audit logs, Defender, stronger container isolation, cost reporting.

## Status and license

KubeTRE is early-stage. Read the open gaps in
[ADR 8](docs/adr/0008-vm-security-model.md) and [ADR 9](docs/adr/0009-access-gateway.md)
before using it for sensitive data.

Released under the [MIT License](LICENSE). AzureTRE and its UI are copyright Microsoft
Corporation under the MIT License; see [ui/LICENSE](ui/LICENSE). KubeTRE is an independent,
experimental port and is not an official Microsoft product.
