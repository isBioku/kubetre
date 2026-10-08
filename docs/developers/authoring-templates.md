# Authoring templates

AzureTRE templates are Porter bundles. A KubeTRE template is a Kubernetes resource, plus a
Helm chart for anything that installs software. This page covers both kinds and the contract
between a chart and KubeTRE.

## Workspace templates

A `WorkspaceTemplate` defines a kind of workspace. It installs no chart: the controller builds
every workspace the same way, shaped by these fields.

```yaml
apiVersion: kubetre.io/v1alpha1
kind: WorkspaceTemplate
metadata:
  name: genomics
spec:
  displayName: Genomics workspace
  description: A workspace for genomics pipelines.
  version: 0.1.0
  podSecurity: restricted          # restricted (default), baseline or privileged
  virtualMachines:
    enabled: true                  # give each workspace an Azure subnet for VMs
  quota:                           # a ResourceQuota for the namespace
    requests.cpu: "16"
    requests.memory: 64Gi
    count/researchvms.platform.kubetre.io: "10"
  egress:
    allowedFQDNs: [pypi.org, files.pythonhosted.org]
  parametersSchema:                # the form; values are kept on the workspace
    type: object
    required: [costCentre]
    properties:
      costCentre: {type: string, title: Cost centre, pattern: "^CC-[0-9]{4}$"}
    additionalProperties: false
```

KubeTRE adds these fields to every workspace form: name, description, overview, owners,
researchers and allowed internet domains.

## Service templates

A `ServiceTemplate` defines a workspace service or, with `kind: UserResource`, a user
resource.

```yaml
apiVersion: kubetre.io/v1alpha1
kind: ServiceTemplate
metadata:
  name: rstudio
spec:
  displayName: RStudio
  description: RStudio Server in the browser.
  kind: UserResource                # or WorkspaceService (the default)
  parentTemplate: virtual-desktops  # required for user resources
  ownerAccount: false               # true: KubeTRE sets value "username" from the owner's email
  requiresVirtualMachines: false    # true: only in workspaces with a VM network
  requiredPodSecurity: restricted   # refused in workspaces that enforce something stricter
  chart:
    url: oci://registry.example.org/charts/rstudio
    version: 0.1.0
    provider: azure                 # authenticate to the registry with Flux's workload identity
  values:                           # set by the administrator; users cannot change these
    image: {repository: registry.example.org/rocker/rstudio, tag: "4.4"}
  valuesSchema:                     # the form; what the user may set
    type: object
    properties:
      size: {type: string, title: Size, enum: [small, medium], default: small, updateable: true}
    additionalProperties: false
```

- **No chart:** the service only groups user resources and is ready at once, like Virtual
  Desktops.
- **Values:** the chart receives the template's `values`, overlaid with the user's values.
  KubeTRE adds a `kubetre` block on top.

## Form schemas

- **Standard.** Write JSON Schema the UI's form library understands: types, `title`,
  `description`, `enum`, `default`, `minimum`/`maximum`, `pattern` and `required`. The API
  validates with JSON Schema 2020-12, and the form renders as Draft 7, so keep to their common
  subset.
- **Strict.** Use `additionalProperties: false`, so unexpected input is rejected.
- **Updates.** Mark a property `updateable: true` to let users change it after creation. All
  others are read-only on the update form. For services and user resources the API also
  refuses changes to them; for workspaces only the form enforces it so far.
- **No defaults applied.** The API does not apply defaults, so a chart must render with only
  the template's `values`. The tests check this.

## The chart contract

**What KubeTRE gives the chart.** Values under `kubetre`:

| Value | Meaning |
|---|---|
| `kubetre.workspace` | the workspace ID |
| `kubetre.service` | this service's name, also the Helm release name |
| `kubetre.owner` | the owner's email, for user resources |
| `kubetre.podSecurity` | the namespace's Pod Security level |

**What the chart must respect:**

- **The deployer account.** It is installed by the workspace's `kubetre-deployer` account,
  which may create pods, services, config maps, secrets, persistent volume claims, service
  accounts, deployments, stateful sets, jobs, pod disruption budgets, `ResearchVM`s, KubeVirt
  `VirtualMachine`s and CDI `DataVolume`s in its own namespace. Anything else fails, including
  network policies, quotas, RBAC and cluster-scoped objects.
- **Pod Security.** Its pods must pass the namespace's level, usually `restricted`.
- **Limits.** Set resource limits; otherwise the workspace's LimitRange defaults apply, and
  they are small.
- **Network.** The workspace's network policies apply. Only guacd can reach its pods, and
  outbound traffic is limited to the allowed domains.

**Offering a Connect button.** Create a Secret labelled `kubetre.io/connection: "true"`, with
`kubetre.io/service` set to the release name:

| Key | Value |
|---|---|
| `protocol` | `rdp`, `ssh` or `vnc` |
| `port` | the port |
| `hostname` | a Service in the workspace namespace: `<name>.<namespace>.svc.cluster.local` |
| `username`, `password` | credentials the gateway signs in with |

For an Azure VM, leave out `hostname` and annotate the Secret with
`kubetre.io/host-from-researchvm: <ResearchVM name>`. The gateway then reads the VM's private
IP and checks it lies in the workspace's subnet. The gateway trusts a connection Secret only
when Helm's release annotation matches its service label, so one chart cannot publish a
connection for another user's resource. See `charts/linux-desktop` and `charts/kubevirt-vm`.

## Cloud resources

Charts never hold cloud credentials. To create Azure resources, add a Crossplane composite
resource:

- **Definition.** An XRD in `crossplane/` defines it.
- **Azure implementation.** A composition in `crossplane/azure/` creates it.
- **Chart.** The chart creates an instance in the workspace namespace, as `charts/research-vm`
  does with `ResearchVM`.
- **Activation.** Add any new managed resource kind to `crossplane/install/activation.yaml`.
- **Permissions.** Give Crossplane's identity the narrowest Azure role that works, in
  `infra/azure/crossplane.tf`.

## Versioning

Use semantic versioning for chart and template versions. Changing a template's chart version
or values applies to existing resources created from it. Version upgrades per resource, as in
AzureTRE, are not available yet.

## Test and register

```sh
make test charts-lint
```

Then publish the chart and register the template: see
[Registering templates](../admin/registering-templates.md).
