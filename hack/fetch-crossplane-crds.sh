#!/usr/bin/env bash
# Refreshes the upstream CRDs that test/crossplane validates against. Keep these versions
# in step with crossplane/install/packages.yaml.
set -euo pipefail
CROSSPLANE=v2.4.2
FLUX=v2.9.6
ENVOY_GATEWAY=v1.9.2
CILIUM=v1.18.0
PROVIDER=v2.7.0
GOTEMPLATING=v0.13.0
ENVCONFIGS=v0.9.0

out="$(cd "$(dirname "$0")/.." && pwd)/test/crossplane/testdata/crds"
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT

fetch() { # repo tag dest files...
  local repo=$1 tag=$2 dest=$3; shift 3
  git clone -q --depth 1 --branch "$tag" --filter=blob:none --sparse "https://github.com/$repo.git" "$tmp/$repo"
  (cd "$tmp/$repo" && git sparse-checkout set --no-cone "${@/#//}")
  mkdir -p "$out/$dest"
  for f in "$@"; do cp "$tmp/$repo/$f" "$out/$dest/"; done
}

fetch crossplane/crossplane "$CROSSPLANE" crossplane \
  cluster/crds/apiextensions.crossplane.io_compositeresourcedefinitions.yaml \
  cluster/crds/apiextensions.crossplane.io_compositions.yaml \
  cluster/crds/apiextensions.crossplane.io_environmentconfigs.yaml \
  cluster/crds/pkg.crossplane.io_providers.yaml \
  cluster/crds/pkg.crossplane.io_functions.yaml \
  cluster/crds/pkg.crossplane.io_deploymentruntimeconfigs.yaml
p=package/crds
fetch crossplane-contrib/provider-upjet-azure "$PROVIDER" provider-azure \
  $p/azure.m.upbound.io_clusterproviderconfigs.yaml \
  $p/compute.azure.m.upbound.io_linuxvirtualmachines.yaml \
  $p/compute.azure.m.upbound.io_windowsvirtualmachines.yaml \
  $p/network.azure.m.upbound.io_firewallpolicyrulecollectiongroups.yaml \
  $p/network.azure.m.upbound.io_networkinterfaces.yaml \
  $p/network.azure.m.upbound.io_securitygroups.yaml \
  $p/network.azure.m.upbound.io_subnetnetworksecuritygroupassociations.yaml \
  $p/network.azure.m.upbound.io_subnetroutetableassociations.yaml \
  $p/network.azure.m.upbound.io_subnets.yaml
fetch crossplane-contrib/function-go-templating "$GOTEMPLATING" functions package/input/gotemplating.fn.crossplane.io_gotemplates.yaml
fetch crossplane-contrib/function-environment-configs "$ENVCONFIGS" functions package/input/environmentconfigs.fn.crossplane.io_inputs.yaml
# Flux: the CRDs KubeTRE's controller and deploy tree write.
curl -sSL -o "$tmp/flux.yaml" "https://github.com/fluxcd/flux2/releases/download/$FLUX/install.yaml"
mkdir -p "$out/flux"
python3 - "$tmp/flux.yaml" "$out/flux" <<'PY'
import sys, yaml
want = {"helmrepositories.source.toolkit.fluxcd.io", "helmreleases.helm.toolkit.fluxcd.io", "ocirepositories.source.toolkit.fluxcd.io"}
for d in yaml.safe_load_all(open(sys.argv[1])):
    if d and d.get("kind") == "CustomResourceDefinition" and d["metadata"]["name"] in want:
        open(f"{sys.argv[2]}/{d['metadata']['name']}.yaml", "w").write(yaml.safe_dump(d, sort_keys=False))
PY
# Gateway API and Envoy Gateway CRDs, from the Envoy Gateway chart the edge stage installs.
helm="$(cd "$(dirname "$0")/.." && pwd)/bin/helm"
"$helm" pull oci://docker.io/envoyproxy/gateway-helm --version "$ENVOY_GATEWAY" --untar --untardir "$tmp/eg" >/dev/null
mkdir -p "$out/envoy-gateway"
"$helm" template eg "$tmp/eg/gateway-helm" --include-crds | python3 -c '
import sys, yaml
want = {"gateways.gateway.networking.k8s.io", "gatewayclasses.gateway.networking.k8s.io", "httproutes.gateway.networking.k8s.io", "envoyproxies.gateway.envoyproxy.io"}
for d in yaml.safe_load_all(sys.stdin):
    if d and d.get("kind") == "CustomResourceDefinition" and d["metadata"]["name"] in want:
        open(sys.argv[1] + "/" + d["metadata"]["name"] + ".yaml", "w").write(yaml.safe_dump(d, sort_keys=False))
' "$out/envoy-gateway"

mkdir -p "$out/cilium"
curl -sSL -o "$out/cilium/ciliumnetworkpolicies.cilium.io.yaml" \
  "https://raw.githubusercontent.com/cilium/cilium/$CILIUM/pkg/k8s/apis/cilium.io/client/crds/v2/ciliumnetworkpolicies.yaml"
echo "updated $out"
