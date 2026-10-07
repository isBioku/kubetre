// Package deploy checks the Flux GitOps tree in deploy/azure: every placeholder is one
// Terraform supplies, and after substitution every object is accepted by the real
// Kubernetes, Crossplane, provider and Flux schemas with strict field validation.
package deploy

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/yaml"

	"github.com/isBioku/kubetre/test/xrdcrd"
)

var (
	root        = filepath.Join("..", "..")
	placeholder = regexp.MustCompile(`\$\{([A-Z0-9_]+)\}`)
)

// Realistic values for every substitution Terraform provides.
var values = map[string]string{
	"AZURE_TENANT_ID":          "11111111-1111-1111-1111-111111111111",
	"AZURE_SUBSCRIPTION_ID":    "22222222-2222-2222-2222-222222222222",
	"LOCATION":                 "uksouth",
	"CROSSPLANE_CLIENT_ID":     "66666666-6666-6666-6666-666666666666",
	"WORKSPACE_RESOURCE_GROUP": "rg-kubetredev-workspaces",
	"VNET_NAME":                "vnet-kubetredev",
	"ROUTE_TABLE_ID":           "/subscriptions/22222222-2222-2222-2222-222222222222/resourceGroups/rg-kubetredev/providers/Microsoft.Network/routeTables/rt-kubetredev-egress",
	"FIREWALL_POLICY_ID":       "/subscriptions/22222222-2222-2222-2222-222222222222/resourceGroups/rg-kubetredev/providers/Microsoft.Network/firewallPolicies/fwp-kubetredev",
	"ACCESS_GATEWAY_PREFIX":    "10.224.8.0/24",
	"SHARED_SERVICES_PREFIX":   "10.224.9.0/24",
	"VM_ADDRESS_POOL":          "10.240.0.0/16",
	"ENCRYPTION_AT_HOST":       "true",
	"ACR_LOGIN_SERVER":         "acrkubetredev.azurecr.io",
	"KUBETRE_VERSION":          "0.1.0",
	"OIDC_ISSUER":              "https://login.microsoftonline.com/11111111-1111-1111-1111-111111111111/v2.0",
	"OIDC_AUDIENCE":            "77777777-7777-7777-7777-777777777777",
	"ROLES_CLAIM":              "roles",
	"IDP_HOST":                 "login.microsoftonline.com",
	"GATEWAY_HOSTNAME":         "gateway.tre.example.org",
	"GATEWAY_OIDC_CLIENT_ID":   "88888888-8888-8888-8888-888888888888",
	"GATEWAY_ILB_IP":           "10.224.9.10",
	"ILB_SUBNET_NAME":          "snet-shared-ilb",
}

// stages are the Flux kustomizations in apply order.
var stages = []string{"crossplane", "packages", "edge", "platform", "gateway"}

// terraformSubstitutions reads the keys of local.gitops_substitutions from gitops.tf.
func terraformSubstitutions(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, "infra", "azure", "gitops.tf"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	start := strings.Index(src, "gitops_substitutions = {")
	end := strings.Index(src[start:], "\n  }")
	keys := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^\s+([A-Z0-9_]+)\s+=`).FindAllStringSubmatch(src[start:start+end], -1) {
		keys[m[1]] = true
	}
	if len(keys) == 0 {
		t.Fatal("no substitutions found in gitops.tf")
	}
	return keys
}

func build(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("kubectl", "kustomize", filepath.Join(root, "deploy", "azure", dir)).CombinedOutput()
	if err != nil {
		t.Fatalf("kustomize %s: %v\n%s", dir, err, out)
	}
	return string(out)
}

func docs(s string) []*unstructured.Unstructured {
	var out []*unstructured.Unstructured
	for _, d := range strings.Split("\n"+s, "\n---") {
		u := &unstructured.Unstructured{}
		if strings.TrimSpace(d) == "" {
			continue
		}
		if err := yaml.Unmarshal([]byte(d), &u.Object); err != nil {
			panic(err)
		}
		if len(u.Object) > 0 {
			out = append(out, u)
		}
	}
	return out
}

func substitutionDisabled(u *unstructured.Unstructured) bool {
	return u.GetAnnotations()["kustomize.toolkit.fluxcd.io/substitute"] == "disabled"
}

func TestPlaceholdersMatchTerraformOutputs(t *testing.T) {
	tf := terraformSubstitutions(t)
	for k := range tf {
		if _, ok := values[k]; !ok {
			t.Errorf("Terraform provides %s but this test has no sample value for it", k)
		}
	}
	used := map[string]bool{}
	for _, dir := range stages {
		for _, u := range docs(build(t, dir)) {
			if substitutionDisabled(u) {
				continue
			}
			raw, _ := yaml.Marshal(u.Object)
			for _, m := range placeholder.FindAllStringSubmatch(string(raw), -1) {
				used[m[1]] = true
				if !tf[m[1]] {
					t.Errorf("%s/%s %s uses ${%s}, which Terraform does not supply", dir, u.GetKind(), u.GetName(), m[1])
				}
			}
		}
	}
	var unused []string
	for k := range tf {
		if !used[k] {
			unused = append(unused, k)
		}
	}
	sort.Strings(unused)
	if len(unused) > 0 {
		t.Errorf("Terraform supplies values nothing uses: %v", unused)
	}
}

func TestSubstitutedManifestsMatchSchemas(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("KUBEBUILDER_ASSETS not set; run `make test`")
	}
	crds := filepath.Join(root, "test", "crossplane", "testdata", "crds")
	env := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join(crds, "crossplane"), filepath.Join(crds, "provider-azure"), filepath.Join(crds, "flux"),
			filepath.Join(crds, "envoy-gateway"), filepath.Join(crds, "cilium"),
			filepath.Join(root, "config", "crd", "bases"),
		},
		CRDs: []*apiextensionsv1.CustomResourceDefinition{
			xrdcrd.FromFile(filepath.Join(root, "crossplane", "xrd-workspacenetwork.yaml")),
			xrdcrd.FromFile(filepath.Join(root, "crossplane", "xrd-researchvm.yaml")),
		},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := env.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = env.Stop() }()
	c, _ := client.New(cfg, client.Options{})
	ctx := context.Background()

	for _, dir := range stages {
		objs := docs(build(t, dir))
		// Namespaces first, for real, so namespaced objects can be dry-run into them.
		for _, u := range objs {
			if u.GetKind() == "Namespace" {
				if err := c.Create(ctx, u.DeepCopy()); err != nil && !strings.Contains(err.Error(), "already exists") {
					t.Fatal(err)
				}
			}
		}
		for _, u := range objs {
			raw, _ := yaml.Marshal(u.Object)
			text := string(raw)
			if !substitutionDisabled(u) {
				text = placeholder.ReplaceAllStringFunc(text, func(m string) string { return values[m[2:len(m)-1]] })
			}
			sub := &unstructured.Unstructured{}
			if err := yaml.Unmarshal([]byte(text), &sub.Object); err != nil {
				t.Fatalf("%s/%s does not parse after substitution: %v", dir, u.GetName(), err)
			}
			if u.GetKind() == "Namespace" || u.GetKind() == "CustomResourceDefinition" {
				continue
			}
			if err := c.Create(ctx, sub, client.DryRunAll, client.FieldValidation("Strict")); err != nil {
				t.Errorf("%s: %s %s/%s rejected: %v", dir, sub.GetKind(), sub.GetNamespace(), sub.GetName(), err)
			}
		}
	}
}

func TestEnvironmentConfigKeepsDefaultsAndTypes(t *testing.T) {
	for _, u := range docs(build(t, "platform")) {
		if u.GetKind() != "EnvironmentConfig" {
			continue
		}
		raw, _ := yaml.Marshal(u.Object)
		text := placeholder.ReplaceAllStringFunc(string(raw), func(m string) string { return values[m[2:len(m)-1]] })
		sub := map[string]any{}
		_ = yaml.Unmarshal([]byte(text), &sub)
		data := sub["data"].(map[string]any)
		if _, ok := data["vmSizes"].(map[string]any); !ok {
			t.Error("the overlay must keep vmSizes from crossplane/azure/environmentconfig.yaml")
		}
		if data["encryptionAtHost"] != true {
			t.Errorf("encryptionAtHost = %#v; it must be a boolean after substitution", data["encryptionAtHost"])
		}
		if p := data["accessGatewayPrefixes"].([]any); len(p) != 1 || p[0] != values["ACCESS_GATEWAY_PREFIX"] {
			t.Errorf("accessGatewayPrefixes = %v", p)
		}
		return
	}
	t.Fatal("no EnvironmentConfig in the platform stage")
}

// The Flux stages Terraform declares must be exactly the directories under deploy/azure.
func TestTerraformDeclaresEveryStage(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(root, "infra", "azure", "gitops.tf"))
	if err != nil {
		t.Fatal(err)
	}
	declared := map[string]bool{}
	for _, m := range regexp.MustCompile(`path\s+=\s+"\$\{var\.gitops_path\}/([a-z-]+)"`).FindAllStringSubmatch(string(raw), -1) {
		declared[m[1]] = true
	}
	for _, s := range stages {
		if !declared[s] {
			t.Errorf("stage %s is not declared in gitops.tf", s)
		}
	}
	if len(declared) != len(stages) {
		t.Errorf("gitops.tf declares %v, test expects %v", declared, stages)
	}
}

// The gateway's Envoy must sit on the internal load balancer address the firewall forwards to.
func TestGatewayUsesTheInternalLoadBalancer(t *testing.T) {
	for _, u := range docs(build(t, "gateway")) {
		if u.GetKind() != "EnvoyProxy" {
			continue
		}
		ann, _, _ := unstructured.NestedStringMap(u.Object, "spec", "provider", "kubernetes", "envoyService", "annotations")
		if ann["service.beta.kubernetes.io/azure-load-balancer-internal"] != "true" ||
			ann["service.beta.kubernetes.io/azure-load-balancer-ipv4"] != "${GATEWAY_ILB_IP}" {
			t.Fatalf("envoy service annotations = %v", ann)
		}
		return
	}
	t.Fatal("no EnvoyProxy in the gateway stage")
}

// Registered templates must pull from this environment's registry, never the placeholder.
func TestTemplatesUseTheEnvironmentRegistry(t *testing.T) {
	n := 0
	for _, u := range docs(build(t, "platform")) {
		if u.GetKind() != "ServiceTemplate" {
			continue
		}
		n++
		raw, _ := yaml.Marshal(u.Object)
		if strings.Contains(string(raw), "registry.example.org") {
			t.Errorf("ServiceTemplate %s still points at the placeholder registry", u.GetName())
		}
		if url, _, _ := unstructured.NestedString(u.Object, "spec", "chart", "url"); !strings.HasPrefix(url, "oci://${ACR_LOGIN_SERVER}/charts/") {
			t.Errorf("ServiceTemplate %s chart url = %s", u.GetName(), url)
		}
	}
	if n != 3 {
		t.Fatalf("expected 3 ServiceTemplates in the platform stage, found %d", n)
	}
}

// The API is published on the gateway, and the cross-namespace grant covers only that.
func TestAPIIsRoutedThroughTheGateway(t *testing.T) {
	var route, grant *unstructured.Unstructured
	for _, u := range docs(build(t, "gateway")) {
		switch {
		case u.GetKind() == "HTTPRoute" && u.GetName() == "api":
			route = u
		case u.GetKind() == "ReferenceGrant":
			grant = u
		}
	}
	if route == nil || grant == nil {
		t.Fatal("missing the API HTTPRoute or its ReferenceGrant")
	}
	rules, _, _ := unstructured.NestedSlice(route.Object, "spec", "rules")
	backend := rules[0].(map[string]any)["backendRefs"].([]any)[0].(map[string]any)
	if backend["name"] != "kubetre-api" || backend["namespace"] != "kubetre-system" {
		t.Fatalf("api route backend = %v", backend)
	}
	to, _, _ := unstructured.NestedSlice(grant.Object, "spec", "to")
	if len(to) != 1 || to[0].(map[string]any)["name"] != "kubetre-api" {
		t.Fatalf("ReferenceGrant must name only the kubetre-api Service: %v", to)
	}
}

// make acr-build tags images with VERSION; Flux deploys kubetre_version. They must agree.
func TestImageVersionsAgree(t *testing.T) {
	v, err := os.ReadFile(filepath.Join(root, "VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	tf, _ := os.ReadFile(filepath.Join(root, "infra", "azure", "variables.tf"))
	m := regexp.MustCompile(`(?s)variable "kubetre_version" \{.*?default\s+=\s+"([^"]+)"`).FindStringSubmatch(string(tf))
	if m == nil || m[1] != strings.TrimSpace(string(v)) {
		t.Fatalf("kubetre_version default %v does not match VERSION %q", m, strings.TrimSpace(string(v)))
	}
}
