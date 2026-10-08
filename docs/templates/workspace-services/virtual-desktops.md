# Virtual Desktops

**Template name:** `virtual-desktops` · **File:** `config/templates/servicetemplate-virtual-desktops.yaml`

The workspace service under which researchers create their own machines. It plays the part of
AzureTRE's Guacamole workspace service.

## How it differs from AzureTRE's Guacamole service

AzureTRE deploys an App Service running Guacamole in each workspace. KubeTRE has one shared
access gateway for the whole TRE: an OIDC broker, stock Apache Guacamole and guacd. Virtual
Desktops therefore installs nothing; it groups the workspace's VMs and desktops, and it is
ready in seconds.

The gateway decides every connection from current data:

- **Ownership.** A user can open a session only to a VM or desktop they own. Workspace owners
  can see every resource in the UI but cannot connect to other people's machines.
- **Target.** A connection must point inside its own workspace: an address in the workspace's
  VM subnet, or a Service in the workspace's namespace.
- **Data channels.** Clipboard in both directions, drive mapping, file transfer, printing and
  audio input are switched off.
- **Hand-off.** Guacamole receives a signed, encrypted, single-use session description that
  is valid for 60 seconds.

## Properties

| Property | Required | Updateable |
|---|---|---|
| Name for the workspace service | yes | yes |
| Description of the workspace service | yes | yes |
| Overview | no | yes |

## User resources

| Template | Runs on | Protocol |
|---|---|---|
| [Windows VM](../user-resources/windows-vm.md) | Azure VM | RDP |
| [Linux VM](../user-resources/linux-vm.md) | Azure VM | SSH |
| [Linux desktop](../user-resources/linux-desktop.md) | container in the workspace | VNC |
| [Linux VM on KubeVirt](../user-resources/linux-vm-kubevirt.md) | KubeVirt VM in the cluster | SSH |

Azure VM templates appear only in workspaces with a VM network, such as
[Workspace with virtual machines](../workspaces/vm-workspace.md). KubeVirt VMs appear only
when the environment enables KubeVirt.

## Rules

- Only workspace owners can add, change or remove Virtual Desktops.
- It must be deployed and enabled before anyone can create a machine under it.
- It cannot be deleted while it still has user resources.
