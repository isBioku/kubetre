# Architecture decisions

Each decision records the context, what was decided and its consequences.

| ADR | Decision |
|---|---|
| [1](0001-operator-model.md) | Kubernetes is the control plane: resources and controllers instead of a queue and a resource processor |
| [2](0002-tenancy.md) | One namespace per workspace, default-deny |
| [3](0003-identity.md) | Plain OIDC with a single API audience |
| [4](0004-egress.md) | FQDN egress allowlists through Cilium, failing closed |
| [5](0005-go-and-kubebuilder-conventions.md) | Go, controller-runtime and kubebuilder markers |
| [6](0006-templates-and-desktops.md) | Templates and research desktops |
| [7](0007-cloud-targets.md) | Cloud targets: AKS first, then EKS and GKE |
| [8](0008-vm-security-model.md) | Security model for research VMs |
| [9](0009-access-gateway.md) | The access gateway: an OIDC broker in front of stock Guacamole |
| [10](0010-azuretre-ui.md) | Run AzureTRE's own UI against an AzureTRE-compatible API |
| [11](0011-kubevirt-vms.md) | Optional Linux VMs on KubeVirt, beside Azure VMs |
