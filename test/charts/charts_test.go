// Package charts checks that rendered KubeTRE charts are admitted by a real API server
// under the Pod Security level their ServiceTemplate declares.
package charts

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/yaml"
)

func helmBinary(t *testing.T) string {
	if p, err := filepath.Abs(filepath.Join("..", "..", "bin", "helm")); err == nil {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if p, err := exec.LookPath("helm"); err == nil {
		return p
	}
	t.Skip("helm not found")
	return ""
}

func render(t *testing.T, chart string, sets ...string) [][]byte {
	t.Helper()
	args := []string{"template", "test", filepath.Join("..", "..", "charts", chart), "--namespace", "ws-chk"}
	for _, s := range sets {
		args = append(args, "--set", s)
	}
	out, err := exec.Command(helmBinary(t), args...).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	var docs [][]byte
	for _, d := range strings.Split("\n"+string(out), "\n---") {
		if d = strings.TrimSpace(d); d != "" {
			docs = append(docs, []byte(d))
		}
	}
	return docs
}

func TestLinuxDesktopPassesRestrictedPodSecurity(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("KUBEBUILDER_ASSETS not set; run `make test`")
	}
	env := &envtest.Environment{}
	cfg, err := env.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = env.Stop() }()
	cs := kubernetes.NewForConfigOrDie(cfg)
	ctx := context.Background()

	_, err = cs.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name:   "ws-chk",
		Labels: map[string]string{"pod-security.kubernetes.io/enforce": "restricted"},
	}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	var pod *corev1.Pod
	for _, d := range render(t, "linux-desktop", "kubetre.workspace=chk", "kubetre.owner=rita@example.com") {
		if !strings.Contains(string(d), "kind: Deployment") {
			continue
		}
		dep := &appsv1.Deployment{}
		if err := yaml.UnmarshalStrict(d, dep); err != nil {
			t.Fatalf("deployment does not decode strictly: %v", err)
		}
		pod = &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "desktop-check"}, Spec: dep.Spec.Template.Spec}
	}
	if pod == nil {
		t.Fatal("no Deployment rendered")
	}
	if _, err := cs.CoreV1().Pods("ws-chk").Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		t.Fatalf("desktop pod rejected under restricted Pod Security: %v", err)
	}

	// Control: the same namespace rejects a pod that breaks the profile.
	bad := pod.DeepCopy()
	bad.Name = "bad"
	priv := true
	bad.Spec.Containers[0].SecurityContext.Privileged = &priv
	if _, err := cs.CoreV1().Pods("ws-chk").Create(ctx, bad, metav1.CreateOptions{}); err == nil {
		t.Fatal("privileged pod was admitted; Pod Security admission is not active")
	}
}

// Users may send no values at all; the API does not apply schema defaults. Every registered
// template must therefore render its chart with only the administrator's values.
func TestTemplatesRenderWithAdminValuesOnly(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("..", "..", "config", "templates", "servicetemplate-*.yaml"))
	more, _ := filepath.Glob(filepath.Join("..", "..", "config", "kubevirt", "servicetemplate-*.yaml"))
	files = append(files, more...)
	if len(files) == 0 {
		t.Fatal("no service templates found")
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var tmpl struct {
			Metadata struct{ Name string }
			Spec     struct {
				Chart  *struct{ URL string }
				Values map[string]any
			}
		}
		if err := yaml.Unmarshal(raw, &tmpl); err != nil {
			t.Fatal(err)
		}
		if tmpl.Spec.Chart == nil {
			continue // grouping services install nothing
		}
		chart := tmpl.Spec.Chart.URL[strings.LastIndex(tmpl.Spec.Chart.URL, "/")+1:]
		valuesFile := filepath.Join(t.TempDir(), "values.yaml")
		out, _ := yaml.Marshal(tmpl.Spec.Values)
		if err := os.WriteFile(valuesFile, out, 0o600); err != nil {
			t.Fatal(err)
		}
		t.Run(tmpl.Metadata.Name, func(t *testing.T) {
			cmd := exec.Command(helmBinary(t), "template", "x", filepath.Join("..", "..", "charts", chart),
				"-f", valuesFile, "--set", "kubetre.workspace=chk", "--set", "kubetre.owner=rita@example.com")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("chart %s does not render with %s's values: %v\n%s", chart, tmpl.Metadata.Name, err, out)
			}
		})
	}
}

// A KubeVirt VM: an owner-named account with password SSH, reached only through its own
// Service in the workspace, on the KubeVirt pool, with a persistent disk copied from ACR.
func TestKubeVirtVMChart(t *testing.T) {
	docs := render(t, "kubevirt-vm", "username=rita", "size=small", "diskGi=40",
		"image=acr.example.io/containerdisks/ubuntu:24.04", "kubetre.owner=rita@example.com")
	byKind := map[string][]map[string]any{}
	for _, d := range docs {
		m := map[string]any{}
		if err := yaml.Unmarshal(d, &m); err != nil {
			t.Fatal(err)
		}
		k, _ := m["kind"].(string)
		byKind[k] = append(byKind[k], m)
	}
	if len(byKind["VirtualMachine"]) != 1 || len(byKind["Service"]) != 1 || len(byKind["Secret"]) != 2 {
		t.Fatalf("unexpected objects: %v", byKind)
	}
	vm := byKind["VirtualMachine"][0]
	get := func(m any, path ...any) any {
		for _, p := range path {
			switch k := p.(type) {
			case string:
				m = m.(map[string]any)[k]
			case int:
				m = m.([]any)[k]
			}
		}
		return m
	}
	spec := get(vm, "spec", "template", "spec")
	if get(spec, "nodeSelector", "kubetre.io/node-pool") != "kubevirt" {
		t.Error("the VM must run on the kubevirt pool")
	}
	if get(spec, "domain", "cpu", "cores") != float64(1) || get(spec, "domain", "memory", "guest") != "4Gi" {
		t.Errorf("size small: %v", get(spec, "domain"))
	}
	if get(spec, "domain", "resources", "limits", "memory") != "4Gi" || get(spec, "domain", "resources", "limits", "cpu") != "1" {
		t.Errorf("the VM must set its own limits, or the workspace LimitRange default applies: %v", get(spec, "domain", "resources"))
	}
	iface := get(spec, "domain", "devices", "interfaces", 0).(map[string]any)
	if _, ok := iface["masquerade"]; !ok || len(iface["ports"].([]any)) != 1 {
		t.Errorf("the VM must expose only SSH through masquerade: %v", iface)
	}
	dv := get(vm, "spec", "dataVolumeTemplates", 0, "spec")
	if get(dv, "source", "registry", "url") != "docker://acr.example.io/containerdisks/ubuntu:24.04" ||
		get(dv, "source", "registry", "pullMethod") != "node" || get(dv, "storage", "volumeMode") != "Filesystem" || get(dv, "storage", "resources", "requests", "storage") != "40Gi" {
		t.Errorf("disk: %v", dv)
	}

	var conn, cloud map[string]any
	for _, s := range byKind["Secret"] {
		if get(s, "metadata", "labels").(map[string]any)["kubetre.io/connection"] == "true" {
			conn = s
		} else {
			cloud = s
		}
	}
	data := conn["stringData"].(map[string]any)
	if data["protocol"] != "ssh" || data["port"] != "22" || data["username"] != "rita" ||
		data["hostname"] != "test-ssh.ws-chk.svc.cluster.local" {
		t.Errorf("connection: %v", data)
	}
	ud := get(cloud, "stringData", "userdata").(string)
	for _, want := range []string{"#cloud-config", "disable_root: true", "- name: rita", data["password"].(string)} {
		if !strings.Contains(ud, want) {
			t.Errorf("cloud-init is missing %q", want)
		}
	}
	if sel := get(byKind["Service"][0], "spec", "selector", "kubetre.io/vm"); sel != "test" {
		t.Errorf("service selector = %v", sel)
	}
}
