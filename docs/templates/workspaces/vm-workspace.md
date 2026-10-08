# Workspace with virtual machines

**Template name:** `vm-workspace` · **File:** `config/templates/workspacetemplate-vm.yaml`

A workspace for researchers who need their own Windows or Linux machines. Besides everything
in the [Base workspace](base.md), it gives the workspace its own Azure network so that
Azure VMs created in it are isolated from every other workspace.

## What it creates

- **Namespace:** everything the [Base workspace](base.md) creates.
- **VM network:** a `/26` subnet allocated from the VM address pool (`10.240.0.0/16` by
  default), created by Crossplane in the platform VNet.
- **Network security group:**
  - Inbound: from the access gateway's pod subnet on SSH and RDP only, and within the subnet.
  - Outbound: within the subnet, HTTPS to the internet, and Windows activation on TCP 1688.
  - Everything else denied, including the rest of the virtual network.
- **Route table association:** all traffic leaves through Azure Firewall.
- **Firewall rule collection group** `rcg-ws-<id>`: HTTPS only to the workspace's allowed
  domains, plus Windows Update for the workspace's VMs.

The workspace shows *deploying* until Crossplane reports the subnet and rules ready, which
takes a few minutes.

## Quota

| Resource | Limit |
|---|---|
| CPU requests | 8 |
| Memory requests | 32 GiB |
| Persistent volume claims | 10 |
| Storage requests | 500 GiB |
| Azure VMs | 20 |

## Properties

The same as the [Base workspace](base.md), without Data classification. The template already
allows `pypi.org`, `files.pythonhosted.org` and `cran.r-project.org`; the workspace's own
allowed domains are added to these.

## Limitations

- The VM address pool sets how many VM workspaces fit: 1,024 with the defaults.
- Workspace subnets are created in the platform VNet, so the platform's address plan must
  leave room for the pool.
