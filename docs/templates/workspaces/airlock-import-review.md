# Airlock Import Review workspace

**Template name:** `airlock-import-review` · **File:** `config/templates/workspacetemplate-airlock-import-review.yaml`

The workspace where airlock managers inspect files being imported, before they reach a
research workspace. It matches AzureTRE's Airlock Import Review workspace.

Its machines can reach the external airlock account on HTTPS, where imports wait for review,
and nothing else on the internet. Like every workspace, only the access gateway can reach its
machines.

## Setting it up

1. As a TRE administrator, create a workspace from **Airlock Import Review workspace**. Add
   the TRE's airlock managers as its researchers or owners, so they can create VMs in it.
2. In it, add a **Virtual Desktops** workspace service and note its ID.
3. In each research workspace, set **Airlock review VMs**, **Import review**:
    - **Import review workspace ID:** this workspace's ID.
    - **Virtual Desktops service ID in that workspace:** the service from step 2.
    - **Review VM template:** for example `linux-vm` or `windows-vm`.

See [Airlock](../../overview/airlock.md).

## Properties

None beyond the name and description.
