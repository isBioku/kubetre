# Unrestricted workspace

**Template name:** `unrestricted` · **File:** `config/templates/workspacetemplate-unrestricted.yaml`

A workspace for open, non-sensitive data, the counterpart of AzureTRE's Unrestricted
workspace. Machines and containers can reach the whole internet on any port, so **data can
leave it**. Never use it for sensitive data.

## What is different

It is a [Workspace with virtual machines](vm-workspace.md) with one change: egress is open.

| Layer | Restricted workspaces | Unrestricted workspace |
|---|---|---|
| Pods (Cilium) | HTTPS to allowed domains only | any port to the internet (`world`) |
| VM security group | HTTPS to the internet, then the firewall narrows it | all traffic to the internet |
| Firewall | HTTPS to allowed domains only | a network rule allowing everything from the workspace's subnet |

Everything else stays isolated:

- **Inbound.** Only the access gateway can reach the workspace's machines.
- **Other workspaces and the platform.** They stay unreachable: the security group still
  denies the VNet, and Cilium's `world` excludes the cluster.

## Properties

The same as [Workspace with virtual machines](vm-workspace.md). Allowed internet domains have
no effect, because everything is allowed.
