# Airlock

Terraform creates the airlock's two storage accounts, its private endpoint and its identity in
every environment, and every new workspace has the airlock turned on. See
[Airlock](../overview/airlock.md) for how it works.

## Settings

| Setting | Where |
|---|---|
| Malware scanning | Terraform: `airlock_malware_scanning_enabled`, `airlock_malware_scanning_cap_gb` |
| Users who can upload imports | Terraform: `gateway_source_ranges` also limits the external account |
| Airlock on or off for a workspace | the workspace's **Enable airlock** property |
| Airlock managers | the workspace's **Airlock managers** property |
| Review VMs | the workspace's **Airlock review VMs** property |

## Review VMs

For **exports**, add a Virtual Desktops service to the workspace and set **Export review** to
its ID and a VM template, such as `linux-vm`. Airlock managers need no other role in the
workspace.

For **imports**, create one [Airlock Import Review workspace](../templates/workspaces/airlock-import-review.md)
for the TRE and point each workspace's **Import review** at it. Airlock managers must be
members of that workspace.

## Email notifications

The controller sends email when it finds an SMTP secret. With Azure Communication Services,
create an Email resource, connect a domain and an SMTP username, then:

```bash
kubectl -n kubetre-system create secret generic kubetre-airlock-smtp \
  --from-literal=host=smtp.azurecomm.net --from-literal=port=587 \
  --from-literal=username='<smtp username>' --from-literal=password='<entra app secret>' \
  --from-literal=from='DoNotReply@<your domain>'
kubectl -n kubetre-system rollout restart deployment/kubetre-controller
```

Any SMTP server that supports STARTTLS works. Recipients are the email addresses on the
workspace.

## Identity

The API and controller use the managed identity `id-<name>-airlock` through workload identity.
It has **Storage Blob Data Contributor** on both accounts and nothing else. It creates
containers, signs user delegation links and copies files. Shared keys are turned off on both
accounts, so no key exists to leak.

## Checking a request

```bash
kubectl get airlock -A
kubectl -n ws-<workspace> describe airlock <id>
```

The status shows each step, reviews, the scan result and the request's history. The
controller logs every action, with the request ID.
