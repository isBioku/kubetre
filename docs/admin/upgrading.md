# Upgrading

## Upgrade KubeTRE

KubeTRE's version is in the `VERSION` file and in Terraform's `kubetre_version`. They must
match: `make acr-build` tags images with `VERSION`, and Flux deploys `kubetre_version`.

1. **Get the new version.** Pull or merge it into the repository Flux follows.
2. **Build the images:**

   ```sh
   make acr-build ACR_NAME=<acr name>
   ```

3. **Publish charts,** if any chart changed: `make publish-charts ACR_NAME=<acr name>`.
4. **Set the version.** Set `kubetre_version` to the new `VERSION`, then preview and apply.
   The apply changes only Flux's substitutions:

   ```sh
   terraform -chdir=infra/azure plan -var-file=profiles/small.tfvars
   terraform -chdir=infra/azure apply -var-file=profiles/small.tfvars
   ```

5. **Push.** Push the commit. Flux rolls out the controller, API, gateway and UI.

Apply Terraform before pushing when a change adds a substitution, otherwise Flux substitutes
an empty value.

## Upgrade a component

| Component | Where its version is pinned |
|---|---|
| Kubernetes | `kubernetes_version` (null uses the region default) |
| Crossplane | `deploy/azure/crossplane/crossplane.yaml` |
| Crossplane providers and functions | `crossplane/install/packages.yaml` |
| Envoy Gateway | `deploy/azure/edge`, and `MIRROR` in the Makefile |
| Guacamole and guacd | `deploy/azure/gateway/kustomization.yaml`, and `MIRROR` |
| KubeVirt and CDI | `deploy/azure/kubevirt`, and `KUBEVIRT_MIRROR` in the Makefile |
| AzureTRE's UI | `ui/`, vendored from an upstream commit; see `ui/README.md` |

Mirror new third-party images into ACR before pushing a version change. When Crossplane or
provider versions change, refresh the test CRDs with `hack/fetch-crossplane-crds.sh` and run
`make test`.

## Upgrade a resource

AzureTRE can move an existing workspace, service or user resource to a newer template version.
KubeTRE cannot yet: changing a template applies to resources created from it.
