# Local development

Clusters run in the cloud only ([ADR 7](../adr/0007-cloud-targets.md)); there is no local
cluster to deploy to. Local work means building, running the tests, and running the API
against a real API server started by the tests or against your cloud cluster.

## Prerequisites

Go 1.27, GNU Make, Python 3 and Node.js 24 (for the UI). The Makefile downloads its own
`controller-gen`, `setup-envtest`, Kubernetes test binaries, Helm and Terraform into `bin/`.

## Build and generate

```sh
make generate   # deepcopy code, CRDs and controller RBAC from api/v1alpha1 and markers
make build      # bin/controller, bin/api, bin/gateway
```

Run `make generate` after changing a type or an RBAC marker, and commit the result.

## Test

| Command | Runs |
|---|---|
| `make test` | `go vet`, unit tests and the envtest suites below, against a real Kubernetes API server |
| `make infra-test` | Terraform format, validate and security tests, with a mocked Azure provider |
| `make ui-test` | AzureTRE's own UI test suite, type checks and KubeTRE's role tests |
| `make charts-lint` | `helm lint` for every chart |

The envtest suites:

| Suite | Checks |
|---|---|
| `internal/controller` | workspaces become ready with isolation; services install, are refused when unsafe, and clean up |
| `internal/tre`, `internal/api` | every endpoint, role and validation rule |
| `internal/gateway` | sign-in, ownership checks, target confinement, hand-off, cross-site refusals |
| `test/e2e` | the API and controller together; the AzureTRE API under its real RBAC |
| `test/charts` | charts render with administrator values only; Pod Security |
| `test/crossplane` | compositions render valid managed resources; the activation policy matches them |
| `test/deploy` | every Flux stage builds and validates against the real CRDs after substitution |

Some of AzureTRE's routing UI tests time out under heavy parallel load on slower machines,
upstream as well. Run `npx vitest run src/routing.test.tsx` on its own to confirm.

## Run the API locally

```sh
make run-api    # header-based development authentication; never expose it
curl -s localhost:8090/api/v1/me -H 'X-Dev-User: rita@example.org' -H 'X-Dev-Roles: TREAdmin'
```

It uses your current kubeconfig.

## Ship a change

1. **Version.** Bump `VERSION`, and `kubetre_version` in `infra/azure/variables.tf` to match.
   A test enforces this.
2. **Images.** Build with `make acr-build` and, if charts changed, run `make publish-charts`.
3. **Deploy.** Apply Terraform, then push. See [Upgrading](../admin/upgrading.md).
