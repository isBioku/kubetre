# Base workspace

**Template name:** `base` · **File:** `config/templates/workspacetemplate-base.yaml`

An isolated workspace for container-based work, with no virtual machines. It is the KubeTRE
counterpart of AzureTRE's Base workspace.

## What it creates

Every workspace, whatever its template, gets a Kubernetes namespace `ws-<id>` containing:

- **Isolation:** default-deny NetworkPolicies in both directions, an allowance for DNS, and
  ingress only from the access gateway's guacd pods.
- **Egress:** a Cilium policy allowing HTTPS only to the workspace's allowed domains.
- **Limits:** a ResourceQuota and a LimitRange with default container limits.
- **Pod Security:** the `restricted` level is enforced.
- **Deployer:** a `kubetre-deployer` service account, through which Flux installs the
  workspace's services. It cannot change any of the above.

## Quota

| Resource | Limit |
|---|---|
| CPU requests | 8 |
| Memory requests | 32 GiB |
| CPU limits | 16 |
| Memory limits | 64 GiB |
| Persistent volume claims | 10 |
| Storage requests | 500 GiB |

## Properties

| Property | Required | Updateable | Notes |
|---|---|---|---|
| Name for the workspace | yes | yes | Display name. The workspace ID is generated from it. |
| Description of the workspace | yes | yes | |
| Workspace Overview | no | yes | Markdown, shown on the workspace page. |
| Workspace owners | no | yes | Email addresses. Defaults to the administrator who creates it. |
| Workspace researchers | no | yes | Email addresses. |
| Allowed internet domains | no | yes | Hostnames reachable on HTTPS. `*.example.org` allows subdomains. |
| Cost centre | yes | no | Must match `CC-` followed by four digits. |
| Data classification | no | no | `open`, `internal` or `sensitive` (default). |

The template allows no internet domains of its own.

## Limitations

- No virtual machines: use [Workspace with virtual machines](vm-workspace.md) for those.
- No airlock and no storage account per workspace yet.
