# 2. Identity

KubeTRE signs users in with Microsoft Entra ID through OpenID Connect. It needs three app
registrations and one group, far fewer than AzureTRE, because it creates no app registration
per workspace and needs no Microsoft Graph permissions.

| Object | Name | Purpose |
|---|---|---|
| Group | `kubetre-<env>-admins` | AKS cluster administrators |
| App registration | `kubetre-<env>-api` | the API: app roles `TREAdmin` and `TREUser`, scope `user_impersonation`, v2 tokens with the `email` claim |
| App registration | `kubetre-<env>-ui` | AzureTRE's UI, a single-page app at `https://<host>/`, pre-authorised for the API's scope |
| App registration | `kubetre-<env>-gateway` | the access gateway's sign-in, redirect `https://<host>/gateway/callback`, with a client secret |

## Which tenant

Use the tenant your users already sign in with. For a development environment where you
cannot create app registrations, a separate Entra tenant works too: set `oidc_issuer` to that
tenant. Any OpenID Connect provider can be used instead of Entra ID if it issues the roles in
a claim (`roles_claim`).

## Create them

Run the setup script with the environment name and the hostname you chose. It is idempotent:
run it again to repair or update the registrations, for example after changing the hostname.

```sh
hack/entra-setup.sh kubetredev tre.example.org
```

It:

- **Groups and roles:** creates the group and adds you to it, and assigns you `TREAdmin`.
- **Applications:** creates or updates the three app registrations, their service principals,
  redirect URIs, token version and claims.
- **Gateway secret:** writes the gateway's client secret, valid for one year, to
  `kubetre-<env>-gateway-client-secret.txt`. Git ignores that file; keep it safe.
- **Output:** prints the Terraform variables for [step 3](infrastructure.md):

```hcl
admin_group_object_ids = ["<group id>"]
oidc_issuer            = "https://login.microsoftonline.com/<tenant id>/v2.0"
oidc_audience          = "<api app id>"
gateway_hostname       = "tre.example.org"
gateway_oidc_client_id = "<gateway app id>"
ui_client_id           = "<ui app id>"
```

## Give people access

Everyone who uses the UI needs a TRE role on the `kubetre-<env>-api` enterprise application.
In the Entra admin centre, open **Enterprise applications**, select the app, then **Users and
groups**:

- **TRE administrators:** `TREAdmin`.
- **Workspace owners and researchers:** `TREUser`. Their workspace roles are set later, by
  email address, in the workspace's form.

Workspace membership is matched on the email in the user's token, so each user needs an email
address in Entra ID.

See [Identities](../admin/identities.md) for every setting.
