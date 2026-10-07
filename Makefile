BIN := $(CURDIR)/bin
CONTROLLER_GEN := $(BIN)/controller-gen
SETUP_ENVTEST := $(BIN)/setup-envtest
ENVTEST_K8S_VERSION ?= 1.37.0
IMG_TAG ?= dev

.PHONY: help
help: ## Show targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-16s %s\n", $$1, $$2}'

$(CONTROLLER_GEN):
	GOBIN=$(BIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest

$(BIN)/helm:
	GOBIN=$(BIN) go install helm.sh/helm/v3/cmd/helm@latest

$(SETUP_ENVTEST):
	GOBIN=$(BIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

.PHONY: generate
generate: $(CONTROLLER_GEN) ## Regenerate deepcopy code, CRDs and controller RBAC
	$(CONTROLLER_GEN) object:headerFile=hack/boilerplate.go.txt paths=./api/...
	$(CONTROLLER_GEN) crd paths=./api/... output:crd:artifacts:config=config/crd/bases
	$(CONTROLLER_GEN) rbac:roleName=kubetre-controller paths=./internal/... output:rbac:artifacts:config=config/rbac
	mv config/rbac/role.yaml config/rbac/controller_role.yaml

.PHONY: fmt vet
fmt: ## Format code
	gofmt -w .
vet: ## Vet code
	go vet ./...

.PHONY: test
test: $(SETUP_ENVTEST) $(BIN)/helm vet ## Run unit and envtest tests against a real API server
	KUBEBUILDER_ASSETS="$$($(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) -p path --bin-dir $(BIN)/envtest)" go test ./... -count=1

.PHONY: build
build: ## Build both binaries into bin/
	go build -o $(BIN)/controller ./cmd/controller
	go build -o $(BIN)/api ./cmd/api

.PHONY: run-controller run-api run-gateway
run-controller: ## Run the controller against the current kubeconfig
	go run ./cmd/controller
run-api: ## Run the API locally with header-based dev auth (never expose)
	go run ./cmd/api --insecure-dev-auth --listen=127.0.0.1:8090

# ---- Cloud publishing. Images are built inside Azure by ACR Tasks; nothing runs locally. ----
ACR_NAME ?= kubetreacr
REGISTRY ?= $(ACR_NAME).azurecr.io
CHART_DIR := $(BIN)/charts

.PHONY: acr-build
acr-build: ## Build controller, api and linux-desktop images in ACR (az login required)
	az acr build --registry $(ACR_NAME) --image kubetre/controller:$(IMG_TAG) --build-arg CMD=controller .
	az acr build --registry $(ACR_NAME) --image kubetre/api:$(IMG_TAG) --build-arg CMD=api .
	az acr build --registry $(ACR_NAME) --image kubetre/gateway:$(IMG_TAG) --build-arg CMD=gateway .
	az acr build --registry $(ACR_NAME) --image kubetre/linux-desktop:$(IMG_TAG) images/linux-desktop

# Third-party images and charts copied into ACR so the cluster never pulls from Docker Hub.
MIRROR := guacamole/guacamole:1.6.0 guacamole/guacd:1.6.0 envoyproxy/gateway:v1.9.2 \
          envoyproxy/envoy:distroless-v1.39.1 envoyproxy/gateway-helm:v1.9.2

.PHONY: acr-mirror
acr-mirror: ## Import the access gateway's third-party images and chart into ACR
	for ref in $(MIRROR); do az acr import --name $(ACR_NAME) --source docker.io/$$ref --image $$ref --force; done

.PHONY: charts-lint publish-charts
charts-lint: ## Lint every chart, including the Windows VM variant
	$(BIN)/helm lint --strict charts/*
	$(BIN)/helm lint --strict charts/research-vm --set os=windows,diskGi=128
publish-charts: charts-lint ## Package charts and push them to ACR as OCI artifacts
	rm -rf $(CHART_DIR) && mkdir -p $(CHART_DIR)
	$(BIN)/helm package charts/* -d $(CHART_DIR)
	az acr login --name $(ACR_NAME)
	for c in $(CHART_DIR)/*.tgz; do $(BIN)/helm push $$c oci://$(REGISTRY)/charts; done

.PHONY: manifests-check
manifests-check: ## Render the Kustomize deployment to stdout
	kubectl kustomize config/default

TERRAFORM := $(BIN)/terraform

.PHONY: infra-validate infra-test
infra-validate: ## Format-check and validate the Azure Terraform (no Azure account needed)
	$(TERRAFORM) -chdir=infra/azure fmt -check -recursive
	$(TERRAFORM) -chdir=infra/azure init -backend=false -input=false >/dev/null
	$(TERRAFORM) -chdir=infra/azure validate
infra-test: infra-validate ## Run the Terraform security tests against a mocked provider
	$(TERRAFORM) -chdir=infra/azure test

.PHONY: gitops-build
gitops-build: ## Render the three Flux stages in deploy/azure
	@for d in crossplane packages edge platform gateway; do echo "--- $$d"; kubectl kustomize deploy/azure/$$d | grep -c '^kind:'; done

.PHONY: install deploy
install: ## Install CRDs into the current cluster
	kubectl apply -k config/crd
deploy: ## Deploy CRDs, RBAC, controller and API
	kubectl apply -k config/default
