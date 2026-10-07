# 8. Security model for research VMs

Status: Accepted, 2026-10-07

## Decision

Research VMs are native cloud VMs. Each VM-enabled workspace gets its own subnet, and every
control below is created by Crossplane from definitions that only administrators can change.

| Threat | Control | Where |
|---|---|---|
| One workspace reaching another | NSG denies all traffic to and from `VirtualNetwork` except the workspace's own subnet and shared services | `composition-workspacenetwork.yaml` |
| Data leaving over the internet | Default route to Azure Firewall; only HTTPS to the workspace's FQDN allowlist and OS update tags is allowed | route association and rule collection group |
| Inbound access from anywhere | NSG admits only RDP and SSH from the access-gateway subnet; no public IP is ever composed | NSG rules, NIC template |
| Credentials on the VM | No managed identity; VM extensions disabled | `composition-researchvm.yaml` |
| Boot-level tampering | Trusted Launch: secure boot and vTPM | VM template |
| Data at rest | Encryption at host plus Azure platform encryption | VM template |
| A chart attaching a VM to another workspace | No subnet parameter exists; the subnet is selected by the VM's own namespace, and charts cannot create WorkspaceNetworks | XRD, deployer role |
| Runaway VM counts | `count/researchvms.platform.kubetre.io` in the workspace ResourceQuota | workspace template |

This mirrors AzureTRE's proven model of per-workspace network isolation, forced-tunnel egress
and a gateway as the only way in. Each VM also gets a hypervisor boundary, which is stronger
than the shared kernel of the container desktops.

These claims are tested: the rendered resources are validated against the Azure provider's
real schemas, and tests assert the deny rules, the absence of public IPs and identities, and
the subnet selector.

## Not yet in place

Do not treat the platform as production-ready until these exist:

1. **Deploying the access gateway.** It is built and tested (ADR 9): OIDC sign-in, owner-only
   sessions, targets confined to the workspace, clipboard and transfer disabled. It has not
   run in Azure yet, and its TLS certificate is still supplied by hand.
2. **A first deployment to Azure.** The AKS, VNet, firewall and route table infrastructure
   exists as Terraform in `infra/azure` with security tests, including platform firewall rules
   for Windows activation and OS updates, but it has not been applied yet.
3. **Package mirrors** so Linux VMs can install software without broad internet allowlists.
4. **Audit and detection**: NSG flow logs, firewall logs and Defender for Servers into Log
   Analytics.
5. **Data paths**: workspace storage and the airlock, which are the only sanctioned ways for
   data to enter or leave.
6. **DaemonSets on gateway nodes.** Any DaemonSet that tolerates every taint also gets an
   address in the gateway pod subnet, which VM NSGs trust. Audit them and restrict their egress.
7. **Container isolation**: container desktops still share a node kernel (ADR 2); sensitive
   workspaces may need dedicated node pools or sandboxed runtimes.
