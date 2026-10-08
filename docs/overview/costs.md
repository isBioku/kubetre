# Costs

## Cost reporting

!!! warning "Not available yet"
    AzureTRE shows costs on workspace, service and resource cards, from Azure Cost Management
    and resource tags. KubeTRE's cost endpoints return "not found", which the UI treats as
    "not supported", so no costs are shown.

A future design has to handle two kinds of cost:

- **Azure resources per workspace**, such as VMs, disks and subnets. Crossplane can tag these
  with workspace and resource IDs, as AzureTRE does.
- **Shared nodes.** Container desktops and KubeVirt VMs share cluster nodes, so Azure Cost
  Management cannot attribute them. They need allocation by namespace, for example from AKS
  cost analysis or OpenCost.

## What an environment costs to run

These run all the time, before anyone signs in:

- **Network:** Azure Firewall Standard.
- **Cluster:** AKS on the Standard tier, and Advanced Container Networking Services for FQDN
  filtering.
- **Nodes:** the system, work and gateway pools, plus the KubeVirt pool if enabled.
- **Registry and logs:** ACR Premium, and Log Analytics ingestion.

Research VMs are billed while they exist. Check the Azure pricing calculator for your region.

To keep costs down:

- **Small profile.** Apply with `-var-file=profiles/small.tfvars`: one node per pool and
  smaller VM sizes.
- **Delete unused machines.** Disable, then delete, VMs that are no longer needed.
- **Tear down.** Remove the environment when it is not in use; see [Tear-down](../admin/tear-down.md).

AzureTRE can stop and start a TRE, and researchers can stop and start their own VMs. KubeTRE
cannot do either yet.
