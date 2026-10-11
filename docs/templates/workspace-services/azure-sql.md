# Azure SQL

**Template name:** `azure-sql` · **Kind:** workspace service · **Composition:** `crossplane/paas/azure/composition-sqldatabase.yaml`

An Azure SQL logical server and database for one workspace, the counterpart of AzureTRE's
Azure SQL workspace service. See [Azure PaaS services](../../admin/paas.md) for what all
PaaS services share.

| Property | Updateable | Notes |
|---|---|---|
| Size | yes | `GP_S_Gen5_*` sizes are serverless and pause after an hour idle; `Basic`, `S0` and `S1` are fixed |
| Maximum size (GB) | yes | 1 to 1024 |
| Database name | no | default `research` |

## Security

- No public network access; a private endpoint in the workspace's subnet.
- **Entra ID only.** There are no SQL logins or passwords. The person who creates the
  service is the server's Entra administrator; they need to have signed in to the TRE.

## Use it

From a workspace machine, connect to the address on the card, such as
`sql-1a2b3c4d5e.database.windows.net`, with Entra ID authentication, for example:

```bash
sqlcmd -S sql-1a2b3c4d5e.database.windows.net -d research --authentication-method ActiveDirectoryDeviceCode
```

The administrator gives other members access in T-SQL:

```sql
CREATE USER [rita@example.org] FROM EXTERNAL PROVIDER;
ALTER ROLE db_datareader ADD MEMBER [rita@example.org];
```

AzureTRE's Azure SQL uses an SQL administrator password kept in the workspace's key vault.
KubeTRE has no passwords to hand out instead.
