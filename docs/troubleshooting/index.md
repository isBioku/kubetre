# Troubleshooting

AzureTRE is debugged through Application Insights, Service Bus, the resource processor and
Cosmos DB. KubeTRE is debugged with `kubectl`: every request becomes a Kubernetes resource
whose status says what is happening. Work down this list:

1. **The resource's status:** `kubectl get workspace <id>` or
   `kubectl -n ws-<id> get workspaceservice <name> -o yaml`. Its `Ready` condition gives the
   reason.
2. **The controller's logs.**
3. **Flux:** the stages in `flux-system`, and each service's `HelmRelease` in its workspace
   namespace.
4. **Crossplane:** the composite and managed resources for networks and VMs.
5. **The access gateway's logs,** for connection problems.
6. **The firewall's deny logs,** for anything that cannot reach the internet.

| Page | For |
|---|---|
| [Logs](logs.md) | where each component logs, and how to see more |
| [Deployment and Flux](flux.md) | stages not ready, image pulls, the API server |
| [Workspaces and services](workspaces.md) | stuck deploying or failing |
| [Connecting](connecting.md) | Connect errors, sessions |
| [Sign-in and the API](sign-in.md) | access denied, 401, "TRE API unavailable" |

## Common problems

| Symptom | Likely cause and fix |
|---|---|
| A Flux stage stays `Ready=False` | See [Deployment and Flux](flux.md#a-stage-is-not-ready). |
| Pods show `ImagePullBackOff` | The image is not in ACR: run `make acr-build`, `make acr-mirror` or `make acr-mirror-kubevirt`, or `kubetre_version` does not match the tag you built. |
| `kubectl` returns `ServiceUnavailable` | The API server is overloaded; see [Deployment and Flux](flux.md#the-api-server-is-unavailable). |
| A workspace stays *deploying* | See [Workspaces and services](workspaces.md#a-workspace-stays-deploying). |
| The UI says you have no access | Assign `TREUser` or `TREAdmin` on the API app, then sign out and in. |
| "The TRE API is currently unavailable" | The API returned an error; see [Sign-in and the API](sign-in.md#the-tre-api-is-currently-unavailable). |
| Connect shows "forbidden" | See [Connecting](connecting.md#forbidden). |
| Guacamole shows an error or an empty list | See [Connecting](connecting.md). |
| A resource cannot be deleted | Disable it first. A workspace service with user resources must have them deleted first. |
| A researcher cannot see a workspace | Their email in the workspace must match the email in their token. |
