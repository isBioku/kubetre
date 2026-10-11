# Azure OpenAI

**Template name:** `azure-openai` · **Kind:** workspace service · **Composition:** `crossplane/paas/azure/composition-openaiservice.yaml`

An Azure OpenAI account with one model deployment for one workspace, the counterpart of
AzureTRE's Azure OpenAI workspace service.

| Property | Updateable | Notes |
|---|---|---|
| Model, Model version | no | for example `gpt-4o-mini`, `2024-07-18` |
| Deployment type | no | `Standard` keeps processing in the region, `DataZoneStandard` in the geography, `GlobalStandard` anywhere |
| Capacity | yes | thousands of tokens per minute |
| Region | no | empty for the TRE's region; set it if the model is not offered there |

## Security

- No public network access; a private endpoint in the workspace's subnet, in the TRE's
  region even when the account is elsewhere.
- **No API keys.** Local authentication is off. Members get **Cognitive Services OpenAI
  User** and use Entra ID tokens.
- Choose the deployment type with your data protection officer: only `Standard` keeps
  prompts in one region.

## Use it

From a workspace machine, with the Azure CLI signed in (`az login --use-device-code --allow-no-subscriptions`):

```python
from azure.identity import DefaultAzureCredential, get_bearer_token_provider
from openai import AzureOpenAI

token = get_bearer_token_provider(DefaultAzureCredential(), "https://cognitiveservices.azure.com/.default")
client = AzureOpenAI(azure_endpoint="https://oai-1a2b3c4d5e.openai.azure.com/",
                     azure_ad_token_provider=token, api_version="2024-10-21")
```
