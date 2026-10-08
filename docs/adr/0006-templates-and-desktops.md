# 6. Templates and research desktops

## 6a. Workspace services are Helm charts

Status: Accepted, 2026-10-07

Workspace services are Helm charts published to the cloud registry (ACR, ECR or Artifact
Registry) as OCI artifacts. A `ServiceTemplate` names a chart and version, fixed admin values,
and a user-facing `valuesSchema`. The API validates user values against that schema; keep
`additionalProperties: false` so users cannot override chart internals. The chart's own
`values.schema.json` validates the full merged values at install time.

The controller creates a Flux `OCIRepository` and `HelmRelease` per `WorkspaceService`. Flux
authenticates to the registry with workload identity (`provider: azure|aws|gcp`) and installs
the chart by impersonating the workspace's `kubetre-deployer` service account. That account can
run workloads, VMs and disks in its own namespace, but cannot change network policies, quotas,
limit ranges or RBAC, so a chart cannot weaken the workspace's isolation.

## 6b. Desktops: Linux containers and native cloud VMs

Status: Accepted, 2026-10-07. Supersedes the earlier KubeVirt decision.

- **Linux container desktop** (`charts/linux-desktop`): XFCE in the browser through noVNC,
  non-root, read-only root filesystem, admitted under the `restricted` Pod Security level
  (verified by `test/charts`). For researchers who do not need a full machine.
- **Windows and Linux VMs** (`charts/research-vm`): real Azure VMs (EC2 later), requested
  through a Crossplane `ResearchVM` and built by a per-cloud Composition. See ADR 8 for the
  network and hardening model.

KubeVirt was rejected (since revisited for optional Linux VMs: see ADR 11) because Windows guests nested in AKS nodes need separately licensed
Windows with restricted hosting rights, nested virtualization costs performance and limits
node choice, and GPUs are hard to pass through. Native VMs avoid all three.

Per-user access to a desktop is currently protected by a generated password. Identity-aware
access through the access gateway (Guacamole with OIDC) is required before production.
