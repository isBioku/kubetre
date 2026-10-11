# Costs

## Cost reporting

The UI shows costs on workspace, service and resource cards, as in AzureTRE: TRE
administrators see every workspace's month-to-date cost, and workspace owners see their
workspace's, broken down by service and resource. Researchers see no costs.

The API implements AzureTRE's `GET /api/costs` and `GET /api/workspaces/{ws}/costs`, with
the same `from_date`, `to_date` and `granularity` (`None` or `Daily`) parameters. It reads
Azure Cost Management's actual costs for the TRE's three resource groups with its own
identity, `id-<name>-costs`, which holds only **Cost Management Reader**. Results are kept
for two hours, as AzureTRE keeps them; Cost Management itself updates a few times a day.

| Cost | Comes from |
|---|---|
| A workspace | Azure resources tagged `kubetre-workspace` (VMs, NICs, security groups, PaaS services), plus its share of the cluster |
| A workspace service | Azure resources tagged `kubetre-service` |
| A user resource | Azure resources tagged `kubetre-vm` or `kubetre-service` with its name |
| Core services | the platform's resource group, the cluster not reserved by workspaces, and workspace resources Azure does not tag |

**The cluster share is an estimate.** Container desktops, KubeVirt VMs, Gitea and OHDSI run on
shared nodes, which Azure bills to the AKS node resource group as a whole. KubeTRE divides
that cost between workspaces in proportion to the CPU and memory their quotas show as
requested, averaged, out of what the nodes can allocate. It uses today's requests for the
whole period, so a workspace that grew this week is charged as if it had always been that
size.

**Untagged resources.** Azure does not copy a VM's tags to its OS disk, so disks count as
core costs, as they would in AzureTRE.

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
