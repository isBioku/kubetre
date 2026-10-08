# Egress control

All traffic out of a KubeTRE environment goes through its own Azure Firewall. Workspaces get
two layers of control. See [Networking](../overview/networking.md) for the rules.

## Allow a domain for a workspace

Edit the workspace in the UI, as a TRE administrator, and add the hostname to **Allowed
internet domains**. A leading `*.` allows subdomains. The change applies to:

- **Pods,** desktops and KubeVirt VMs, through the workspace's Cilium FQDN policy.
- **Azure VMs,** through the workspace's firewall rule collection group, once Azure has
  updated the firewall policy, which takes a few minutes.

Only HTTPS on port 443 is allowed. Rules by IP address or for other ports are not supported.

## Allow a domain for every workspace of a kind

Add it to `egress.allowedFQDNs` in the workspace template and push. Workspaces created from the
template get the template's list plus their own.

## Allow a domain for the platform

The cluster itself, for example to pull a new Crossplane package, uses `platform_egress_fqdns`
in Terraform. Add only exact hostnames you trust and apply.

## Not available yet

- **Forced tunnelling.** AzureTRE can route all traffic to an upstream enterprise firewall
  (`firewall_force_tunnel_ip`). KubeTRE cannot yet.
- **DNS filtering for VMs.** AzureTRE can attach an Azure DNS security policy. KubeTRE does
  not, although DNS for pods goes through Cilium's proxy.
