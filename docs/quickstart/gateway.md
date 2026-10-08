# 5. Gateway hostname and secrets

Users reach everything through one hostname: the UI at `/`, the API at `/api`, the access
gateway at `/gateway` and Guacamole at `/guacamole`.

## DNS

Create an A record for your hostname pointing at the firewall's public IP:

```sh
terraform -chdir=infra/azure output -raw firewall_public_ip
```

No domain? Set `firewall_dns_label`, for example `kubetredev-gw`, and apply. Azure then names
the firewall's IP `<label>.<region>.cloudapp.azure.com`; use that as `gateway_hostname` and
rerun `hack/entra-setup.sh` with it.

## TLS certificate and sign-in secret

Create the gateway's namespace and two secrets. Flux would create the namespace later, but
creating it now means the secrets are ready when the gateway starts.

```sh
kubectl create namespace kubetre-gateway
kubectl -n kubetre-gateway create secret tls kubetre-gateway-tls \
  --cert=fullchain.pem --key=privkey.pem
kubectl -n kubetre-gateway create secret generic kubetre-gateway-oidc \
  --from-file=client-secret=kubetre-<env>-gateway-client-secret.txt
```

Use a certificate from your organisation's CA, or from Let's Encrypt with a DNS challenge.
Renewal is not automated yet, so note the expiry date. For a short test, a self-signed
certificate works; browsers warn once and sign-in still works:

```sh
H=<gateway_hostname>
openssl req -x509 -newkey rsa:3072 -sha256 -days 30 -nodes -keyout privkey.pem -out fullchain.pem \
  -subj "/CN=$H" -addext "subjectAltName=DNS:$H" -addext "extendedKeyUsage=serverAuth"
```

See [Custom domain and TLS](../admin/custom-domain.md) to change either later.
