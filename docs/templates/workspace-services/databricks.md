# Azure Databricks

**Template name:** `databricks` · **Kind:** workspace service · **Composition:** `crossplane/paas/azure/composition-databricksworkspace.yaml`

An Azure Databricks workspace (premium) for one workspace, the counterpart of AzureTRE's
Azure Databricks workspace service.

## Network

- **VNet injection.** KubeTRE gives the service two more subnets from the VM address pool,
  for Databricks' host and container networks, and releases them when it is deleted.
  Clusters get no public IPs.
- **Egress through the firewall.** The subnets route through Azure Firewall. The service's
  own rule collection allows only the Databricks control plane, its metastore, storage and
  Event Hubs, by service tag. Its security group allows nothing else.
- **No public network access.** The UI and API are reached through two private endpoints in
  the workspace's subnet, for the API and for browser sign-in.

## Access

Workspace owners get **Contributor** on the Databricks workspace, which makes them its
administrators. They add researchers in the Databricks workspace's settings.

## Use it

Open the address on the card, `https://adb-....azuredatabricks.net/`, in a browser on a
workspace machine. It appears once Azure has created the workspace, which takes several
minutes.
