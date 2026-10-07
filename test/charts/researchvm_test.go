package charts

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/yaml"

	"github.com/isBioku/kubetre/test/xrdcrd"
)

func TestResearchVMChartProducesValidRequests(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("KUBEBUILDER_ASSETS not set; run `make test`")
	}
	env := &envtest.Environment{CRDs: []*apiextensionsv1.CustomResourceDefinition{
		xrdcrd.FromFile(filepath.Join("..", "..", "crossplane", "xrd-researchvm.yaml")),
	}}
	cfg, err := env.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = env.Stop() }()
	c, _ := client.New(cfg, client.Options{})
	ctx := context.Background()
	if err := c.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ws-chk"}}); err != nil {
		t.Fatal(err)
	}

	cases := map[string][]string{
		"windows": {"os=windows", "diskGi=128", "image.publisher=MicrosoftWindowsServer", "image.offer=WindowsServer", "image.sku=2022-datacenter-azure-edition"},
		"linux":   {"os=linux", "size=large"},
	}
	for name, sets := range cases {
		t.Run(name, func(t *testing.T) {
			sets = append(sets, "kubetre.workspace=chk", "kubetre.owner=rita@example.com")
			var secret corev1.Secret
			var vm *unstructured.Unstructured
			for _, d := range render(t, "research-vm", sets...) {
				switch {
				case strings.Contains(string(d), "kind: Secret"):
					if err := yaml.UnmarshalStrict(d, &secret); err != nil {
						t.Fatal(err)
					}
				case strings.Contains(string(d), "kind: ResearchVM"):
					vm = &unstructured.Unstructured{}
					if err := yaml.Unmarshal(d, &vm.Object); err != nil {
						t.Fatal(err)
					}
				}
			}
			if vm == nil || secret.Name == "" {
				t.Fatal("chart must render a Secret and a ResearchVM")
			}
			vm.SetNamespace("ws-chk")
			if err := c.Create(ctx, vm, client.DryRunAll, client.FieldValidation("Strict")); err != nil {
				t.Fatalf("ResearchVM rejected by the XRD schema: %v", err)
			}
			ref, _, _ := unstructured.NestedString(vm.Object, "spec", "parameters", "passwordSecretName")
			if ref != secret.Name {
				t.Fatalf("VM references secret %q but chart creates %q", ref, secret.Name)
			}
			pw := secret.StringData["password"]
			for _, class := range []string{`[A-Z]`, `[a-z]`, `[0-9]`, `[^A-Za-z0-9]`} {
				if !regexp.MustCompile(class).MatchString(pw) {
					t.Fatalf("password lacks character class %s", class)
				}
			}
			if len(pw) < 12 || len(pw) > 72 {
				t.Fatalf("password length %d outside Azure limits", len(pw))
			}
		})
	}
}
