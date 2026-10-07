Upstream CRDs used only by tests to validate KubeTRE's Crossplane manifests and rendered
Azure resources against the real schemas. Refresh with `hack/fetch-crossplane-crds.sh`
when bumping versions.

| Source | Version |
|---|---|
| crossplane/crossplane | v2.4.2 |
| crossplane-contrib/provider-upjet-azure | v2.7.0 |
| crossplane-contrib/function-go-templating | v0.13.0 |
| crossplane-contrib/function-environment-configs | v0.9.0 |
| fluxcd/flux2 | v2.9.6 |
| envoyproxy/gateway (chart CRDs, incl. Gateway API) | v1.9.2 |
| cilium/cilium | v1.18.0 |
