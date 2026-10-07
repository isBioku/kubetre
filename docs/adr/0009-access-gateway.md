# 9. Access gateway: an OIDC broker in front of stock Guacamole

Status: Accepted, 2026-10-07. Not yet deployed to Azure.

## Decision

Researchers reach desktops and VMs only through the access gateway in namespace
`kubetre-gateway`. It has three parts:

- **Broker** (`cmd/gateway`, `internal/gateway`): signs users in with OIDC (authorization code
  with PKCE, state and nonce), lists the sessions they may open, and on each connect hands
  Guacamole a payload for exactly one session. The payload uses Guacamole's own
  `guacamole-auth-json` format: HMAC-SHA256 signed, AES-128 encrypted, single-use, valid for
  60 seconds, and passed in the URL fragment so it never reaches a server log.
- **Guacamole** (stock `guacamole/guacamole:1.6.0`): only the encrypted-JSON extension is
  enabled, so it has no login of its own and trusts nothing the broker did not sign.
- **guacd** (stock `guacamole/guacd:1.6.0`): opens the RDP, SSH and VNC connections. It runs
  only on the gateway node pool, whose pod subnet is the one source workspace VM NSGs accept.

AzureTRE needed a custom Java Guacamole extension that called its API. Here authorization lives
in Go code we test, and Guacamole is unmodified.

## Authorization rules

| Rule | Enforced by |
|---|---|
| A user sees only personal services they own, in workspaces they belong to | `WorkspaceService.spec.owner`, set only by the API, and workspace membership |
| Workspace owners do not get researchers' machines | ownership, not role, decides |
| A chart cannot publish a session for someone else's service | the owning service comes from Helm's `meta.helm.sh/release-name`, which charts cannot set |
| A session cannot point outside its workspace | VM addresses must be inside the workspace's subnet; desktop hosts must be Services in the workspace namespace |
| Authorization is current | re-evaluated on every connect, not cached in the session |
| No data channel through the session | clipboard both ways, drive mapping, file transfer, SFTP, printing and audio input are off; the desktop image also disables VNC clipboard |

## Network path

Internet, then Azure Firewall DNAT (TCP 443 only), then Envoy Gateway on an internal load
balancer, then broker or Guacamole, then guacd, then the VM or desktop. Network policies
default-deny the gateway namespace; guacd may reach only the VM address range on 22 and 3389
and workspace pods on 5901; the broker may reach only the Kubernetes API and the sign-in
provider. Workspace namespaces accept desktop traffic only from guacd pods.

## Consequences and open points

- The broker can list Secrets cluster-wide, because RBAC cannot restrict a list by label.
  It runs on the isolated gateway pool and writes only its own key Secret.
- TLS for the public hostname comes from the Secret `kubetre-gateway-tls`, created by the
  operator. Automating it with cert-manager is future work.
- Session recording, idle timeouts and per-session audit in Log Analytics beyond the broker's
  "session opened" log line are future work.
- Guacamole's container keeps a writable root filesystem because Tomcat writes to it.
