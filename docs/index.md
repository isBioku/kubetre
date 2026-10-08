# KubeTRE

KubeTRE is an experimental, community port of Microsoft's
[Azure Trusted Research Environment (AzureTRE)](https://github.com/microsoft/AzureTRE) to
Kubernetes. It keeps AzureTRE's user interface and API, so researchers and administrators work
exactly as they do in AzureTRE, and replaces the back end with Kubernetes operators, Helm,
Flux and Crossplane.

!!! note "Not a Microsoft product"
    AzureTRE and its user interface are Microsoft's work, used here under the MIT licence.
    KubeTRE is an independent experiment to test whether AzureTRE can be cloud-native. It is
    not an official Microsoft release and is not supported by Microsoft.

## What is a Trusted Research Environment?

Organisations that hold sensitive data, such as health records, clinical trial results or
commercial designs, need to let researchers work with it without letting it leak. A Trusted
Research Environment (TRE) gives each project a **workspace**: a secure boundary with the
tools researchers need inside, inbound access only for authorised people, and outbound access
only to approved destinations.

A TRE usually sits next to a data platform that supplies research-ready datasets. The data
platform is not part of the TRE.

## What KubeTRE offers

- **AzureTRE's experience.** Microsoft's AzureTRE UI, unchanged in look and behaviour, backed
  by an AzureTRE-compatible API.
- **Self-service for administrators.** TRE administrators create workspaces from templates.
- **Self-service for research teams.** Workspace owners add workspace services; researchers
  create their own Windows or Linux machines and open them in the browser.
- **Isolation by default.** Each workspace is an isolated Kubernetes namespace with a quota,
  default-deny networking and an internet allowlist. Workspaces with VMs also get their own
  Azure subnet, network security group and firewall rules.
- **Controlled access.** One access gateway signs users in, checks that they own the machine,
  and opens a Guacamole session with clipboard and file transfer switched off.
- **Choice of machines.** Azure VMs, Linux container desktops, and optionally Linux VMs on
  KubeVirt.
- **Extensible.** New tools are Helm charts registered as templates.
- **Entra ID.** Sign-in with Microsoft Entra ID; any OpenID Connect provider can be used.

AzureTRE features not available yet include the airlock, shared services such as package
mirrors, cost reporting and most workspace service templates. See
[KubeTRE and AzureTRE](overview/azuretre-comparison.md).

## Where to start

| You want to | Read |
|---|---|
| Understand how it works | [Architecture](overview/architecture.md) |
| Deploy it | [Quickstart](quickstart/index.md) |
| Use it as a researcher | [Using KubeTRE](using/index.md) |
| Add your own tools | [Authoring templates](developers/authoring-templates.md) |
| Fix a problem | [Troubleshooting](troubleshooting/index.md) |
