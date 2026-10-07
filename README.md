# KubeTRE

A cloud-native Trusted Research Environment for Kubernetes, designed from the ideas in
[AzureTRE](https://github.com/microsoft/AzureTRE) but not ported from its code.

A TRE administrator creates **workspaces** from **templates** through an API. Each workspace is
an isolated namespace with a quota, a default-deny network, and an internet allowlist. Researchers
see only the workspaces they belong to.

## Status

| Milestone | What exists |
|---|---|
| 1. Control plane | `Workspace` and `WorkspaceTemplate` CRDs; controller creating an isolated namespace with Pod Security, quota, default-deny NetworkPolicies and Cilium FQDN egress; HTTP API with single-audience OIDC |
| 2. Services and VMs (in progress) | `ServiceTemplate` and `WorkspaceService` CRDs; controller installing Helm charts through Flux as a confined per-workspace identity; API for shared and personal services; Linux container desktop; Windows and Linux Azure VMs through Crossplane, each workspace on its own firewalled subnet |

| 3. Azure infrastructure | Terraform for AKS, VNet, Azure Firewall, ACR, Log Analytics, Crossplane identity and Flux, with mocked-provider security tests; Flux GitOps tree that installs Crossplane and KubeTRE |

| 4. Access gateway | OIDC broker plus stock Guacamole: owner-only sessions, targets confined to the workspace, clipboard and file transfer off; Envoy Gateway behind a single firewall DNAT rule |

Hosting is cloud-only: AKS first, EKS and GKE later ([ADR 7](docs/adr/0007-cloud-targets.md)).
Design decisions are in [docs/adr](docs/adr).

## Layout

```text
api/v1alpha1/        CRD Go types (source of truth for the CRDs)
internal/controller/ Workspace reconciler
internal/api/        HTTP API and authentication
internal/gateway/    access gateway broker: OIDC sign-in, session authorization, Guacamole hand-off
internal/access/     identity and workspace membership shared by the API and the gateway
cmd/                 controller and api entrypoints
charts/              linux-desktop and research-vm Helm charts
crossplane/          XRDs, Azure compositions and package installs for workspace networks and VMs
infra/azure/         Terraform for one environment: AKS, network, firewall, ACR, identities, Flux
deploy/azure/        Flux stages: Crossplane, its packages, then the KubeTRE platform
images/              linux-desktop container image
config/              CRDs, RBAC, deployments, samples, and the access gateway (Kustomize)
test/e2e/            API + controller against a real API server
test/charts/         rendered charts checked against Pod Security admission and the VM XRD
test/crossplane/     compositions rendered and checked against the real Azure provider schemas
test/deploy/         GitOps tree checked against Terraform outputs and real Flux/Crossplane schemas
docs/adr/            Architecture decision records
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

To deploy an environment, follow [infra/azure/README.md](infra/azure/README.md): create the
state store, `terraform apply`, publish images and charts with `make acr-build publish-charts`,
then point Flux at this repository. The VM security model and its open gaps are in
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

KubeTRE is early-stage and has not yet been deployed to Azure. Read the open gaps in
[ADR 8](docs/adr/0008-vm-security-model.md) and [ADR 9](docs/adr/0009-access-gateway.md)
before using it for sensitive data.

Released under the [MIT License](LICENSE). KubeTRE is inspired by
[AzureTRE](https://github.com/microsoft/AzureTRE) (also MIT) and is not affiliated with or
endorsed by Microsoft.
