# Connecting

How a connection works: **Connect** opens `/gateway/connect/<workspace>/<resource>`. The
broker signs the user in, checks ownership and the target, and serves a short page. That page
ends any old Guacamole session and opens Guacamole with a single-use payload. Guacamole asks
guacd to connect.

Each session the broker opens is logged:

```sh
kubectl -n kubetre-gateway logs deploy/broker | grep "session opened"
```

## Forbidden

The broker refuses connect links that did not come from this site, using the browser's
`Sec-Fetch-Site` header. Open the machine from the UI's **Connect** button, or type the
address. A link from another website, an email or a chat is refused by design.

## Connect redirects to sign-in with an error

The gateway app's redirect URI must be exactly `https://<host>/gateway/callback`. Rerun
`hack/entra-setup.sh <env> <host>`.

## Not ready

| Message | Meaning |
|---|---|
| "waiting for the virtual machine's address" | Azure is still creating the VM |
| "virtual machine address is outside the workspace subnet" | refused for safety; check the VM |
| "hostname must be a Service in the workspace namespace" | the chart's connection Secret points elsewhere |
| "connection not found" | not your machine, or it was deleted |

## Guacamole shows "An error has occurred"

The sign-in reached one Guacamole replica, and the next request went to another. Guacamole
keeps sessions in memory, so the gateway's route must be sticky. Check that the policy is
accepted:

```sh
kubectl -n kubetre-gateway get backendtrafficpolicy guacamole-affinity -o jsonpath='{.status.ancestors[*].conditions}'
```

## Guacamole shows an empty list, or "not logged in"

- **Empty list.** Guacamole reused an older session in the browser. The broker's hand-off page
  ends it before each connection; select **Connect** again from the UI.
- **"Not logged in".** The affinity cookie must have `Path=/guacamole/`. Check
  `curl -sk -D - -o /dev/null https://<host>/guacamole/ | grep -i set-cookie`.

## The session opens but the machine refuses it

```sh
kubectl -n kubetre-gateway logs deploy/guacd
```

- **Azure VMs.** The VM's security group must admit the gateway pod subnet on 3389 or 22.
  Only guacd may run there; check `kubectl -n kubetre-gateway get pods -o wide`.
- **Desktops and KubeVirt VMs.** The workspace's network policy admits only guacd, and the
  gateway's policy lets guacd reach workspace pods on 5901 and 22.
- **Linux machines.** The account and password come from the machine's credentials Secret in
  the workspace namespace. A KubeVirt VM's cloud-init sets them on first boot; recreate the
  VM if that boot failed.
