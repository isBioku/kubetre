package tre

import (
	"context"
	"math"
	"testing"

	corev1 "k8s.io/api/core/v1"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
	"github.com/isBioku/kubetre/internal/controller"
	"github.com/isBioku/kubetre/internal/costs"
)

type fakeCosts struct{ queries []costs.Query }

// Month to date: £100 platform, £50 cluster nodes, £30 workspace resources of which £20 are
// tagged study1 (£12 its service, £5 rita's VM) and £10 are untagged OS disks.
func (f *fakeCosts) Costs(_ context.Context, q costs.Query) ([]costs.Row, error) {
	f.queries = append(f.queries, q)
	if !q.ByTag {
		return []costs.Row{
			{Cost: 100, Currency: "GBP", ResourceGroup: "rg-core"},
			{Cost: 50, Currency: "GBP", ResourceGroup: "mc-rg"},
			{Cost: 30, Currency: "GBP", ResourceGroup: "rg-ws"},
		}, nil
	}
	return []costs.Row{
		{Cost: 100, Currency: "GBP", ResourceGroup: "rg-core", TagKey: "kubetre-env", TagValue: "dev"},
		{Cost: 20, Currency: "GBP", ResourceGroup: "rg-ws", TagKey: "kubetre-workspace", TagValue: "study1"},
		{Cost: 12, Currency: "GBP", ResourceGroup: "rg-ws", TagKey: "kubetre-service", TagValue: "desktops"},
		{Cost: 5, Currency: "GBP", ResourceGroup: "rg-ws", TagKey: "kubetre-vm", TagValue: "rita-vm"},
		{Cost: 10, Currency: "GBP", ResourceGroup: "rg-ws"},
	}, nil
}

func costEnv(t *testing.T) (*env, *fakeCosts) {
	ns := controller.NamespacePrefix + "study1"
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}, Status: corev1.NodeStatus{Allocatable: corev1.ResourceList{
		corev1.ResourceCPU: k8sresource.MustParse("10"), corev1.ResourceMemory: k8sresource.MustParse("40Gi")}}}
	quota := &corev1.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: "kubetre-quota", Namespace: ns,
		Labels: map[string]string{controller.LabelWorkspace: "study1"}},
		Status: corev1.ResourceQuotaStatus{Used: corev1.ResourceList{
			corev1.ResourceRequestsCPU: k8sresource.MustParse("2"), corev1.ResourceRequestsMemory: k8sresource.MustParse("4Gi")}}}
	desktops := &treV1.WorkspaceService{ObjectMeta: metav1.ObjectMeta{Name: "desktops", Namespace: ns},
		Spec: treV1.WorkspaceServiceSpec{TemplateRef: "virtual-desktops", DisplayName: "Virtual Desktops"}}
	vm := &treV1.WorkspaceService{ObjectMeta: metav1.ObjectMeta{Name: "rita-vm", Namespace: ns},
		Spec: treV1.WorkspaceServiceSpec{TemplateRef: "windows-vm", DisplayName: "Rita's VM", ParentService: "desktops", Owner: rita}}
	e := newEnv(t, vmWorkspace("study1"), node, quota, desktops, vm)
	f := &fakeCosts{}
	e.server.Costs = f
	e.server.CostGroups = CostGroups{Core: "rg-core", Workspaces: "rg-ws", Cluster: "mc-rg"}
	return e, f
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.001 }

func firstCost(v any) float64 { return m(list(v)[0])["cost"].(float64) }

func TestTRECosts(t *testing.T) {
	e, f := costEnv(t)
	e.expect(403, "GET", "/api/costs", owner, "")
	got := e.expect(200, "GET", "/api/costs", admin, "")
	// study1 reserves (2/10 CPU + 4/40 memory) / 2 = 15% of the cluster: £7.50.
	ws := m(list(got["workspaces"])[0])
	if ws["id"] != "study1" || !near(firstCost(ws["costs"]), 27.5) {
		t.Fatalf("workspaces = %v", got["workspaces"])
	}
	// Core: £100 platform + £42.50 unreserved cluster + £10 untagged workspace resources.
	if c := firstCost(got["core_services"]); !near(c, 152.5) {
		t.Fatalf("core = %v", got["core_services"])
	}
	if m(list(got["core_services"])[0])["date"] != nil || m(list(got["core_services"])[0])["currency"] != "GBP" {
		t.Fatalf("core row = %v", list(got["core_services"])[0])
	}
	if len(f.queries) != 2 || f.queries[0].ByTag || !f.queries[1].ByTag {
		t.Fatalf("queries = %+v", f.queries)
	}
	e.expect(400, "GET", "/api/costs?from_date=2026-10-01", admin, "")
	e.expect(400, "GET", "/api/costs?from_date=2026-10-05&to_date=2026-10-01", admin, "")
	e.expect(400, "GET", "/api/costs?granularity=Hourly", admin, "")
	e.expect(200, "GET", "/api/costs?from_date=2026-09-01T00:00:00Z&to_date=2026-10-01T00:00:00Z&granularity=Daily", admin, "")
	if q := f.queries[len(f.queries)-1]; !q.Daily || q.From.Month() != 9 {
		t.Fatalf("query = %+v", q)
	}
}

func TestWorkspaceCosts(t *testing.T) {
	e, _ := costEnv(t)
	e.expect(403, "GET", "/api/workspaces/study1/costs", rita, "")
	e.expect(404, "GET", "/api/workspaces/study1/costs", nobody, "")
	got := e.expect(200, "GET", "/api/workspaces/study1/costs", owner, "")
	if got["id"] != "study1" || !near(firstCost(got["costs"]), 27.5) {
		t.Fatalf("workspace = %v", got)
	}
	svc := m(list(got["workspace_services"])[0])
	if svc["id"] != "desktops" || !near(firstCost(svc["costs"]), 12) {
		t.Fatalf("service = %v", svc)
	}
	ur := m(list(svc["user_resources"])[0])
	if ur["id"] != "rita-vm" || !near(firstCost(ur["costs"]), 5) {
		t.Fatalf("user resource = %v", ur)
	}
}

func TestCostsNotConfigured(t *testing.T) {
	e := newEnv(t, vmWorkspace("study1"))
	e.expect(404, "GET", "/api/costs", admin, "")
	e.expect(404, "GET", "/api/workspaces/study1/costs", owner, "")
}
