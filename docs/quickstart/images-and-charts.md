# 4. Images and charts

The cluster pulls only from its own registry, never from the internet. KubeTRE's images are
built inside Azure by ACR Tasks, and third-party images are copied in once. From the
repository root:

```sh
ACR=$(terraform -chdir=infra/azure output -raw acr_name)
make acr-build ACR_NAME=$ACR        # controller, API, gateway, UI, Linux desktop
make acr-mirror ACR_NAME=$ACR       # Guacamole, guacd, Envoy Gateway
make publish-charts ACR_NAME=$ACR   # Helm charts for the templates
```

`make acr-build` tags images with the `VERSION` file, which must equal `kubetre_version` in
Terraform. The registry denies public access except from `operator_ip_ranges`; ACR Tasks run
outside that range, so `make acr-build` opens the registry's network rules only for the
duration of the build and restores them afterwards.

If `az acr import` hits Docker Hub's rate limit, run `make acr-mirror` again later, or pass
Docker Hub credentials to `az acr import`.

For [Linux VMs on KubeVirt](../admin/kubevirt.md), also run `make acr-mirror-kubevirt`.
