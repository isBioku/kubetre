# 11. Optional Linux VMs on KubeVirt, beside Azure VMs

Status: Accepted, 2026-10-08. Revisits ADR 6b, which rejected KubeVirt.

## Decision

Environments can also offer Linux VMs that run inside the cluster on KubeVirt. Researchers
then choose, per VM, between an Azure VM and a KubeVirt VM: both are user resource templates
under Virtual Desktops in AzureTRE's UI. Azure VMs stay the default and the only Windows
option. KubeVirt is off unless `kubevirt_enabled` is set in `infra/azure`.

| | Azure VM (`research-vm`) | KubeVirt VM (`kubevirt-vm`) |
|---|---|---|
| Runs on | its own Azure VM | a pod on the tainted `kubevirt` node pool (nested virtualization) |
| Network isolation | workspace subnet, NSG and firewall rules | workspace namespace policies: default deny, Cilium FQDN egress |
| Access | guacd, RDP or SSH to the VM's private IP | guacd, SSH to the VM's Service in the workspace namespace |
| Disk | Azure managed disk | Azure disk through a PVC, copied once from an image in ACR by CDI |
| Windows | yes, licence included | no (needs separately licensed images) |
| Start-up and cost | minutes, billed per VM | faster, shares nodes |

## How it fits the existing controls

- **Pod Security.** KubeVirt is set to give VM pods the RuntimeDefault seccomp profile. With
  its non-root launcher, which drops all capabilities except NET_BIND_SERVICE, a VM pod meets
  the `restricted` level, so it runs in the same workspaces as everything else.
- **Installation rights.** Workspace charts still install as the confined `kubetre-deployer`,
  which may now create VirtualMachines and DataVolumes in its own namespace only.
- **Images.** KubeVirt, CDI and the Ubuntu disk image are mirrored into ACR
  (`make acr-mirror-kubevirt`). CDI copies the disk with the node's own registry credentials
  (`pullMethod: node`), so no new egress is needed.
- **Gateway.** guacd may reach workspace pods on SSH as well as VNC. The broker still accepts
  only a Service in the connection's own workspace namespace as a target.

## Consequences

- A KubeVirt VM's isolation is that of a pod. At the Azure Firewall it is indistinguishable
  from other pods on the shared pod subnet, so Cilium policies are its egress control. For
  the most sensitive data, Azure VMs remain the stronger choice.
- The KubeVirt pool needs a node size with nested virtualization; Dv5 and Dsv5 have it.
- KubeVirt and CDI are two more operators to upgrade (versions pinned in
  `deploy/azure/kubevirt` and the Makefile).
