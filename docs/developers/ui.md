# UI

KubeTRE's UI is Microsoft's AzureTRE UI: React 18, Fluent UI, MSAL and React JSON Schema
Form, vendored in `ui/` from an upstream commit under its MIT licence. It is kept as close to
upstream as possible so it looks and behaves exactly like AzureTRE.

## Changes from upstream

Each change is marked `Modified for KubeTRE` in the source:

1. **Runtime configuration.** `src/config.ts` and `index.html` load settings from `/config.js`
   at runtime instead of a `config.json` built into the bundle. One image serves every
   environment.
2. **Workspace roles.** `src/hooks/useAuthApiCall.ts` reads a workspace's roles from the API's
   `userRoles`. AzureTRE decodes them from a token for the workspace's own app registration,
   which KubeTRE does not have. TRE-wide roles still come from the token.
3. **Container image.** `Dockerfile` and `nginx.conf`: nginx, unprivileged, read-only root
   file system.

One upstream test that validates AzureTRE's Porter template schemas is removed, because those
templates are not in this repository. `ui/README.md` records the upstream commit.

## Configuration

The `kubetre-ui` ConfigMap in `kubetre-gateway` holds `config.js`. The Azure overlay fills it
from Terraform:

| Setting | Value |
|---|---|
| `rootClientId` | `ui_client_id` |
| `rootTenantId` | the tenant |
| `treApplicationId` | `api://<oidc_audience>` |
| `treUrl` | `/api` |
| `treId` | `name` |
| `version` | `kubetre_version` |
| `uiSiteName`, `uiFooterText` | "Azure TRE", "Azure Trusted Research Environment" |
| `userManagementEnabled` | `false`: members are set in the workspace form |

## Develop

```sh
cd ui
npm ci
npx tsc --noEmit -p .
npx vitest run
```

To run the UI against a live environment, create `public/config.js` with that environment's
settings, add `http://localhost:3000` as a redirect URI on the UI app registration, and run
`npx vite`.

## Update from upstream

Copy the new upstream `ui/app`, keep `LICENSE`, reapply the three marked changes, and run
`make ui-test`. Record the new upstream commit in `ui/README.md`.
