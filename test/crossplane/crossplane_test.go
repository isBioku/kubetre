// Package crossplane validates KubeTRE's Crossplane definitions against the real Crossplane,
// function and Azure provider schemas, and renders the compositions the way
// function-go-templating does, then submits every generated Azure resource to a real API
// server (server-side dry run) so schema and CEL errors surface before reaching a cloud.
package crossplane

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/yaml"

	corev1 "k8s.io/api/core/v1"

	"github.com/isBioku/kubetre/test/xrdcrd"
)

var (
	root = filepath.Join("..", "..")
	k8s  client.Client
)

func TestMain(m *testing.M) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		println("KUBEBUILDER_ASSETS not set; run `make test`")
		os.Exit(0)
	}
	crds := filepath.Join("testdata", "crds")
	env := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join(crds, "crossplane"), filepath.Join(crds, "functions"), filepath.Join(crds, "provider-azure"),
		},
		CRDs: []*apiextensionsv1.CustomResourceDefinition{
			xrdcrd.FromFile(filepath.Join(root, "crossplane", "xrd-workspacenetwork.yaml")),
			xrdcrd.FromFile(filepath.Join(root, "crossplane", "xrd-researchvm.yaml")),
		},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := env.Start()
	if err != nil {
		panic(err)
	}
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	k8s, _ = client.New(cfg, client.Options{Scheme: scheme})
	_ = k8s.Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ws-study"}})
	code := m.Run()
	_ = env.Stop()
	os.Exit(code)
}

func readYAML(path string) []*unstructured.Unstructured {
	raw, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return parseDocs(string(raw))
}

func parseDocs(s string) []*unstructured.Unstructured {
	var out []*unstructured.Unstructured
	for _, d := range strings.Split("\n"+s, "\n---") {
		if strings.TrimSpace(d) == "" {
			continue
		}
		u := &unstructured.Unstructured{}
		if err := yaml.Unmarshal([]byte(d), &u.Object); err != nil {
			panic(err.Error() + "\n" + d)
		}
		if len(u.Object) > 0 {
			out = append(out, u)
		}
	}
	return out
}

func dryRunCreate(t *testing.T, u *unstructured.Unstructured) error {
	t.Helper()
	// Strict field validation rejects unknown or misspelled fields instead of pruning them.
	return k8s.Create(context.Background(), u.DeepCopy(), client.DryRunAll, client.FieldValidation("Strict"))
}

// ---- static manifests ----

func TestManifestsMatchCrossplaneSchemas(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join(root, "crossplane", "*.yaml"))
	more, _ := filepath.Glob(filepath.Join(root, "crossplane", "*", "*.yaml"))
	files = append(files, more...)
	if len(files) < 7 {
		t.Fatalf("expected crossplane manifests, found %v", files)
	}
	for _, f := range files {
		if filepath.Base(f) == "kustomization.yaml" {
			continue
		}
		for _, u := range readYAML(f) {
			if strings.HasPrefix(u.GetAPIVersion(), "pkg.crossplane.io") {
				continue // package installs are not in the vendored CRDs
			}
			if err := dryRunCreate(t, u); err != nil {
				t.Errorf("%s %s/%s rejected: %v", filepath.Base(f), u.GetKind(), u.GetName(), err)
			}
		}
	}
}

// Control: the provider schemas really are enforced, so the passing checks above mean something.
func TestProviderSchemasAreEnforced(t *testing.T) {
	vm := &unstructured.Unstructured{}
	vm.SetAPIVersion("compute.azure.m.upbound.io/v1beta1")
	vm.SetKind("WindowsVirtualMachine")
	vm.SetName("control")
	vm.SetNamespace("ws-study")
	_ = unstructured.SetNestedField(vm.Object, "Standard_D2s_v5", "spec", "forProvider", "size")
	if err := dryRunCreate(t, vm); err == nil || !strings.Contains(err.Error(), "location is a required parameter") {
		t.Fatalf("expected a missing-location error, got %v", err)
	}
	_ = unstructured.SetNestedField(vm.Object, "uksouth", "spec", "forProvider", "location")
	_ = unstructured.SetNestedField(vm.Object, "ReadWrite", "spec", "forProvider", "osDisk", "caching")
	_ = unstructured.SetNestedField(vm.Object, true, "spec", "forProvider", "notARealField")
	if err := dryRunCreate(t, vm); err == nil || !strings.Contains(err.Error(), "notARealField") {
		t.Fatalf("expected the unknown field to be rejected, got %v", err)
	}
}

func TestFunctionInputsMatchFunctionSchemas(t *testing.T) {
	for _, f := range []string{"composition-workspacenetwork.yaml", "composition-researchvm.yaml"} {
		comp := readYAML(filepath.Join(root, "crossplane", "azure", f))[0]
		steps, _, _ := unstructured.NestedSlice(comp.Object, "spec", "pipeline")
		for _, s := range steps {
			in, ok := s.(map[string]any)["input"].(map[string]any)
			if !ok {
				continue
			}
			u := &unstructured.Unstructured{Object: runtime.DeepCopyJSON(in)}
			u.SetName("check")
			u.SetNamespace("default")
			if err := dryRunCreate(t, u); err != nil {
				t.Errorf("%s step %v input rejected: %v", f, s.(map[string]any)["step"], err)
			}
		}
	}
}

// ---- rendering ----

func environment(t *testing.T) map[string]any {
	t.Helper()
	ec := readYAML(filepath.Join(root, "crossplane", "azure", "environmentconfig.yaml"))[0]
	data, _, _ := unstructured.NestedMap(ec.Object, "data")
	return data
}

func templateOf(t *testing.T, file string) string {
	t.Helper()
	comp := readYAML(filepath.Join(root, "crossplane", "azure", file))[0]
	steps, _, _ := unstructured.NestedSlice(comp.Object, "spec", "pipeline")
	for _, s := range steps {
		if tpl, ok, _ := unstructured.NestedString(s.(map[string]any), "input", "inline", "template"); ok {
			return tpl
		}
	}
	t.Fatalf("no inline template in %s", file)
	return ""
}

// render mirrors function-go-templating: sprig functions, missingkey=error, and the
// observed composite and context passed as data.
func render(t *testing.T, file string, xr map[string]any, observed map[string]any) (composed []*unstructured.Unstructured, status map[string]any) {
	t.Helper()
	tpl, err := template.New(file).Funcs(sprig.TxtFuncMap()).Option("missingkey=error").Parse(templateOf(t, file))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	obs := map[string]any{"composite": map[string]any{"resource": xr}}
	if observed != nil {
		obs["resources"] = observed
	}
	data := map[string]any{
		"observed": obs,
		"context":  map[string]any{"apiextensions.crossplane.io/environment": environment(t)},
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, u := range parseDocs(buf.String()) {
		if u.GetAnnotations()["gotemplating.fn.crossplane.io/composition-resource-name"] == "" {
			status, _, _ = unstructured.NestedMap(u.Object, "status")
			continue
		}
		composed = append(composed, u)
	}
	return composed, status
}

func xr(kind, name string, params map[string]any) map[string]any {
	return map[string]any{
		"apiVersion": "platform.kubetre.io/v1alpha1", "kind": kind,
		"metadata": map[string]any{"name": name, "namespace": "ws-study"},
		"spec":     map[string]any{"parameters": params},
	}
}

// submit gives each composed resource a name, as Crossplane does, and dry-runs it.
func submit(t *testing.T, composed []*unstructured.Unstructured) map[string]*unstructured.Unstructured {
	t.Helper()
	byName := map[string]*unstructured.Unstructured{}
	for _, u := range composed {
		n := u.GetAnnotations()["gotemplating.fn.crossplane.io/composition-resource-name"]
		u.SetName("x-" + n)
		u.SetNamespace("ws-study")
		if err := dryRunCreate(t, u); err != nil {
			t.Errorf("%s (%s) rejected by the provider schema: %v", n, u.GetKind(), err)
		}
		byName[n] = u
	}
	return byName
}

func TestWorkspaceNetworkComposition(t *testing.T) {
	params := map[string]any{"addressPrefix": "10.240.0.64/26", "firewallPriority": float64(1001),
		"allowedFQDNs": []any{"pypi.org", "*.r-project.org"}}
	composed, status := render(t, "composition-workspacenetwork.yaml", xr("WorkspaceNetwork", "vm-network", params), nil)
	got := submit(t, composed)

	for _, want := range []string{"subnet", "nsg", "nsg-association", "route-association", "egress-rules"} {
		if got[want] == nil {
			t.Fatalf("missing composed resource %q", want)
		}
	}
	if got["subnet"].GetAnnotations()["crossplane.io/external-name"] != "snet-ws-study" {
		t.Errorf("subnet name = %v", got["subnet"].GetAnnotations())
	}
	if l := got["subnet"].GetLabels(); l["kubetre.io/workspace"] != "study" || l["kubetre.io/role"] != "vm-subnet" {
		t.Errorf("subnet labels = %v (VMs select their subnet by these)", l)
	}

	rules, _, _ := unstructured.NestedSlice(got["nsg"].Object, "spec", "forProvider", "securityRule")
	byRule := map[string]map[string]any{}
	for _, r := range rules {
		m := r.(map[string]any)
		byRule[m["name"].(string)] = m
	}
	if r := byRule["deny-virtual-network-out"]; r == nil || r["access"] != "Deny" || r["destinationAddressPrefix"] != "VirtualNetwork" {
		t.Errorf("cross-workspace traffic is not denied: %v", r)
	}
	if r := byRule["deny-all-in"]; r == nil || r["priority"] != float64(4096) {
		t.Errorf("inbound default deny missing: %v", r)
	}
	if r := byRule["allow-access-gateway-in"]; r == nil || len(r["sourceAddressPrefixes"].([]any)) == 0 {
		t.Errorf("access gateway rule = %v", r)
	}
	for name, r := range byRule {
		if r["access"] == "Allow" && r["direction"] == "Inbound" && r["sourceAddressPrefix"] == "*" {
			t.Errorf("rule %s allows inbound from anywhere", name)
		}
	}

	colls, _, _ := unstructured.NestedSlice(got["egress-rules"].Object, "spec", "forProvider", "applicationRuleCollection")
	rule := colls[0].(map[string]any)["rule"].([]any)[0].(map[string]any)
	if rule["destinationFqdns"].([]any)[1] != "*.r-project.org" || rule["sourceAddresses"].([]any)[0] != "10.240.0.64/26" {
		t.Errorf("firewall rule = %v", rule)
	}
	if p, _, _ := unstructured.NestedFloat64(got["egress-rules"].Object, "spec", "forProvider", "priority"); p != 1001 {
		t.Errorf("firewall priority = %v", p)
	}
	if status["subnetId"] != "" {
		t.Errorf("status before observation = %v", status)
	}

	// Observed subnet id flows to the composite's status.
	_, status = render(t, "composition-workspacenetwork.yaml", xr("WorkspaceNetwork", "vm-network", params), map[string]any{
		"subnet": map[string]any{"resource": map[string]any{"status": map[string]any{"atProvider": map[string]any{"id": "/subscriptions/x/subnets/snet-ws-study"}}}},
	})
	if status["subnetId"] != "/subscriptions/x/subnets/snet-ws-study" {
		t.Errorf("status = %v", status)
	}
}

func TestResearchVMComposition(t *testing.T) {
	for _, tc := range []struct {
		os, kind string
		image    map[string]any
		disk     float64
	}{
		{"windows", "WindowsVirtualMachine", map[string]any{"publisher": "MicrosoftWindowsServer", "offer": "WindowsServer", "sku": "2022-datacenter-azure-edition", "version": "latest"}, 128},
		{"linux", "LinuxVirtualMachine", map[string]any{"publisher": "Canonical", "offer": "ubuntu-24_04-lts", "sku": "server", "version": "latest"}, 64},
	} {
		t.Run(tc.os, func(t *testing.T) {
			params := map[string]any{"os": tc.os, "size": "medium", "diskGi": tc.disk, "image": tc.image,
				"adminUsername": "researcher", "passwordSecretName": "rita-vm-credentials", "owner": "rita@example.com"}
			composed, status := render(t, "composition-researchvm.yaml", xr("ResearchVM", "rita-vm", params), nil)
			got := submit(t, composed)
			if len(got) != 2 || got["nic"] == nil || got["vm"] == nil {
				t.Fatalf("composed = %v", got)
			}
			vm := got["vm"]
			if vm.GetKind() != tc.kind {
				t.Fatalf("kind = %s", vm.GetKind())
			}
			fp, _, _ := unstructured.NestedMap(vm.Object, "spec", "forProvider")
			if fp["size"] != "Standard_D4s_v5" || fp["secureBootEnabled"] != true || fp["vtpmEnabled"] != true ||
				fp["encryptionAtHostEnabled"] != true || fp["allowExtensionOperations"] != false {
				t.Errorf("hardening settings = %v", fp)
			}
			if _, ok := fp["identity"]; ok {
				t.Error("VM must not have a managed identity")
			}
			if cn := fp["computerName"].(string); len(cn) > 15 {
				t.Errorf("computerName %q exceeds 15 characters", cn)
			}
			ipc, _, _ := unstructured.NestedSlice(got["nic"].Object, "spec", "forProvider", "ipConfiguration")
			cfg := ipc[0].(map[string]any)
			if _, ok := cfg["publicIpAddressId"]; ok {
				t.Error("NIC must not have a public IP")
			}
			if sel := cfg["subnetIdSelector"].(map[string]any)["matchLabels"].(map[string]any); sel["kubetre.io/workspace"] != "study" {
				t.Errorf("subnet selector = %v", sel)
			}
			if _, ok := cfg["subnetIdSelector"].(map[string]any)["namespace"]; ok {
				t.Error("subnet selector must stay in the VM's own namespace")
			}
			if status["privateIp"] != "" || len(status["computerName"].(string)) == 0 {
				t.Errorf("status = %v", status)
			}
		})
	}
}

func TestResearchVMValidationRules(t *testing.T) {
	base := func() *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: xr("ResearchVM", "v", map[string]any{
			"os": "windows", "size": "medium", "diskGi": int64(128), "adminUsername": "researcher", "passwordSecretName": "s",
			"image": map[string]any{"publisher": "p", "offer": "o", "sku": "s", "version": "latest"},
		})}
	}
	if err := dryRunCreate(t, base()); err != nil {
		t.Fatalf("valid VM rejected: %v", err)
	}
	for name, mutate := range map[string]func(*unstructured.Unstructured){
		"small windows disk": func(u *unstructured.Unstructured) {
			_ = unstructured.SetNestedField(u.Object, int64(64), "spec", "parameters", "diskGi")
		},
		"reserved username": func(u *unstructured.Unstructured) {
			_ = unstructured.SetNestedField(u.Object, "administrator", "spec", "parameters", "adminUsername")
		},
		"unknown size": func(u *unstructured.Unstructured) {
			_ = unstructured.SetNestedField(u.Object, "huge", "spec", "parameters", "size")
		},
	} {
		u := base()
		mutate(u)
		if err := dryRunCreate(t, u); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
