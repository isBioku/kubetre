# Small test profile: the whole platform plus one research VM in 10 vCPUs, for short test
# runs on a new subscription. Use with: terraform apply -var-file=profiles/small.tfvars
#
# Nodes: system 1 x D2s_v5, gateway 1 x D2s_v5, work 1 x D4s_v5 = 8 vCPUs; a "medium" research
# VM uses a further 2. Autoscaling is pinned (min = max) so it never asks for more quota, and
# node image upgrades that need a surge node will wait. Not for production.

system_vm_size     = "Standard_D2s_v5"
system_node_count  = { min = 1, max = 1 }
work_vm_size       = "Standard_D4s_v5"
work_node_count    = { min = 1, max = 1 }
gateway_vm_size    = "Standard_D2s_v5"
gateway_node_count = { min = 1, max = 1 }

# Used only when kubevirt_enabled is set: one D4s_v5 for KubeVirt VMs.
kubevirt_node_count = { min = 1, max = 1 }

research_vm_sizes = {
  small  = "Standard_D2s_v5"
  medium = "Standard_D2s_v5"
  large  = "Standard_D4s_v5"
}

# Standard even for tests: on the Free tier the API server kept dropping out under Flux and
# Crossplane, so Crossplane, Flux and the KubeTRE controller lost leader election in a loop.
aks_sku_tier = "Standard"

# Cheaper for test windows. Keep it on for anything holding real data.
defender_enabled   = false
log_retention_days = 30
