# Gitea

**Template name:** `gitea` · **Kind:** workspace service · **Chart:** `charts/gitea` (workspace mode)

A private Git server for one workspace, the counterpart of AzureTRE's Gitea workspace service.
Members keep code and notebooks under version control without it leaving the workspace.

It needs a workspace with virtual machines. It is reached only from the workspace's machines,
through an internal load balancer in the workspace's subnet; its pod is the only one in the
workspace that those machines may reach.

## Add it

A workspace owner opens the workspace, selects **Create new** under workspace services and
chooses **Gitea**:

| Property | Updateable | Notes |
|---|---|---|
| Storage (GiB) | no | 10 to 2048, default 50 |
| Import repositories from | yes | hosts members may import from, for example the shared Gitea mirror's address; empty allows any host the workspace can reach |

## Use it

The service's card shows its address, such as `http://10.240.0.12`. The UI marks it as
internal: open it in a browser on a workspace machine, not on your own computer.

1. Select **Register** and create your account. Only machines in the workspace can reach
   Gitea, so only its members can register.
2. Create repositories, or import one from the shared Gitea mirror.
3. Clone over HTTP from the workspace's machines:

    ```bash
    git clone http://10.240.0.12/<user>/<repository>.git
    ```

New repositories are private by default.

## Differences from AzureTRE

- **Accounts.** AzureTRE signs members in with the workspace's Entra application. KubeTRE has
  no per-workspace applications, and Entra ID requires HTTPS redirect addresses, so members
  register local accounts. The network limits registration to the workspace.
- **HTTP.** Traffic between the machines and Gitea stays inside the workspace's subnet but is
  not encrypted. AzureTRE serves Gitea over HTTPS from App Service.
- **Storage.** Repositories are on a managed disk in the cluster, with an SQLite database,
  rather than Azure Files and MySQL.
