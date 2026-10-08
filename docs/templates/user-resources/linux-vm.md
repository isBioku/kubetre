# Linux VM

**Template name:** `linux-vm` · **Chart:** `charts/research-vm` · **Parent:** [Virtual Desktops](../workspace-services/virtual-desktops.md)

A personal Ubuntu 24.04 Azure VM with root access, opened in the browser as an SSH terminal.
It corresponds to AzureTRE's Guacamole Linux VM.

## Requirements

A workspace with a VM network, such as [Workspace with virtual machines](../workspaces/vm-workspace.md).

## Properties

| Property | Required | Updateable | Values |
|---|---|---|---|
| Name for the resource | yes | yes | |
| Description of the resource | yes | yes | |
| Overview | no | yes | |
| Size | no | no | `small`, `medium` (default), `large` |
| Disk (GiB) | no | no | 30 to 1024, default 64 |

Sizes map to Azure VM sizes as for the [Windows VM](windows-vm.md#properties).

## The VM

- **Image:** `Canonical` / `ubuntu-24_04-lts` / `server`, latest.
- **Account:** named after the owner, with sudo. Password generated and used by the gateway.
- **Hardening and network:** as for the [Windows VM](windows-vm.md#the-vm), with SSH instead
  of RDP.

## Differences from AzureTRE

AzureTRE's Linux VM offers a graphical desktop over xrdp. KubeTRE's Linux VM is a terminal.
For a graphical Linux desktop, use the [Linux desktop](linux-desktop.md).
