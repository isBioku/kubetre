# 10. Run AzureTRE's own UI against an AzureTRE-compatible API

Status: Accepted, 2026-10-08.

## Decision

KubeTRE's user interface is Microsoft's AzureTRE UI, vendored in `ui/` under its MIT licence,
with Microsoft's branding, layout and wording kept. KubeTRE serves the parts of AzureTRE's REST
API the UI uses (`internal/tre`), so workspaces, workspace services and user resources are all
created from the UI, as in AzureTRE.

| AzureTRE | KubeTRE |
|---|---|
| Workspace document in Cosmos DB | `Workspace` resource |
| Workspace service, user resource | `WorkspaceService`; a user resource has `parentService` set |
| Template registered from a Porter bundle | `WorkspaceTemplate`, `ServiceTemplate` (`kind: UserResource`, `parentTemplate`) |
| Guacamole workspace service | `virtual-desktops`, a template with no chart |
| Operation documents written by the resource processor | Operations derived from the resource's UID, generation and phase |
| Workspace roles in a per-workspace app registration's token | Membership on the `Workspace`, returned as `userRoles` |
| `connection_uri` to the workspace's Guacamole | `https://<host>/gateway/connect/<workspace>/<resource>` on the shared broker |

## Changes to the UI

Three, each marked "Modified for KubeTRE" in the source:

1. Configuration is read at runtime from `/config.js`, so one image serves every environment.
2. A workspace's roles come from the API's `userRoles`. AzureTRE decodes them from a token for
   the workspace's own app registration. KubeTRE has one API audience, so that token carries
   no workspace roles. TRE-wide roles (`TREAdmin`, `TREUser`) still come from the token.
3. A container image (nginx, unprivileged, read-only root file system).

AzureTRE's UI test suite runs unchanged (`make ui-test`), plus tests for the role lookup.

## Behaviour kept from AzureTRE

- Only TRE administrators create workspaces. Workspace owners add workspace services.
  Owners and researchers create user resources under a deployed, enabled service.
- Researchers see only their own user resources. Owners see all of them.
- A resource must be disabled before it is deleted. Updates send the `_etag` and fail on
  conflict. Template fields marked `updateable` are the only ones that change after creation.
- Callers outside a workspace get 404, not 403, so workspace and resource names do not leak.

## Not yet supported

Airlock (workspaces report `enable_airlock: false`, so the UI hides it), shared services, costs
(the API returns 404, which the UI treats as "not supported"), resource history, template
upgrades and custom actions such as starting and stopping VMs. User management stays in the
workspace form rather than AzureTRE's Microsoft Graph-backed Users page.

## Consequences

- The connect link is a GET opened in a new tab, so the broker accepts GET `/connect` only when
  the browser's Fetch Metadata says the navigation came from this site or was typed by the
  user. After sign-in it returns only to a connect path of its own.
- The broker moved from `/` to `/gateway`, and the UI is at `/`. Envoy Gateway routes by the
  longest prefix.
- An operation's status is computed, not stored, so there is no operation history after a
  resource is deleted. The UI only needs the current status to finish its notifications.
