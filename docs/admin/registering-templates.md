# Registering templates

AzureTRE builds Porter bundles, publishes them to its registry and registers them through
the API. In KubeTRE a template is a Kubernetes resource in Git, and Flux registers it.

## Register a template

1. **Publish the chart,** if the template installs one:

   ```sh
   make publish-charts ACR_NAME=<acr name>
   ```

   This lints, packages and pushes every chart in `charts/` to `oci://<acr>/charts/<name>`.

2. **Add the template.** Put the `WorkspaceTemplate` or `ServiceTemplate` in
   `config/templates`, and add it to `config/templates/kustomization.yaml`. Use
   `registry.example.org` in chart and image references; the Azure overlay
   (`deploy/azure/platform/kustomization.yaml`) replaces it with the environment's registry.
3. **Test it.**

   ```sh
   make test
   ```

   The tests check every template against the CRD schemas and render every chart with only
   the template's values.

4. **Commit and push.** Flux applies it within five minutes, or at once if you trigger it.
5. **Check it.**

   ```sh
   kubectl get workspacetemplates,servicetemplates
   ```

The template appears in the UI's template picker straight away.

## Change or remove a template

- **Change:** edit the YAML and push. Existing resources keep running; changed chart versions
  or values apply to them at their next reconciliation.
- **Remove:** delete the YAML and its kustomization entry, and push. Remove the resources
  created from it first.

AzureTRE's per-resource template upgrade, `PATCH` with `templateVersion`, is not available
yet.

See [Authoring templates](../developers/authoring-templates.md) to write one.
