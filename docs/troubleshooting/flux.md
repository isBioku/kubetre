# Deployment and Flux

## A stage is not ready

```sh
kubectl -n flux-system get kustomizations
kubectl -n flux-system describe kustomization kubetre-<stage>
```

- **Dependency not ready.** Stages wait for the one before. Fix the first stage that is not
  ready.
- **Waiting on CRDs.** `kubetre-platform` can fail once while Crossplane's CRDs install; it
  retries every minute.
- **Empty substitutions.** A new setting was pushed before Terraform supplied its value.
  Apply Terraform, then reconcile.
- **Health checks timing out.** Find the failing workload in the stage's namespace.

Reconcile at once instead of waiting for the five-minute sync:

```sh
kubectl -n flux-system annotate gitrepository kubetre reconcile.fluxcd.io/requestedAt="$(date +%s)" --overwrite
```

## A Crossplane provider is not healthy

```sh
kubectl get providers.pkg.crossplane.io
kubectl describe providers.pkg.crossplane.io provider-azure-network
```

Usually the firewall blocked a package download. Run the firewall query in [Logs](logs.md)
and add the exact host to `platform_egress_fqdns`. Upbound serves package layers from a
CloudFront host; allow that host only, never `*.cloudfront.net`.

## The API server is unavailable

`kubectl` returns `ServiceUnavailable`, and Flux, Crossplane and the controller restart with
"leader election lost" or "failed to get server groups".

- **Too many CRDs.** Crossplane's providers can register hundreds of CRDs. KubeTRE activates
  only the eight it uses (`crossplane/install/activation.yaml`). On a cluster first installed
  with Crossplane's catch-all policy, delete it:

  ```sh
  kubectl delete managedresourceactivationpolicy default
  ```

  CRDs already created stay until the providers are reinstalled.
- **Free tier.** A cluster on the Free tier has a small control plane with no SLA. Use
  `aks_sku_tier = "Standard"`, which every profile now uses. The change takes about ten
  minutes, with no downtime.

## Pods cannot start

| Status | Check |
|---|---|
| `ImagePullBackOff` | `kubectl describe pod`; build or mirror the image into ACR, or fix the tag |
| `Pending` | `kubectl describe pod`: insufficient resources, untolerated taints, or no matching node |
| `CrashLoopBackOff` | the pod's logs, with `--previous` |
| Rejected by admission | `kubectl get events -n <namespace>`; Pod Security or quota |
