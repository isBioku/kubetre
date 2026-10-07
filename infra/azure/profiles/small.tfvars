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

research_vm_sizes = {
  small  = "Standard_D2s_v5"
  medium = "Standard_D2s_v5"
  large  = "Standard_D4s_v5"
}

# Cheaper for test windows. Keep both on for anything holding real data.
aks_sku_tier       = "Free"
defender_enabled   = false
log_retention_days = 30
