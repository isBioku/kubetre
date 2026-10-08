# AzureTRE UI (vendored)

This is the user interface of Microsoft's [Azure Trusted Research Environment](https://github.com/microsoft/AzureTRE),
copied from upstream commit `7be5806` (2026-10-06) under its MIT licence. Copyright (c) Microsoft
Corporation; see [LICENSE](LICENSE). It is kept as close to upstream as possible so that it looks and
behaves exactly like AzureTRE.

Changes for running against KubeTRE's AzureTRE-compatible API, each marked
"Modified for KubeTRE" in the source:

- `src/config.ts` and `index.html`: configuration is loaded at runtime from `/config.js`
  instead of a `config.json` baked in at build time.
- `src/hooks/useAuthApiCall.ts`: a workspace's roles come from the API (`userRoles`), because
  KubeTRE uses one API audience rather than an Entra app registration per workspace. TRE-wide
  roles still come from the token.
- `Dockerfile`, `nginx.conf`: container image for Kubernetes.
- `src/models/resourceTemplate.validation.test.ts` is removed. It validated AzureTRE's Porter
  bundle schemas from the upstream repository's `templates/` folder, which KubeTRE does not have.

Run upstream's tests with `npm ci && npx vitest run`.
