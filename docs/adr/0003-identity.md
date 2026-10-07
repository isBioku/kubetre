# 3. Plain OIDC with a single API audience

Status: Accepted, 2026-10-07

## Context
AzureTRE creates an Entra ID app registration per workspace and validates a different token
audience per workspace. That ties the product to Microsoft Graph and requires directory admin
rights at runtime.

## Decision
- The API accepts JWTs from any OIDC issuer for one audience. The roles claim path is
  configurable (`roles` for Entra, `realm_access.roles` for Keycloak).
- Platform roles keep AzureTRE's names: `TREAdmin`, `TREUser`.
- Workspace roles (`WorkspaceOwner`, `WorkspaceResearcher`, `AirlockManager`) come from
  membership lists on the Workspace, matched by subject or email. Non-members get 404.
- Creating a workspace creates nothing in the identity provider.

## Consequences
- Entra ID, Keycloak, Dex and others all work unchanged.
- Group-based membership (OIDC `groups` claim) is a planned extension.
