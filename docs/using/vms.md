# Using virtual machines

## Creating your machine

1. **Open Virtual Desktops.** Open your workspace, then its **Virtual Desktops** service. If
   there is none, ask the workspace owner to add it.
2. **Choose a template.** Under **Resources**, select **Create new**, then choose:

   | Template | Best for |
   |---|---|
   | Windows VM | Windows tools, a full Windows desktop |
   | Linux VM | an Ubuntu server you administer, as a terminal |
   | Linux desktop | a quick graphical Linux desktop, ready in about a minute |
   | Linux VM on KubeVirt | an Ubuntu terminal that starts faster than an Azure VM, if offered |

3. **Fill in the form.** Give a name and description, and choose a size and disk.
4. **Submit.** The card shows *deploying*, then *deployed*: five to ten minutes for an Azure
   VM, about a minute for a desktop.

Your account on the machine is named after your email address: `rita.smith@example.org`
becomes `rita-smith`.

## Connecting

Select **Connect** on your machine's card. A new tab opens and, the first time in a session,
asks you to sign in. The session opens in the browser.

- **Clipboard and files.** Copy and paste between your computer and the machine is switched
  off, as are file transfer and drive mapping. This keeps data inside the workspace.
- **Internet.** Machines can reach only the workspace's allowed internet domains, on HTTPS.
- **Your machines in one place.** `https://<your TRE>/gateway/` lists every machine you can
  connect to.

If **Connect** is greyed out, the machine is still deploying, updating or disabled.

## Who can see what

- **Researchers** see only their own machines.
- **Workspace owners** see every machine in the workspace, but can connect only to their
  own.

## Changing your machine

Use **Update** on the card's **...** menu to change its name, description or overview. Size
and disk are fixed at creation: delete the machine and create a new one to change them.

## Deleting your machine

1. **Disable.** From the card's **...** menu, select **Disable**, and wait until the card shows
   it is disabled.
2. **Delete.** From the same menu, select **Delete**.

Deleting removes the machine and its disk. Anything stored on it is lost.

## Following progress

- **Locked cards.** While an operation runs, the card shows a progress bar and is locked for
  changes.
- **Operations.** The bell icon at the top right lists current operations.
- **Refresh.** Use **Refresh** on the action bar to reload, or wait: pages refresh on their own
  while the tab is visible.

## Not available yet

AzureTRE lets you **Start**, **Stop** and **Reset password** on a VM. KubeTRE has none of
these yet. A machine runs, and costs money, until it is deleted.
