# API

The KubeTRE API (`cmd/api`) serves two APIs on one port:

| Path | API | Code | Used by |
|---|---|---|---|
| `/api/...` | AzureTRE's REST API | `internal/tre` | AzureTRE's UI |
| `/api/v1/...` | KubeTRE's own API | `internal/api` | scripts |

Both validate the caller's OpenID Connect token: signature, issuer, audience and expiry.
TRE roles come from the token's roles claim, and workspace roles from the `Workspace`
resource. There is no Swagger UI.

## How AzureTRE's API is implemented

| AzureTRE concept | KubeTRE |
|---|---|
| Workspace document | `Workspace` resource; `id` is its name |
| Workspace service, user resource | `WorkspaceService`; a user resource has `parentService` |
| `properties` | template parameters or chart values, plus name, description and overview |
| `_etag` | the resource's `resourceVersion`; a stale `etag` header gets 409 |
| `isEnabled` | `spec.enabled`; delete requires it to be false |
| `deploymentStatus` | derived from the resource's phase and generation |
| Operation | derived from the resource's UID, generation and phase; the ID is `<uid>_<action>_<generation>` |
| `scope_id` | the API's own application ID URI; all workspaces share one audience |
| Workspace roles | `userRoles` on the workspace, a KubeTRE extension the UI reads |
| `connection_uri` | `https://<host>/gateway/connect/<workspace>/<resource>` for machines |

Validation follows AzureTRE: properties must match the template's JSON Schema, and on update
of a service or user resource, fields not marked `updateable` cannot change. Workspace
parameters are not yet checked for `updateable` by the API, only by the form. A workspace or service that does not exist, or
that the caller cannot see, returns 404, so names do not leak.

## Permissions map

Who may call each endpoint. "Member" means any workspace role. A TRE administrator without a
workspace role is treated as a member for reading.

### Core

| Endpoint | TRE admin | TRE user |
|---|---|---|
| `GET /api/.metadata`, `GET /api/health` | yes, no sign-in | yes, no sign-in |
| `GET /api/workspace-templates[/{name}]` | yes | yes |
| `GET /api/workspace-service-templates[/{name}]` | yes | yes |
| `GET /api/workspace-service-templates/{parent}/user-resource-templates/{name}` | yes | yes |
| `GET /api/workspaces` | all | own workspaces |
| `GET /api/workspaces/{ws}`, `/scopeid` | yes | members |
| `POST /api/workspaces` | yes | no |
| `PATCH`, `DELETE /api/workspaces/{ws}` | yes | no |
| `GET /api/workspaces/{ws}/operations[/{op}]`, `/history` | yes | members |
| `GET /api/operations` | own in-progress operations | own in-progress operations |

### Workspace

| Endpoint | Owner | Researcher | Airlock manager |
|---|---|---|---|
| `GET .../workspace-services[/{svc}]` | yes | yes | yes |
| `POST`, `PATCH`, `DELETE .../workspace-services[/{svc}]` | yes (and TRE admins) | no | no |
| `GET .../workspace-services/{svc}/operations[/{op}]` | yes | yes | yes |
| `GET .../workspace-service-templates/{parent}/user-resource-templates` | yes | yes | yes |
| `GET .../user-resources` | all | own | own |
| `GET .../user-resources/{ur}` | all | own | own |
| `POST .../user-resources` | yes | yes | no |
| `PATCH .../user-resources/{ur}` | all | own | own |
| `DELETE .../user-resources/{ur}` | all (and TRE admins) | own | own |
| `GET .../user-resources/{ur}/operations[/{op}]` | all | own | own |
| `GET /api/workspaces/{ws}/users` | yes (and TRE admins) | no | no |
| `GET /api/workspaces/{ws}/roles`, `/requests` | yes | yes | yes |

### Not implemented

| Endpoint | Response |
|---|---|
| `GET /api/costs`, `/api/workspaces/{ws}/costs` | 404, shown by the UI as "not supported" |
| `GET /api/shared-services`, `/api/shared-service-templates` | empty lists |
| `GET /api/requests`, `/api/workspaces/{ws}/requests` | empty lists |
| `POST .../invoke-action`, template `POST`s, airlock `POST`s, `/migrations` | not available |

## KubeTRE's own API

| Endpoint | Purpose |
|---|---|
| `GET /api/v1/me` | the caller's identity and whether they are a TRE administrator |
| `GET /api/v1/templates`, `/service-templates` | templates and their form schemas |
| `GET`, `POST /api/v1/workspaces`; `GET`, `DELETE /api/v1/workspaces/{name}` | workspaces |
| `GET`, `POST /api/v1/workspaces/{name}/services`; `GET`, `DELETE .../services/{service}` | services |

```sh
TOKEN=$(az account get-access-token --scope api://<api app id>/user_impersonation --query accessToken -o tsv)
curl -s https://<host>/api/v1/me -H "Authorization: Bearer $TOKEN"
curl -s https://<host>/api/workspaces -H "Authorization: Bearer $TOKEN"
```

## Permissions in the cluster

The API's service account `kubetre-api` can read templates and create, update and delete
`Workspace` and `WorkspaceService` resources, nothing else (`config/rbac/api_role.yaml`). An
end-to-end test runs every UI action under exactly that role.
