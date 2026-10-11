# OHDSI Atlas

**Template name:** `ohdsi` · **Kind:** workspace service · **Chart:** `charts/ohdsi`

OHDSI Atlas and WebAPI for defining cohorts and running analyses on data in the OMOP Common
Data Model, the counterpart of AzureTRE's OHDSI workspace service.

It needs a workspace with virtual machines. It runs in the workspace's namespace:

| Part | Image | Reached by |
|---|---|---|
| Front end (nginx) | `nginxinc/nginx-unprivileged` | the workspace's machines, through an internal load balancer in the workspace's subnet |
| Atlas | `ohdsi/atlas` | the front end, at `/atlas/` |
| WebAPI | `ohdsi/webapi` | the front end, at `/WebAPI/` |
| WebAPI's database (PostgreSQL) | `postgres` | WebAPI |

WebAPI's database holds Atlas's own data: cohort definitions, concept sets and job history.
Research data stays in its own database.

## Add it

A workspace owner opens the workspace, selects **Create new** under workspace services and
chooses **OHDSI Atlas**:

| Property | Updateable | Notes |
|---|---|---|
| WebAPI database storage (GiB) | no | 5 to 500, default 20 |
| WebAPI memory (GiB) | yes | 2, 4 or 8 |

WebAPI creates its tables on first start, which takes a few minutes.

## Use it

Open the address on the service's card, such as `http://10.240.0.13/atlas/`, in a browser on
a workspace machine.

To analyse data, add a CDM source in Atlas, under **Configuration**, **Sources**: give the
JDBC connection to a database that holds your OMOP CDM, vocabulary and results schemas, such
as an Azure SQL or PostgreSQL database the workspace can reach.

## Differences from AzureTRE

- **Sign-in.** WebAPI's security is turned off: everyone who can open Atlas can change its
  definitions and sources. Only the workspace's machines can reach it. AzureTRE signs users in
  with Entra ID.
- **Hosting.** Atlas and WebAPI run in the cluster, not on App Service, and WebAPI's database
  is PostgreSQL in the cluster, not Azure Database for PostgreSQL.
- **Data sources.** AzureTRE can create a Synapse data source with the service. In KubeTRE
  sources are added in Atlas.
- **HTTP.** Traffic stays inside the workspace's subnet but is not encrypted.
