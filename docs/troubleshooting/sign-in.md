# Sign-in and the API

## The UI says you have no access

Assign the user `TREUser` or `TREAdmin` on the `kubetre-<env>-api` enterprise application.
TRE roles come from the sign-in token, so the user must sign out and in again.

## Sign-in fails with a redirect URI error

The UI app's single-page redirect URIs must include `https://<host>`. Rerun
`hack/entra-setup.sh <env> <host>` with the hostname users actually use.

## A researcher cannot see their workspace

Workspace membership is matched on the email in the token. Check what the API sees:

```sh
curl -s https://<host>/api/v1/me -H "Authorization: Bearer $TOKEN"
```

The email must be in the workspace's owners or researchers. Accounts without an email in
Entra ID cannot be matched.

## The API returns 401

The token's audience must be the API app ID and its issuer must end in `/v2.0`. Get a token
with:

```sh
az account get-access-token --scope api://<api app id>/user_impersonation --query accessToken -o tsv
```

## The TRE API is currently unavailable

The UI shows this for any server error. The API's logs give the cause:

```sh
kubectl -n kubetre-system logs deploy/kubetre-api | grep '"level":"ERROR"'
```

- **"forbidden" from Kubernetes:** the API's service account lacks a permission. Check
  `config/rbac/api_role.yaml` matches the release; the end-to-end RBAC test covers every UI
  action.
- **Timeouts:** see [The API server is unavailable](flux.md#the-api-server-is-unavailable).

## isAdmin is false

Assign `TREAdmin`, then get a new token: roles are read from the token.
