# 8. Virtual desktops

Researchers get machines through the **Virtual Desktops** workspace service, the counterpart
of AzureTRE's Guacamole service. It is already registered with its user resources.

## Add Virtual Desktops

A workspace owner does this once per workspace:

1. **Open the workspace.**
2. **Add the service.** Under **Workspace Services**, select **Create new**, then **Create**
   under **Virtual Desktops**.
3. **Name it and submit.**

It is ready in seconds, because it installs nothing: remote sessions go through the TRE's
shared access gateway.

## Create a VM

A researcher, or the owner, does this for their own machine:

1. **Open Virtual Desktops.** Open the workspace, then **Virtual Desktops**.
2. **Choose a machine.** Under **Resources**, select **Create new**, then pick a template:
   - **Windows VM (Windows Server 2022)**: an Azure VM, over RDP.
   - **Linux VM (Ubuntu 24.04)**: an Azure VM, as an SSH terminal.
   - **Linux desktop**: an XFCE desktop in a container, over VNC.
   - **Linux VM on KubeVirt (Ubuntu 24.04)**: shown only when KubeVirt is enabled.
3. **Fill in the form.** Give a name and description, and choose a size and disk.
4. **Submit.** Then select **Go to resource**.

An Azure VM takes five to ten minutes; the card shows *deploying*, then *deployed*.

## Connect

Select **Connect** on the machine's card. A new tab opens `https://<host>/gateway/connect/...`:

1. **Sign in.** The gateway asks you to sign in, the first time in a session.
2. **Check.** It confirms you own the machine and that the machine lies inside the workspace.
3. **Hand off.** It hands a single-use session to Guacamole.

You are signed in to the machine with an account named after your email address. Clipboard,
drive mapping and file transfer are off.

`https://<host>/gateway/` lists every machine you can connect to, across workspaces.

## Rules to know

- **Visibility.** Researchers see only their own machines. Owners see every machine, but can
  connect only to their own.
- **Deleting.** Disable a machine before deleting it, from the card's **...** menu, as in
  AzureTRE. Deleting a VM removes its disk and network card.
- **No power actions.** There is no Start, Stop or Reset password action yet.
