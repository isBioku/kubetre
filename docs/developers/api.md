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

Validation follows AzureTRE: properties must match the template's JSON Schema, and on update,
fields not marked `updateable` cannot change. A workspace or service that does not exist, or
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
| `GET /api/workspaces/{ws}/roles` | yes | yes | yes |

### Airlock

As in AzureTRE, TRE administrators get no access without a workspace role.

| Endpoint | Owner | Researcher | Airlock manager |
|---|---|---|---|
| `GET /api/workspaces/{ws}/requests[/{id}]` | yes | yes | yes |
| `POST .../requests`, `.../submit`, `.../cancel` | yes | yes | no |
| `POST .../review`, `.../revoke`, `.../review-user-resource` | no | no | yes |
| `GET .../requests/{id}/link` | draft and approved | draft and approved | in review |
| `GET /api/requests` | own requests; with `airlock_manager=true`, every request in workspaces they manage | | |

### Costs

| Endpoint | Who |
|---|---|
| `GET /api/costs` | TRE administrators |
| `GET /api/workspaces/{ws}/costs` | the workspace's owners and TRE administrators |

Without cost reporting configured (no `--costs-client-id`), both return 404, which the UI
shows as "not supported". See [Costs](../overview/costs.md).

### Not implemented

| Endpoint | Response |
|---|---|
| `POST .../invoke-action`, template `POST`s, `/migrations` | not available |

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
