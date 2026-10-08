# Quickstart

These steps take you from an empty Azure subscription to a researcher working in a Windows VM
inside an isolated workspace, created entirely from AzureTRE's UI. Follow them in order. The
first deployment takes two to three hours, most of it waiting for Azure.

| Step | What you do | AzureTRE equivalent |
|---|---|---|
| [1. Prerequisites](prerequisites.md) | tools, permissions, quota, decisions | Prerequisites, deployment repository |
| [2. Identity](identity.md) | create the Entra ID applications | AD tenant, authentication setup |
| [3. Infrastructure](infrastructure.md) | Terraform: AKS, network, firewall, registry | Pre-deployment and deployment steps |
| [4. Images and charts](images-and-charts.md) | build and copy images into the registry | part of `make all` |
| [5. Gateway hostname and secrets](gateway.md) | DNS, TLS certificate, sign-in secret | `make letsencrypt` |
| [6. GitOps](gitops.md) | let Flux install the platform, then check it | part of `make all` |
| [7. First workspace](first-workspace.md) | create a workspace in the UI | Install base workspace |
| [8. Virtual desktops](virtual-desktops.md) | add Virtual Desktops and a VM, connect | Install workspace service and user resource |
| [9. Verify security](verify.md) | check the controls before real data | |

There is no CI/CD pipeline yet: deployment runs from a workstation. AzureTRE's step for
configuring shared services has no equivalent, because KubeTRE has none yet.

!!! warning "Before using sensitive data"
    KubeTRE is experimental. Read the [security gaps](verify.md#known-security-gaps) and agree
    them with your information security officer before any sensitive data goes in.
