# Windows VM

**Template name:** `windows-vm` · **Chart:** `charts/research-vm` · **Parent:** [Virtual Desktops](../workspace-services/virtual-desktops.md)

A personal Windows Server 2022 Azure VM, opened in the browser over RDP. The Windows licence
is included in the VM's price. It corresponds to AzureTRE's Guacamole Windows VM.

## Requirements

A workspace with a VM network, such as [Workspace with virtual machines](../workspaces/vm-workspace.md).
The template is hidden elsewhere, and the controller refuses it in a workspace without one.

## Properties

| Property | Required | Updateable | Values |
|---|---|---|---|
| Name for the resource | yes | yes | |
| Description of the resource | yes | yes | |
| Overview | no | yes | |
| Size | no | no | `small`, `medium` (default), `large` |
| Disk (GiB) | no | no | 128 to 1024, default 128 |

Sizes map to Azure VM sizes set by the environment (`research_vm_sizes` in Terraform):

| Size | Default | Small profile |
|---|---|---|
| small | Standard_D2s_v5 | Standard_D2s_v5 |
| medium | Standard_D4s_v5 | Standard_D2s_v5 |
| large | Standard_D8s_v5 | Standard_D4s_v5 |

## The VM

- **Image:** `MicrosoftWindowsServer` / `WindowsServer` / `2022-datacenter-azure-edition`, latest.
- **Account:** named after the owner, from their email address (`rita.smith@example.org`
  becomes `rita-smith`). Names Azure reserves fall back to `researcher`. The name is set once,
  at creation. The password is generated, kept in the workspace, and never shown: the gateway
  signs the owner in.
- **Hardening:** Trusted Launch with Secure Boot and vTPM, encryption at host, no managed
  identity (the guest holds no Azure credentials), VM extensions disabled, no public IP.
- **Network:** a private IP in the workspace's subnet, reachable only from the gateway on RDP.
  Outbound HTTPS only to the workspace's allowed domains, through Azure Firewall.

## Lifecycle

- **Create:** five to ten minutes. The card shows *deploying* until Azure reports the VM.
- **Connect:** select **Connect** on the card. Only the VM's owner can connect.
- **Disable, then delete:** Crossplane deletes the VM, its disk and its network card.
