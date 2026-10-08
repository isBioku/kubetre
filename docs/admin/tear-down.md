# Tear-down

Delete in this order, or Azure refuses to delete the network while VMs still use it.

1. **Delete every workspace.** In the UI, disable and then delete each one. The controller
   removes its namespace, and Crossplane removes its VMs, subnet, security group and firewall
   rules. Check that `rg-<env>-workspaces` is empty:

   ```sh
   az resource list -g rg-<env>-workspaces -o table
   ```

2. **Stop Flux.** Set `gitops_repository_url = ""` and apply.
3. **Destroy:**

   ```sh
   terraform -chdir=infra/azure destroy -var-file=profiles/small.tfvars
   ```

4. **Optionally,** delete the Entra ID app registrations (`kubetre-<env>-api`, `-ui`,
   `-gateway`) and the `kubetre-<env>-admins` group, and the Terraform state storage account.

```sh
for n in api ui gateway; do az ad app delete --id "$(az ad app list --display-name kubetre-<env>-$n --query '[0].appId' -o tsv)"; done
```
