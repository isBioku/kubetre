# Health Data Services

**Template name:** `health-data-services` · **Kind:** workspace service · **Composition:** `crossplane/paas/azure/composition-healthdataservice.yaml`

An Azure Health Data Services workspace with a FHIR R4 service and, optionally, a DICOM
service, the counterpart of AzureTRE's Health Services workspace service.

| Property | Updateable | Notes |
|---|---|---|
| Add a DICOM service | no | default off |

## Security

- A private endpoint in the workspace's subnet for the FHIR and DICOM services. The DICOM
  service has public network access turned off. The provider cannot yet turn it off for the
  Health Data Services workspace, so the FHIR service still accepts requests with a valid
  token from elsewhere; only members hold roles on it.
- Members get **FHIR Data Contributor**, and **DICOM Data Owner** if DICOM is added. They
  use Entra ID tokens.

## Use it

From a workspace machine:

```bash
az login --use-device-code --allow-no-subscriptions
TOKEN=$(az account get-access-token --resource https://hds1a2b3c4d5e-fhir.fhir.azurehealthcareapis.com --query accessToken -o tsv)
curl -H "Authorization: Bearer $TOKEN" https://hds1a2b3c4d5e-fhir.fhir.azurehealthcareapis.com/metadata
```
