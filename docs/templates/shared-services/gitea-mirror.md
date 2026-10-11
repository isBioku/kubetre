# Gitea (source mirror)

**Template name:** `gitea-mirror` · **Kind:** shared service · **Chart:** `charts/gitea`

Read-only mirrors of public Git repositories for the whole TRE, the counterpart of AzureTRE's
Gitea shared service. Researchers clone from the mirror; nothing can be pushed out through it.

## Add it

A TRE administrator opens **Shared Services**, selects **Create new**, chooses **Gitea (source
mirror)** and lists the repositories to mirror:

| Property | Updateable | Notes |
|---|---|---|
| Repositories to mirror | yes | name and public `https://` clone URL for each |
| Refresh interval | yes | for example `8h` |
| Storage (GiB) | no | 10 to 2048, default 50 |

To add a repository later, **Update** the shared service and add it to the list. Removing one
from the list does not delete its mirror.

## Use it

From a workspace machine, clone from the address on the shared service's card:

```sh
git clone http://10.224.9.21/kubetre-admin/<repository>.git
```

Browse the mirrors at the same address. Sign-in is not needed to read.

## How it is built

- **Workload:** Gitea, rootless, one replica, SQLite on a persistent volume, in `kubetre-shared`.
- **Lockdown:**
  - Registration is closed, so no one can create repositories.
  - SSH and webhooks are off.
  - Migrations are allowed only from GitHub and GitLab.
- **Setup:** after each install or upgrade, a job creates any missing pull mirrors with the
  generated admin account.
- **Address:** an internal load balancer at a fixed address in the shared services subnet.
- **Network:** the same boundary as [Nexus](nexus.md). Upstream access is limited to the
  GitHub hosts in `mirror_upstream_fqdns`.
