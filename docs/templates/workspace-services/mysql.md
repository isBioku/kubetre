# MySQL

**Template name:** `mysql` · **Kind:** workspace service · **Composition:** `crossplane/paas/azure/composition-mysqldatabase.yaml`

An Azure Database for MySQL flexible server for one workspace, the counterpart of AzureTRE's
MySQL workspace service.

| Property | Updateable | Notes |
|---|---|---|
| Size | yes | `B_Standard_B1ms` and up |
| Storage (GB) | yes | grows automatically |
| Database name | no | default `research` |
| Administrator password | yes | at least 12 characters, with upper and lower case letters and a digit |

## Security

- No public network access; a private endpoint in the workspace's subnet.
- **A password, set by you.** Entra sign-in for MySQL needs a server identity with
  Microsoft Graph permissions, which KubeTRE does not grant. The owner who creates the
  service chooses the `treadmin` password in the form. The API stores it in a Secret in the
  workspace namespace and never returns it. Leave the field empty when updating to keep it.

## Use it

From a workspace machine:

```bash
mysql -h mysql-1a2b3c4d5e.mysql.database.azure.com -u treadmin -p research
```

Create accounts for other members with `CREATE USER`.
