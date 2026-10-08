# Using KubeTRE

KubeTRE uses AzureTRE's UI, so anyone who has used AzureTRE already knows how to use it.
This section covers what researchers and workspace owners do day to day.

| You are | You can | Read |
|---|---|---|
| Researcher | create your own machines, connect to them, delete them | [Using virtual machines](vms.md) |
| Workspace owner | add workspace services, see all machines in the workspace | [Using virtual machines](vms.md), [Virtual Desktops](../templates/workspace-services/virtual-desktops.md) |
| TRE administrator | create workspaces and choose their members | [First workspace](../quickstart/first-workspace.md), [Workspace templates](../templates/index.md) |

## Signing in

Open your TRE's address, for example `https://tre.example.org`, and sign in with your
organisation's account. You need the `TREUser` or `TREAdmin` role; ask your TRE administrator
if the UI says you have no access. Multi-factor authentication follows your organisation's
Entra ID policy.

The **Workspaces** page lists the workspaces you belong to. To be added to a workspace, ask
its owner or a TRE administrator to add your email address to it.

## Moving data

There is no airlock yet, so there is no sanctioned way to bring files into a workspace or take
results out. Data your project needs must be provided inside the workspace by your
administrators. Machines can download from the workspace's allowed internet domains, such as
package repositories.
