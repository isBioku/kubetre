# KubeVirt

KubeVirt lets researchers choose a Linux VM that runs inside the cluster, beside Azure VMs.
It is off by default. [ADR 11](../adr/0011-kubevirt-vms.md) explains the design and the
trade-offs.

## Enable it

1. **Mirror the images.** Copy KubeVirt, CDI and the Ubuntu disk image into ACR, and publish
   the charts:

   ```sh
   make acr-mirror-kubevirt publish-charts ACR_NAME=<acr name>
   ```

2. **Apply.** Set `kubevirt_enabled = true` in `terraform.tfvars` and apply. Terraform adds:
   - a tainted `kubevirt` node pool, Standard_D4s_v5 by default, which supports nested
     virtualization;
   - the `kubetre-kubevirt` Flux stage, which installs KubeVirt, CDI and the template.
3. **Check.**

   ```sh
   kubectl -n kubevirt get kubevirt kubevirt -o jsonpath='{.status.phase}'   # Deployed
   kubectl get cdi cdi -o jsonpath='{.status.phase}'                           # Deployed
   kubectl get servicetemplate linux-vm-kubevirt
   ```

**Linux VM on KubeVirt** then appears under Virtual Desktops in every workspace.

## How it is set up

- **Control components.** KubeVirt's operator, API and controller run on the AKS system
  pool. Upstream KubeVirt expects control-plane nodes, which AKS does not expose.
- **VMs.** KubeVirt's node agent and the VMs run only on the `kubevirt` pool.
- **Pod Security.** VM pods get the RuntimeDefault seccomp profile, so they pass the
  workspace's `restricted` Pod Security level.
- **Disks.** CDI copies the Ubuntu image into a filesystem volume with the node's own registry
  credentials. Its importer pods run on the `kubevirt` pool too, so each disk is created in
  the same zone as its VM.
- **Access.** The workspace's `kubetre-deployer` account may create VirtualMachines and
  DataVolumes in its own namespace only.

## Size the pool

| VM size | vCPU | Memory |
|---|---|---|
| small | 1 | 4 GiB |
| medium | 2 | 8 GiB |
| large | 3 | 12 GiB |

A Standard_D4s_v5 node fits one medium VM with room for KubeVirt's overhead. Raise
`kubevirt_node_count` for more.

## Windows on KubeVirt

Not offered. A Windows guest nested in a Linux node is not covered by Azure's included
licence, so it needs separately licensed Windows Server and a maintained image. Windows VMs
should be Azure VMs.

## Disable it

Delete every KubeVirt VM first. Then set `kubevirt_enabled = false` and apply. Terraform
removes the node pool and the Flux stage. Flux does not delete what the stage installed, so
remove it yourself:

```sh
kubectl delete servicetemplate linux-vm-kubevirt
kubectl -n kubevirt delete kubevirt kubevirt --wait
kubectl delete cdi cdi --wait
kubectl delete namespace kubevirt cdi
```
