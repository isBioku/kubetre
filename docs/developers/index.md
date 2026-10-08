# Developers

KubeTRE is a Go project built with controller-runtime, plus Helm charts, Crossplane
compositions, Terraform and Microsoft's AzureTRE UI. Conventions are in
[ADR 5](../adr/0005-go-and-kubebuilder-conventions.md).

## Repository layout

```text
api/v1alpha1/        CRD types; the CRDs in config/crd are generated from them
cmd/                 controller, api and gateway binaries
internal/controller/ reconcilers for Workspace and WorkspaceService
internal/tre/        AzureTRE-compatible API (/api), used by the UI
internal/api/        KubeTRE's own API (/api/v1) and token authentication
internal/access/     identities, workspace roles, VM account names
internal/gateway/    access gateway broker: sign-in, authorisation, Guacamole hand-off
internal/schema/     JSON Schema validation of template parameters
ui/                  AzureTRE's UI (Microsoft, MIT), vendored with three marked changes
charts/              Helm charts installed by service templates
config/              CRDs, RBAC, controller and API manifests, templates, gateway
crossplane/          XRDs and Azure compositions for workspace networks and VMs
deploy/azure/        the Flux stages for Azure
infra/azure/         Terraform for AKS, network, firewall, registry and identities
images/              container images, such as the Linux desktop
hack/                setup and maintenance scripts
test/                envtest suites for charts, Crossplane, deployment and end-to-end
docs/                this documentation and the architecture decisions
```

## Where to start

| You want to | Read |
|---|---|
| build and test locally | [Local development](local-development.md) |
| understand or change the API | [API](api.md) |
| change the UI | [UI](ui.md) |
| add a new tool for researchers | [Authoring templates](authoring-templates.md) |
