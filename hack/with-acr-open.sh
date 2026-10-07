#!/usr/bin/env bash
# Runs a command with the registry's network rules temporarily open, then closes them again.
#
#   hack/with-acr-open.sh <acr-name> <command...>
#
# ACR quick tasks (az acr build) run on shared agents without a managed identity, so they
# cannot use the "bypass for tasks" setting and are blocked by a Deny-by-default registry.
# During the window the registry still requires Entra ID authentication (no admin user, no
# anonymous pull); only the IP filter is relaxed. The trap restores Deny on any exit.
set -euo pipefail
acr="${1:?usage: $0 <acr-name> <command...>}"; shift

restore() {
  az acr update --name "$acr" --default-action Deny --only-show-errors >/dev/null \
    && echo "registry $acr network rules restored to Deny" \
    || echo "WARNING: could not restore Deny on $acr; run: az acr update -n $acr --default-action Deny" >&2
}
trap restore EXIT INT TERM

az acr update --name "$acr" --default-action Allow --only-show-errors >/dev/null
echo "registry $acr network rules temporarily open for: $*"
"$@"
