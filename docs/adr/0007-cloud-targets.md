# 7. Cloud targets: AKS first, then EKS and GKE

Status: Accepted, 2026-10-07

KubeTRE runs only on managed cloud Kubernetes. AKS is the primary target. Nothing is hosted on
developer machines; images are built by the cloud registry (`make acr-build`) and charts are
pushed there (`make publish-charts`). Unit and envtest tests may run anywhere.

## What each cloud must provide

| Need | AKS | EKS | GKE |
|---|---|---|---|
| FQDN egress for pods | Azure CNI powered by Cilium with Advanced Container Networking Services (paid add-on) | Self-installed Cilium | Dataplane V2 uses its own `FQDNNetworkPolicy`; the controller does not support it yet |
| Egress firewall for VMs | Azure Firewall with a route table | AWS Network Firewall | Cloud NGFW |
| Research VMs | Crossplane `upbound/provider-azure-compute` | Composition to write, `provider-aws-ec2` | Composition to write, `provider-gcp-compute` |
| Chart and image registry | ACR attached to the cluster, Flux `provider: azure` | ECR, `provider: aws` | Artifact Registry, `provider: gcp` |
| Disks for container desktops | Azure Disk CSI | EBS CSI | Persistent Disk CSI |
| Flux | AKS GitOps extension or upstream install | Upstream install | Upstream install |

Only the Compositions and the EnvironmentConfig differ per cloud. The XRDs, charts, API and
controller are cloud-neutral.

## Required AKS layout

Implemented by `infra/azure` (Terraform, azurerm 5.8) and `deploy/azure` (Flux). Summary:

- Azure CNI with a routable pod subnet (not overlay) for the **access-gateway node pool**, so
  the VM network security groups can admit RDP and SSH from that subnet alone.
- A VNet address range reserved for workspace VM subnets, passed to the controller as
  `--vm-address-pool` and not overlapping node, pod or service ranges.
- Azure Firewall with a policy and a route table whose default route points at it.
- Crossplane with the packages in `crossplane/install/`, using workload identity scoped to the
  workspace resource group, the VNet, the route table and the firewall policy.
- Cluster egress itself through the firewall (`outbound_type = userDefinedRouting`), with the
  firewall's public IP added to the API server's authorized ranges.
- Flux delivers three stages in order: Crossplane, its packages, then KubeTRE. Terraform passes
  environment values as Flux substitution variables; `test/deploy` checks they match.

## Constraints to verify on the first AKS deployment

- That ACNS FQDN filtering accepts the `CiliumNetworkPolicy` shape the controller renders.
- That the Upbound provider packages at the pinned versions are pullable without an Upbound
  subscription; if not, build them from `crossplane-contrib/provider-upjet-azure`.
- That `EncryptionAtHost` is registered on the subscription, or set `encryptionAtHost: false`.
