# Custom domain and TLS

KubeTRE serves everything on one hostname, `gateway_hostname`: the UI, the API, the access
gateway and Guacamole. Unlike AzureTRE's custom domain, it covers remote desktop sessions too.

## Change the hostname

1. **Point DNS.** Create an A record for the new name pointing at the firewall's public IP
   (`terraform output firewall_public_ip`).
2. **Update Entra ID.** Rerun `hack/entra-setup.sh <env> <new host>`. It updates the UI's and
   the gateway's redirect URIs.
3. **Apply.** Set `gateway_hostname` in `terraform.tfvars` and apply.
4. **Replace the certificate.** Install a certificate for the new name, as below.

Without a domain, set `firewall_dns_label` to get `<label>.<region>.cloudapp.azure.com`.

## Install or renew the certificate

The certificate is the `kubetre-gateway-tls` secret in the `kubetre-gateway` namespace. Envoy
Gateway picks up a replaced secret without a restart.

```sh
kubectl -n kubetre-gateway create secret tls kubetre-gateway-tls \
  --cert=fullchain.pem --key=privkey.pem --dry-run=client -o yaml | kubectl apply -f -
```

Use your organisation's CA, or Let's Encrypt with a DNS challenge. Renewal is not automated
yet; there is no equivalent of AzureTRE's `make letsencrypt`.
