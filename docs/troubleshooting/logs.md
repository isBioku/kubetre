# Logs

| Component | Command |
|---|---|
| KubeTRE controller | `kubectl -n kubetre-system logs deploy/kubetre-controller` |
| KubeTRE API | `kubectl -n kubetre-system logs deploy/kubetre-api` |
| Access gateway broker | `kubectl -n kubetre-gateway logs deploy/broker` |
| Guacamole | `kubectl -n kubetre-gateway logs deploy/guacamole` |
| guacd | `kubectl -n kubetre-gateway logs deploy/guacd` |
| Flux | `kubectl -n flux-system logs deploy/kustomize-controller` and `deploy/helm-controller` |
| Crossplane | `kubectl -n crossplane-system logs deploy/crossplane` and the provider pods |
| Envoy Gateway access logs | `kubectl -n envoy-gateway-system logs -l gateway.envoyproxy.io/owning-gateway-name=kubetre -c envoy` |

Each deployment has several replicas; add `--all-containers --prefix` with `-l` selectors to
read them all.

## More detail

The controller accepts controller-runtime's logging flags. Add `--zap-log-level=debug` to its
arguments in `config/manager/controller.yaml` and push. The API and the broker log JSON at
info level, including every session the broker opens and every request that fails.

## Azure logs

AKS control-plane logs and Azure Firewall logs go to the Log Analytics workspace `log-<env>`.

Firewall denials:

```kusto
AZFWApplicationRule
| where TimeGenerated > ago(1h) and Action == "Deny"
| summarize Count = count() by Fqdn, SourceIp
| order by Count desc
```

Kubernetes API audit, for example who deleted a workspace:

```kusto
AKSAuditAdmin
| where TimeGenerated > ago(1d) and ObjectRef has "workspaces"
| project TimeGenerated, User, Verb, ObjectRef
```
