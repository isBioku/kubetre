# Azure Machine Learning

**Template name:** `azureml` · **Kind:** workspace service · **Composition:** `crossplane/paas/azure/composition-mlworkspace.yaml`

An Azure Machine Learning workspace for one workspace, with its own storage account, key
vault and Application Insights, the counterpart of AzureTRE's Azure ML workspace service.
Members add their own [compute instances](../user-resources/azureml-compute-instance.md)
under it.

## Security

- None of the Azure ML workspace, storage account or key vault has public network access.
  Private endpoints in the workspace's subnet serve the API, notebooks, blob and file
  storage, and the vault.
- Application Insights sends to the TRE's Log Analytics workspace.
- Members get **AzureML Data Scientist** on the workspace, and **Storage Blob Data
  Contributor** and **Storage File Data Privileged Contributor** on its storage.
- The template adds `ml.azure.com` and `management.azure.com` to the workspace's allowed
  domains, for the studio.

## Use it

Open the address on the card, `https://ml.azure.com/?wsid=...`, in a browser on a workspace
machine, and sign in.

AzureTRE also allows the studio's content delivery and container registry domains; add any
the studio needs to the workspace's allowed domains.
