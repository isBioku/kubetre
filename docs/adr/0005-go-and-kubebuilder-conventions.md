# 5. Go, controller-runtime and kubebuilder markers

Status: Accepted, 2026-10-07

## Decision
Both the controller and the API are written in Go and share the CRD types in `api/v1alpha1`.
CRDs, deepcopy code and controller RBAC are generated with controller-gen. Controller tests run
against a real `kube-apiserver` and `etcd` through envtest, so no cluster or Docker is needed.

## Consequences
- One toolchain and one set of types instead of a Python API mirroring Kubernetes objects.
- AzureTRE's Python code is a reference for behaviour, not a code source.
