# Linux desktop

**Template name:** `linux-desktop` · **Chart:** `charts/linux-desktop` · **Parent:** [Virtual Desktops](../workspace-services/virtual-desktops.md)

A personal XFCE desktop that runs as a container in the workspace and opens in the browser
over VNC. It starts in about a minute and needs no VM network, so it works in any workspace.
AzureTRE has no direct equivalent.

## Properties

| Property | Required | Updateable | Values |
|---|---|---|---|
| Name for the resource | yes | yes | |
| Description of the resource | yes | yes | |
| Overview | no | yes | |
| Size | no | no | `small` (1 CPU, 2 GiB, default), `medium` (2 CPU, 4 GiB), `large` (4 CPU, 8 GiB) |
| Home directory (GiB) | no | no | 10 to 200, default 20 |

## The desktop

- **Image:** `kubetre/linux-desktop` from the environment's registry (`images/linux-desktop`).
- **Security:** runs as non-root with a read-only root file system, and passes the
  `restricted` Pod Security level.
- **Storage:** the home directory is a persistent volume that survives restarts.
- **Network:** the workspace's policies apply: no inbound except from guacd, outbound HTTPS
  only to the allowed domains.

## Limitations

A container shares the node's kernel with other workloads. Use a VM where that separation
matters.
