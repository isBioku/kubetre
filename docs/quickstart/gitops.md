# 6. GitOps

Flux installs everything inside the cluster from Git. Turn it on by pointing Terraform at the
repository, your fork or `https://github.com/isBioku/kubetre`, and applying again:

```hcl
gitops_repository_url = "https://github.com/<you>/kubetre"
```

```sh
terraform -chdir=infra/azure apply -var-file=profiles/small.tfvars
```

Terraform hands Flux this environment's values, such as the registry, hostname and client
IDs, so nothing environment-specific is committed to Git. Flux then applies these stages from
`deploy/azure`, in order:

| Stage | Installs |
|---|---|
| `kubetre-crossplane` | Crossplane |
| `kubetre-crossplane-packages` | the Azure network and compute providers and the composition functions, activating only the eight resource kinds KubeTRE uses |
| `kubetre-edge` | Envoy Gateway |
| `kubetre-platform` | KubeTRE's CRDs, controller, API, Crossplane compositions and templates |
| `kubetre-gateway` | the access gateway, Guacamole, guacd and AzureTRE's UI, and their routes |
| `kubetre-kubevirt` | KubeVirt, CDI and the KubeVirt VM template, only when `kubevirt_enabled` |

Watch them become ready, within about 15 minutes:

```sh
kubectl get kustomizations -n flux-system -w
```

Flux checks the repository every five minutes. To apply a push at once:

```sh
kubectl -n flux-system annotate gitrepository kubetre reconcile.fluxcd.io/requestedAt="$(date +%s)" --overwrite
```

## Check the platform

```sh
kubectl get providers.pkg.crossplane.io,functions     # all HEALTHY=True
kubectl get workspacetemplates,servicetemplates      # 2 workspace and 4 service templates
kubectl -n kubetre-system get pods                   # controller and API running
kubectl -n kubetre-gateway get pods                  # broker, guacamole, guacd and ui running
kubectl -n envoy-gateway-system get svc              # EXTERNAL-IP equals the gateway ILB address
curl -s https://<gateway_hostname>/api/health
```

Use `providers.pkg.crossplane.io` rather than `providers`: Azure Policy's Gatekeeper also
defines a `Provider` type.

Then check the firewall for anything the platform needed but was denied. In the Log Analytics
workspace `log-<env>`, run:

```kusto
AZFWApplicationRule
| where TimeGenerated > ago(1h) and Action == "Deny"
| summarize Count = count() by Fqdn, SourceIp
| order by Count desc
```

Add platform hostnames you trust to `platform_egress_fqdns` and apply again. Denials from
workspace subnets are expected: that is the isolation working.
