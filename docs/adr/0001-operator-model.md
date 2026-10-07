# 1. Kubernetes is the control plane

Status: Accepted, 2026-10-07

## Context
AzureTRE stores desired state in Cosmos DB, sends work over Service Bus to a resource
processor on a VM, which runs Porter bundles and reports back over a second queue. Most of
the API's complexity is keeping these in sync: sessions for ordering, status polling,
retries, compensation when a send fails.

## Decision
Workspaces and templates are Kubernetes custom resources. The API validates intent and writes
a custom resource. Controllers reconcile it and report through `status.conditions`.
There is no queue between the API and the controllers.

## Consequences
- Ordering, retries, optimistic concurrency and watch-based progress come from the API server.
- `etcd` is not a reporting database. History, audit, airlock requests and cost snapshots will
  live in PostgreSQL when those features arrive.
- Templates become controllers or rendered packages rather than opaque bundles. See ADR 6.
