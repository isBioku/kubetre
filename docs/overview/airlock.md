# Airlock

!!! warning "Not available yet"
    KubeTRE has no airlock. Workspaces report `enable_airlock: false`, so the UI hides the
    airlock pages. There is currently no sanctioned way to move files into or out of a
    workspace.

In AzureTRE, the airlock is the only approved route for data to enter or leave a workspace.
A researcher creates a request, uploads a file, and submits it. Imports can be scanned for
malware. An airlock manager reviews the request, if needed in a review VM, and approves or
rejects it. Every step is audited and the requester is notified.

## Where KubeTRE stands

- The `AirlockManager` role exists: a workspace's airlock managers are listed on the
  `Workspace` and returned to the UI, but the role grants nothing yet.
- The airlock API endpoints return empty lists.

## Design notes for a future airlock

AzureTRE moved from storage accounts per stage, ten or more per TRE plus five per workspace,
to two shared storage accounts. In the new design a request's stage lives in container
metadata, and access is limited with attribute-based conditions and private endpoints per
workspace. AzureTRE dropped the older design because it was slower, more expensive and
multiplied resources. A KubeTRE airlock should start from the newer design:

- **Requests as resources.** An `AirlockRequest` resource that the controller reconciles,
  instead of Service Bus, Event Grid and Functions.
- **Stage in metadata.** The request's stage kept as metadata, with data copied only when it is
  sealed on submission and when it is approved.
- **Narrow upload links.** Short-lived upload and download links, scoped to one request.
- **Workspace-only access.** Access only from inside the workspace, through a private endpoint
  in the workspace subnet or an in-cluster proxy bounded by network policy.
- **Request history.** Kept in a database rather than in Kubernetes, as
  [ADR 1](../adr/0001-operator-model.md) plans.

AzureTRE's newer airlock signs upload links with an Entra application per workspace. KubeTRE
has no per-workspace applications ([ADR 3](../adr/0003-identity.md)), so it would need
another signer.
