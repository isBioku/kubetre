package gateway

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
	"github.com/isBioku/kubetre/internal/access"
)

var (
	rita  = access.Identity{Subject: "sub-rita", Email: "Rita@Example.com"}
	ravi  = access.Identity{Subject: "sub-ravi", Email: "ravi@example.com"}
	olive = access.Identity{Subject: "sub-olive", Email: "olive@example.com"}
	eve   = access.Identity{Subject: "sub-eve", Email: "eve@example.com"}
)

func workspace(name, prefix string, researchers ...string) *treV1.Workspace {
	ws := &treV1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       treV1.WorkspaceSpec{DisplayName: name, TemplateRef: "vm", Owners: []string{"olive@example.com"}, Researchers: researchers},
		Status:     treV1.WorkspaceStatus{Phase: treV1.PhaseReady},
	}
	if prefix != "" {
		ws.Status.VMNetwork = &treV1.VMNetworkStatus{AddressPrefix: prefix, FirewallPriority: 1000}
	}
	return ws
}

func service(ns, name, owner string) *treV1.WorkspaceService {
	return &treV1.WorkspaceService{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       treV1.WorkspaceServiceSpec{TemplateRef: "t", DisplayName: name + " display", Owner: owner},
	}
}

// connSecret builds a connection Secret as the chart of release svc would, with Helm's
// release annotation.
func connSecret(ns, name, svc string, data map[string]string, vm string) *corev1.Secret {
	return connSecretFrom(ns, name, svc, svc, data, vm)
}

func connSecretFrom(ns, name, labelledService, helmRelease string, data map[string]string, vm string) *corev1.Secret {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: ns,
			Labels:      map[string]string{LabelConnection: "true", LabelService: labelledService},
			Annotations: map[string]string{AnnotationHelmRelease: helmRelease},
		},
		Data: map[string][]byte{},
	}
	for k, v := range data {
		s.Data[k] = []byte(v)
	}
	if vm != "" {
		s.Annotations[AnnotationHostFromVM] = vm
	}
	return s
}

func researchVM(ns, name, ip string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(researchVMGVK)
	u.SetNamespace(ns)
	u.SetName(name)
	if ip != "" {
		_ = unstructured.SetNestedField(u.Object, ip, "status", "privateIp")
	}
	return u
}

func fixture(t *testing.T) *Resolver {
	t.Helper()
	scheme := runtime.NewScheme()
	_ = treV1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	rdp := map[string]string{"protocol": "rdp", "port": "3389", "username": "researcher", "password": "pw"}
	vnc := func(host string) map[string]string {
		return map[string]string{"protocol": "vnc", "port": "5901", "password": "pw", "hostname": host}
	}
	objs := []client.Object{
		workspace("study", "10.240.0.64/26", "rita@example.com", "ravi@example.com"),
		workspace("other", "10.240.0.128/26", "ravi@example.com"),
		service("ws-study", "rita-vm", "rita@example.com"),
		service("ws-study", "rita-desktop", "rita@example.com"),
		service("ws-study", "ravi-vm", "ravi@example.com"),
		service("ws-study", "git", ""), // shared service: never exposed through the gateway
		service("ws-study", "rita-evil", "rita@example.com"),
		service("ws-study", "rita-pending", "rita@example.com"),
		service("ws-study", "rita-stray-ip", "rita@example.com"),
		service("ws-other", "ravi-other-vm", "ravi@example.com"),
		connSecret("ws-study", "rita-vm-credentials", "rita-vm", rdp, "rita-vm"),
		connSecret("ws-study", "rita-desktop-vnc", "rita-desktop", vnc("rita-desktop.ws-study.svc.cluster.local"), ""),
		connSecret("ws-study", "ravi-vm-credentials", "ravi-vm", rdp, "ravi-vm"),
		connSecret("ws-study", "git-creds", "git", vnc("git.ws-study.svc.cluster.local"), ""),
		// Points at another workspace's Service: must be refused.
		connSecret("ws-study", "rita-evil-vnc", "rita-evil", vnc("victim.ws-other.svc.cluster.local"), ""),
		connSecret("ws-study", "rita-pending-credentials", "rita-pending", rdp, "rita-pending"),
		// VM reports an address outside this workspace's subnet: must be refused.
		connSecret("ws-study", "rita-stray-ip-credentials", "rita-stray-ip", rdp, "rita-stray-ip"),
		// Installed by Rita's release but labelled with Ravi's service: a forged entry.
		connSecretFrom("ws-study", "forged", "ravi-vm", "rita-evil", vnc("rita-evil.ws-study.svc.cluster.local"), ""),
		// No Helm release annotation at all: not created by a KubeTRE service.
		connSecretFrom("ws-study", "unmanaged", "rita-vm", "", rdp, "rita-vm"),
		connSecret("ws-other", "ravi-other-credentials", "ravi-other-vm", rdp, "ravi-other-vm"),
		researchVM("ws-study", "rita-vm", "10.240.0.70"),
		researchVM("ws-study", "ravi-vm", "10.240.0.71"),
		researchVM("ws-study", "rita-pending", ""),
		researchVM("ws-study", "rita-stray-ip", "10.240.0.130"),
		researchVM("ws-other", "ravi-other-vm", "10.240.0.130"),
	}
	return &Resolver{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()}
}

func byID(t *testing.T, r *Resolver, id access.Identity) map[string]Connection {
	t.Helper()
	conns, err := r.Connections(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]Connection{}
	for _, c := range conns {
		out[c.ID()] = c
	}
	return out
}

func TestUsersSeeOnlyTheirOwnSessions(t *testing.T) {
	r := fixture(t)
	got := byID(t, r, rita)
	want := []string{"study/rita-vm-credentials", "study/rita-desktop-vnc", "study/rita-evil-vnc", "study/rita-pending-credentials", "study/rita-stray-ip-credentials"}
	if len(got) != len(want) {
		t.Fatalf("rita sees %v", keys(got))
	}
	for _, w := range want {
		if _, ok := got[w]; !ok {
			t.Fatalf("rita is missing %s; sees %v", w, keys(got))
		}
	}
	if _, ok := got["study/forged"]; ok {
		t.Fatal("a secret labelled with someone else's service leaked to rita")
	}

	if _, ok := got["study/unmanaged"]; ok {
		t.Fatal("a secret not installed by a KubeTRE Helm release was trusted")
	}

	if got := byID(t, r, ravi); len(got) != 2 || got["study/ravi-vm-credentials"].Hostname != "10.240.0.71" || got["study/forged"].Name != "" {
		t.Fatalf("ravi sees %v", keys(got))
	}
	// Workspace owners do not get researchers' personal machines, and outsiders get nothing.
	if got := byID(t, r, olive); len(got) != 0 {
		t.Fatalf("owner sees %v", keys(got))
	}
	if got := byID(t, r, eve); len(got) != 0 {
		t.Fatalf("outsider sees %v", keys(got))
	}
}

func TestTargetsAreConfinedToTheWorkspace(t *testing.T) {
	got := byID(t, fixture(t), rita)
	for id, wantReady := range map[string]bool{
		"study/rita-vm-credentials":       true,
		"study/rita-desktop-vnc":          true,
		"study/rita-evil-vnc":             false,
		"study/rita-pending-credentials":  false,
		"study/rita-stray-ip-credentials": false,
	} {
		if got[id].Ready != wantReady {
			t.Errorf("%s ready = %v (%s), want %v", id, got[id].Ready, got[id].Reason, wantReady)
		}
	}
	if got["study/rita-vm-credentials"].Hostname != "10.240.0.70" || got["study/rita-vm-credentials"].Port != 3389 {
		t.Errorf("vm target = %+v", got["study/rita-vm-credentials"])
	}
	if got["study/rita-evil-vnc"].Hostname != "" {
		t.Error("a refused target must not carry a hostname")
	}
}

func TestHardenedParametersCloseDataChannels(t *testing.T) {
	for _, proto := range []string{"rdp", "ssh", "vnc"} {
		p := HardenedParameters(Connection{Protocol: proto, Hostname: "h", Port: 1})
		for k, v := range map[string]string{"disable-copy": "true", "disable-paste": "true", "enable-sftp": "false"} {
			if p[k] != v {
				t.Errorf("%s: %s = %q", proto, k, p[k])
			}
		}
	}
	p := HardenedParameters(Connection{Protocol: "rdp", Hostname: "h", Port: 1})
	for k, v := range map[string]string{"enable-drive": "false", "disable-download": "true", "disable-upload": "true", "enable-printing": "false"} {
		if p[k] != v {
			t.Errorf("rdp: %s = %q", k, p[k])
		}
	}
}

func keys(m map[string]Connection) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
