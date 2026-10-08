# 9. Verify security

Do these checks before anyone uses real data.

1. **Clipboard and file transfer.** In a session, copying text out, pasting text in and
   dragging a file in must all fail.
2. **Privacy between researchers.** A second researcher in the same workspace must not see
   your VM in the UI. The workspace owner sees it, but **Connect** must fail for them.
3. **Workspace isolation.** From inside a VM, connecting to another workspace's subnet must
   fail, and browsing to a site not in the allowed domains must fail.
4. **No public addresses.** This must show no public IPs:

   ```sh
   az vm list-ip-addresses -g rg-<env>-workspaces -o table
   ```

5. **Audit.** The access gateway logs each session it opens:

   ```sh
   kubectl -n kubetre-gateway logs deploy/broker | grep "session opened"
   ```

6. **Cross-site protection.** A connect link opened from another website must be refused
   with "forbidden".

## Known security gaps

These are recorded in the architecture decisions. Close them, or accept them in writing with
your information security officer, before using sensitive data:

- **Data movement.** There is no airlock, so no sanctioned way to move data in or out.
- **Package mirrors.** There are none, so machines reach their allowed domains directly.
- **Certificates.** TLS certificates are installed and renewed by hand.
- **Sessions and audit.** There is no session recording, and audit is limited to logs.
- **Container desktops.** They share a node's kernel with other workloads, unlike VMs.
- **KubeVirt VMs.** They are isolated like pods. At the firewall they look like any other pod.
- **Broker permissions.** The access gateway's broker can read Secrets across the cluster.
- **Gateway pool.** DaemonSets on the gateway pool get addresses that VM security groups trust.
- **Automation.** API roles can only be assigned to users, so there is no automation identity
  for CI.

See [ADR 8](../adr/0008-vm-security-model.md), [ADR 9](../adr/0009-access-gateway.md) and
[ADR 11](../adr/0011-kubevirt-vms.md).
