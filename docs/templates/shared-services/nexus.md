# Nexus (package mirror)

**Template name:** `nexus` · **Kind:** shared service · **Chart:** `charts/nexus-mirror`

A read-only package mirror for the whole TRE, the counterpart of AzureTRE's Nexus shared
service. Machines install packages from it instead of reaching the internet.

| Repository | Format | Mirrors |
|---|---|---|
| `pypi` | Python | `https://pypi.org/` |
| `cran` | R | `https://cloud.r-project.org/` |
| `conda` | conda | `https://conda.anaconda.org/` |
| `npm` | npm | `https://registry.npmjs.org/` |
| `ubuntu` | apt (noble) | `http://archive.ubuntu.com/ubuntu/` |

## Add it

A TRE administrator opens **Shared Services** in the UI, selects **Create new** and chooses
**Nexus (package mirror)**. It is ready in a few minutes; the card then shows its addresses:

| Property | Use from |
|---|---|
| `address`, for example `http://10.224.9.20` | workspace VMs (Azure VMs and KubeVirt VMs) |
| `cluster_address` | desktops and other pods in workspaces |

## How it is built

- **Workload:** Sonatype Nexus Community Edition, one replica with a persistent volume. It runs
  in the `kubetre-shared` namespace under the restricted Pod Security level.
- **Setup:** after each install or upgrade, a job sets a generated admin password, creates the
  proxy repositories and allows anonymous read. Nobody needs credentials to install packages,
  and nobody can publish to it.
- **Address:** an internal load balancer at a fixed address in the shared services subnet,
  which workspace VM security groups allow on port 80.
- **Network:**
  - Inbound only from workspace namespaces and the workspace VM pool.
  - Outbound only to the upstream hosts in `mirror_upstream_fqdns`, enforced by a Cilium
    policy and the firewall.

## Properties

| Property | Updateable | Values |
|---|---|---|
| Storage (GiB) | no | 20 to 4096, default 100 |

## Limitations

- **HTTP only.** Traffic stays inside the private network, but it is not encrypted.
- **Fixed repositories.** The list is set by the chart's values; change them in the template.
- **Licence.** Nexus Community Edition has usage limits; check Sonatype's terms for your scale.
