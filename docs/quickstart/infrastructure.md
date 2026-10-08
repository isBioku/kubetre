# 3. Infrastructure

Terraform in `infra/azure` creates the cluster and everything around it. See
[Azure resources](../overview/azure-resources.md) for the full list.

## State store

Terraform keeps its state in Azure Storage with shared-key access disabled:

```sh
az group create -n rg-kubetre-tfstate -l uksouth
az storage account create -n <stateaccount> -g rg-kubetre-tfstate --sku Standard_ZRS \
  --min-tls-version TLS1_2 --allow-shared-key-access false --allow-blob-public-access false
az storage container create -n tfstate --account-name <stateaccount> --auth-mode login
az role assignment create --role "Storage Blob Data Contributor" \
  --assignee "$(az ad signed-in-user show --query id -o tsv)" \
  --scope "$(az storage account show -n <stateaccount> --query id -o tsv)"
```

## Configuration

In `infra/azure`, create `backend.hcl`:

```hcl
resource_group_name  = "rg-kubetre-tfstate"
storage_account_name = "<stateaccount>"
container_name       = "tfstate"
key                  = "kubetredev.tfstate"
use_azuread_auth     = true
```

Copy `terraform.tfvars.example` to `terraform.tfvars`:

- **Fill in:** `name`, `location`, `zones` and `operator_ip_ranges`.
- **Paste:** the lines printed by `hack/entra-setup.sh`.
- **Leave** `gitops_repository_url` empty for now. Flux is turned on in [step 6](gitops.md),
  after the images exist.

Both files are ignored by Git. [Configuration](../admin/configuration.md) lists every
variable.

## Apply

Pin the subscription, then apply. Add `-var-file=profiles/small.tfvars` for a short,
low-cost test: one node per pool and smaller VM sizes.

```sh
cd infra/azure
export ARM_SUBSCRIPTION_ID=<subscription-id> ARM_TENANT_ID=<tenant-id>
terraform init -backend-config=backend.hcl
terraform apply -var-file=profiles/small.tfvars
```

This takes 20 to 40 minutes. Then connect to the cluster:

```sh
$(terraform output -raw get_credentials)
kubelogin convert-kubeconfig -l azurecli
kubectl get nodes
```

The API server accepts only `operator_ip_ranges`. If `kubectl` times out, check your public IP
has not changed, for example because of a VPN.
