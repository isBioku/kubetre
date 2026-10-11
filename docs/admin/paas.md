# Azure PaaS services

Azure SQL, MySQL, Azure OpenAI, Health Data Services, Azure Machine Learning and Azure
Databricks are workspace services that Crossplane creates in Azure, one set per workspace
service. They are on by default; set `paas_services_enabled = false` to leave them out.

## What the setting installs

| Where | What |
|---|---|
| Terraform (`infra/azure/paas.tf`) | 11 private DNS zones linked to the VNet; federated credentials for 10 more Crossplane providers; Crossplane's permissions in the workspaces resource group |
| Flux stage `paas` (`deploy/azure/paas`) | the providers, their activation policy, one XRD and composition per service (`crossplane/paas`), and the templates (`config/paas`) |
| ACR | the `azure-paas` chart, pushed by `make publish-charts` |

The providers run in the cluster, about 10 more pods in `crossplane-system`.

## How every service is built

- **No public network access.** Each service has private endpoints in the workspace's VM
  subnet, registered in the private DNS zones. The workspace's machines reach them; nothing
  else does. The template needs a workspace with virtual machines.
- **Entra ID instead of keys.** Members' Entra object IDs are recorded by the API the first
  time they use the TRE. Compositions grant those IDs data roles on the service, so members
  sign in with their own accounts. Members who have never opened the TRE get access once they
  do. Removing someone from the workspace removes their roles.
- **Sign-in from the workspace.** Templates add the Entra sign-in hosts
  (`login.microsoftonline.com`, `aadcdn.msftauth.net`, `aadcdn.msauth.net`) to the workspace's
  allowed domains while the service exists.
- **Connection details.** Each composition writes an `ExternalName` Service in the workspace
  namespace. The API reads it and shows the address on the service's card as internal.

## Crossplane's permissions

With the setting on, Crossplane's identity also holds, in the workspaces resource group only:

| Role | Why |
|---|---|
| Contributor | create the services |
| Role Based Access Control Administrator, with a condition | grant members data roles; the condition allows only the roles below |
| Private DNS Zone Contributor, on each zone | register private endpoints |

The condition lists Cognitive Services OpenAI User, AzureML Data Scientist, FHIR Data
Contributor, DICOM Data Owner, Storage Blob Data Contributor, Storage File Data Privileged
Contributor, Key Vault Administrator and Contributor. Crossplane can never grant Owner or
User Access Administrator.

## Status

The compositions are validated against the providers' schemas in `test/crossplane`, but
they have not yet been deployed to Azure. Expect to adjust them on first use, in particular
the egress Azure ML studio and Databricks need.
