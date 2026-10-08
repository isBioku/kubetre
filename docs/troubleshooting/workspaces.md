# Workspaces and services

## A workspace stays deploying

```sh
kubectl get workspace <id> -o jsonpath='{.status.conditions}'
kubectl -n ws-<id> get workspacenetwork vm-network
kubectl -n ws-<id> describe workspacenetwork vm-network
kubectl get managed -l crossplane.io/composite=vm-network
```

- **`VMNetworkProvisioning`.** Crossplane is still creating the subnet, security group or
  firewall rules. The managed resources' events show the Azure error, commonly a missing
  permission for Crossplane's identity or an address pool overlap.
- **Subnet not found.** The subnet must be created in the VNet's resource group; check the
  `kubetre-azure` EnvironmentConfig.

## A service or machine stays deploying, or fails

```sh
kubectl -n ws-<id> get workspaceservice <name> -o yaml
kubectl -n ws-<id> get helmrelease,ocirepository <name>
kubectl -n ws-<id> describe helmrelease <name>
kubectl -n ws-<id> get events --sort-by=.lastTimestamp
```

| Reason | Meaning |
|---|---|
| `PodSecurityTooStrict` | the template needs a weaker Pod Security level than the workspace enforces |
| `VirtualMachinesNotEnabled` | an Azure VM template in a workspace without a VM network |
| `TemplateNotFound` | the template was removed or renamed |
| Helm install or upgrade failed | the chart's resources did not become ready; see the events |

For Azure VMs:

```sh
kubectl -n ws-<id> get researchvm <name>
kubectl -n ws-<id> describe researchvm <name>
```

For KubeVirt VMs:

```sh
kubectl -n ws-<id> get vm,vmi,datavolume,pvc
kubectl -n ws-<id> describe vmi <name>
```

A KubeVirt VM pod larger than the workspace's limits, or a disk import that keeps failing,
shows in the namespace's events.

## Changing a resource by hand

AzureTRE sometimes needs Cosmos DB documents edited by hand. In KubeTRE, edit the resource:

```sh
kubectl edit workspace <id>
kubectl -n ws-<id> edit workspaceservice <name>
```

The API and UI read the same resources, so changes show at once. Prefer the UI: it validates
against the template.
