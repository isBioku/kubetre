# Linux VM on KubeVirt

**Template name:** `linux-vm-kubevirt` · **Chart:** `charts/kubevirt-vm` · **Parent:** [Virtual Desktops](../workspace-services/virtual-desktops.md)

A personal Ubuntu 24.04 VM that runs inside the cluster on KubeVirt, opened in the browser as
an SSH terminal. Researchers choose it instead of an Azure VM when they want a machine that
starts faster and shares nodes. It is optional: the environment must set
`kubevirt_enabled = true` (see [KubeVirt](../admin/kubevirt.md)).

## Properties

| Property | Required | Updateable | Values |
|---|---|---|---|
| Name for the resource | yes | yes | |
| Description of the resource | yes | yes | |
| Overview | no | yes | |
| Size | no | no | `small` (1 vCPU, 4 GiB), `medium` (2 vCPU, 8 GiB, default), `large` (3 vCPU, 12 GiB) |
| Disk (GiB) | no | no | 20 to 512, default 32 |

## The VM

- **Disk:** a persistent Azure disk, copied once from the Ubuntu image in the environment's
  registry by CDI. The first VM in a workspace takes a few minutes while the image is copied.
- **Account:** named after the owner, with sudo, set by cloud-init. Root login is disabled.
- **Placement:** only on the tainted `kubevirt` node pool.
- **Pod Security:** the VM's pod passes the `restricted` level.
- **Network:** the workspace's policies apply. Only guacd can reach the VM, and only on SSH.
  Outbound HTTPS only to the allowed domains.

## Compared with an Azure VM

| | Azure VM | KubeVirt VM |
|---|---|---|
| Isolation | own subnet, NSG and firewall rules | namespace network policies, like any pod |
| Windows | yes, licence included | no |
| Start-up | five to ten minutes | about a minute after the first disk copy |
| Billing | per VM | shares the KubeVirt nodes |

For sensitive data, prefer the Azure VM. [ADR 11](../adr/0011-kubevirt-vms.md) explains the
trade-off.
