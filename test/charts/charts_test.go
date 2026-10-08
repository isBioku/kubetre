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
