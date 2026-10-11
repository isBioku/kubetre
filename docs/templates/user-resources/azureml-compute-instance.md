# Azure ML compute instance

**Template name:** `azureml-compute-instance` · **Kind:** user resource of [Azure Machine Learning](../workspace-services/azureml.md) · **Composition:** `crossplane/paas/azure/composition-mlcomputeinstance.yaml`

A personal Azure ML compute instance, the counterpart of AzureTRE's Azure ML compute
instance user resource.

| Property | Updateable | Notes |
|---|---|---|
| Size | no | `Standard_DS3_v2`, `Standard_D4s_v5`, `Standard_E4s_v5`, or `Standard_NC4as_T4_v3` with a GPU |

It runs in the workspace's VM subnet with no public IP and no local authentication, and is
assigned to the person who creates it. They open it from Azure ML studio on a workspace
machine.
