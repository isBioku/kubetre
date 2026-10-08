# 7. First workspace

Templates are already registered: Flux applied them in [step 6](gitops.md). In AzureTRE you
would register the base workspace bundle first; here there is nothing to do.

```sh
kubectl get workspacetemplates
```

## Create the workspace

1. **Sign in.** Open `https://<gateway_hostname>` and sign in with an account that has
   `TREAdmin`. This is AzureTRE's UI.
2. **Start a workspace.** On **Workspaces**, select **Create new**.
3. **Pick a template.** Select **Create** under **Workspace with virtual machines**.
4. **Fill in the form:**
   - **Name for the workspace** and **Description of the workspace**.
   - **Workspace owners** and **Workspace researchers**, by email address. Owners default to
     you.
   - **Allowed internet domains**, for example `pypi.org`, beyond those the template allows.
   - **Cost centre**, in the form `CC-1234`.
5. **Submit.** Then select **Go to resource**.

The workspace shows *deploying* while the controller creates its namespace and Crossplane
creates its subnet, security group and firewall rules. After a few minutes it shows
*deployed*. The bell icon at the top right follows the operation, as in AzureTRE.

There is no authentication type to choose and no app registration to create for the
workspace: members are the email addresses in the form.

## What was created

```sh
kubectl get workspace                                   # PHASE Ready
kubectl get ns ws-<id> --show-labels                    # pod-security enforce=restricted
kubectl -n ws-<id> get networkpolicy,resourcequota,limitrange
kubectl -n ws-<id> get workspacenetwork                 # the VM subnet, SYNCED and READY
```

In Azure, `rg-<env>-workspaces` now holds `nsg-ws-<id>`, the VNet holds `snet-ws-<id>`, and
the firewall policy holds `rcg-ws-<id>`.

Next, [add virtual desktops](virtual-desktops.md).
