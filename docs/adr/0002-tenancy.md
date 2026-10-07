# 2. One namespace per workspace, default-deny

Status: Accepted, 2026-10-07. Review with the information security officer before production.

## Context
AzureTRE isolates each workspace in its own VNet peered to a hub with Azure Firewall.

## Decision
Each workspace gets a namespace `ws-<name>` with:
- the `restricted` Pod Security Standard,
- a ResourceQuota from its template and a LimitRange with defaults,
- a default-deny NetworkPolicy for ingress and egress, plus explicit allows for the same
  namespace, cluster DNS, namespaces labelled `kubetre.io/shared-services=true`, and ingress
  from namespaces labelled `kubetre.io/ingress=true`.

Researchers get no Kubernetes RBAC. Membership is enforced by the KubeTRE API.

## Consequences
- Namespaces share a kernel and a control plane. Workspaces holding identifiable health data
  may need stronger isolation: dedicated node pools with taints, gVisor or Kata runtime
  classes, or a vcluster per workspace. The Workspace API leaves room for a `tenancy` field.
- NetworkPolicy requires a CNI that enforces it. Cilium is the reference CNI (ADR 4).
