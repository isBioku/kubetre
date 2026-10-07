# 4. FQDN egress allowlists through Cilium, failing closed

Status: Accepted, 2026-10-07

## Context
AzureTRE's Azure Firewall rules are mostly hostname allowlists per workspace and service.
Kubernetes NetworkPolicy cannot express hostnames.

## Decision
Templates and workspaces declare `egress.allowedFQDNs`. The controller renders a
`CiliumNetworkPolicy` allowing HTTPS to those names, with DNS proxied so Cilium can learn the
addresses. If Cilium is not installed, the controller reports `EgressPolicyReady=False` and the
default-deny policy blocks all internet egress. The workspace fails closed.

## Consequences
- Cilium is a hard requirement for any workspace that needs internet access.
- A stable egress IP per workspace, for data providers who allowlist sources, needs Cilium's
  egress gateway. Not yet implemented.
