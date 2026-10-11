package tre

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
	"github.com/isBioku/kubetre/internal/airlock"
	"github.com/isBioku/kubetre/internal/controller"
)

const manager = "mia@example.com"

func airlockWorkspace(name string) *treV1.Workspace {
	ws := vmWorkspace(name)
	ws.Spec.AirlockManagers = []string{manager}
	ws.Spec.Airlock.Enabled = true
	return ws
}

// controllerMoves stands in for the airlock controller.
func (e *env) controllerMoves(ws, id, to string) {
	e.t.Helper()
	ar := &treV1.AirlockRequest{}
	if err := e.c.Get(context.Background(), types.NamespacedName{Namespace: controller.NamespacePrefix + ws, Name: id}, ar); err != nil {
		e.t.Fatal(err)
	}
	if err := airlock.Transition(ar, to, nil, ar.CreationTimestamp.Time); err != nil {
		e.t.Fatal(err)
	}
	if err := e.c.Status().Update(context.Background(), ar); err != nil {
		e.t.Fatal(err)
	}
}

func actions(v any) []string {
	out := []string{}
	for _, a := range v.([]any) {
		out = append(out, a.(string))
	}
	return out
}

func TestAirlockRequestLifecycle(t *testing.T) {
	e := newEnv(t, airlockWorkspace("study1"))
	base := "/api/workspaces/study1/requests"
	body := `{"type":"import","title":"Census extract","businessJustification":"Needed for analysis"}`

	e.expect(403, "POST", base, manager, body)
	e.expect(404, "POST", base, nobody, body)
	e.expect(403, "GET", base, admin, "")
	created := e.expect(201, "POST", base, rita, body)
	req := m(created["airlockRequest"])
	id := req["id"].(string)
	if req["status"] != "draft" || m(req["createdBy"])["id"] != rita || req["workspaceId"] != "study1" {
		t.Fatalf("request = %v", req)
	}
	if got := actions(created["allowedUserActions"]); strings.Join(got, ",") != "cancel,submit" {
		t.Fatalf("actions = %v", got)
	}
	if !e.store.Exists(airlock.External, id+"-draft") {
		t.Fatal("the draft container must exist before the response")
	}

	// Links: the requester writes the draft; the manager cannot see it yet.
	url := e.expect(200, "GET", base+"/"+id+"/link", rita, "")["containerUrl"].(string)
	if !strings.Contains(url, ".blob.core.windows.net/"+id+"-draft?") || !strings.Contains(url, "sp=rwdl") {
		t.Fatalf("draft link = %s", url)
	}
	e.expect(403, "GET", base+"/"+id+"/link", manager, "")

	e.expect(200, "POST", base+"/"+id+"/submit", sam, "")
	e.expect(400, "POST", base+"/"+id+"/submit", rita, "")
	e.controllerMoves("study1", id, treV1.AirlockInReview)

	// Review: managers read and decide; researchers cannot read during review.
	e.expect(403, "GET", base+"/"+id+"/link", rita, "")
	if url := e.expect(200, "GET", base+"/"+id+"/link", manager, "")["containerUrl"].(string); !strings.Contains(url, "sp=rl") {
		t.Fatalf("review link = %s", url)
	}
	got := e.expect(200, "GET", base+"/"+id, manager, "")
	if a := actions(got["allowedUserActions"]); strings.Join(a, ",") != "review" {
		t.Fatalf("manager actions in review = %v", a)
	}
	e.expect(403, "POST", base+"/"+id+"/review", rita, `{"approval":true,"decisionExplanation":"fine"}`)
	e.expect(422, "POST", base+"/"+id+"/review", manager, `{"decisionExplanation":"no decision"}`)
	reviewed := m(e.expect(200, "POST", base+"/"+id+"/review", manager, `{"approval":true,"decisionExplanation":"Aggregated data only"}`)["airlockRequest"])
	if reviewed["status"] != "approval_in_progress" {
		t.Fatalf("status = %v", reviewed["status"])
	}
	reviews := reviewed["reviews"].([]any)
	if len(reviews) != 1 || m(reviews[0])["reviewDecision"] != "approved" || m(m(reviews[0])["reviewer"])["id"] != manager {
		t.Fatalf("reviews = %v", reviews)
	}
	e.expect(400, "GET", base+"/"+id+"/link", rita, "")

	e.controllerMoves("study1", id, treV1.AirlockApproved)
	if url := e.expect(200, "GET", base+"/"+id+"/link", rita, "")["containerUrl"].(string); !strings.Contains(url, "internal.blob.core.windows.net/"+id+"?") {
		t.Fatalf("an approved import is downloaded inside the TRE: %s", url)
	}
	e.expect(422, "POST", base+"/"+id+"/revoke", manager, `{}`)
	revoked := m(e.expect(200, "POST", base+"/"+id+"/revoke", manager, `{"reason":"Contained identifiers"}`)["airlockRequest"])
	if revoked["status"] != "revoked" || len(revoked["history"].([]any)) < 4 {
		t.Fatalf("revoked = %v", revoked)
	}
	e.expect(400, "GET", base+"/"+id+"/link", rita, "")
}

func TestAirlockDisabledAndCancel(t *testing.T) {
	off := airlockWorkspace("closed")
	off.Spec.Airlock.Enabled = false
	e := newEnv(t, off, airlockWorkspace("study1"))
	e.expect(405, "POST", "/api/workspaces/closed/requests", rita, `{"type":"export","title":"x","businessJustification":"y"}`)
	e.expect(422, "POST", "/api/workspaces/study1/requests", rita, `{"type":"sideways","title":"x"}`)

	id := m(e.expect(201, "POST", "/api/workspaces/study1/requests", rita, `{"type":"export","title":"Results","businessJustification":"Paper"}`)["airlockRequest"])["id"].(string)
	if !e.store.Exists(airlock.Internal, id+"-draft") {
		t.Fatal("export drafts are written inside the TRE")
	}
	if s := m(e.expect(200, "POST", "/api/workspaces/study1/requests/"+id+"/cancel", owner, "")["airlockRequest"])["status"]; s != "cancelled" {
		t.Fatalf("status = %v", s)
	}
	e.expect(400, "POST", "/api/workspaces/study1/requests/"+id+"/cancel", owner, "")
}

func TestAirlockListsAndFilters(t *testing.T) {
	e := newEnv(t, airlockWorkspace("study1"))
	base := "/api/workspaces/study1/requests"
	for _, b := range []struct{ user, typ, title string }{{rita, "import", "b"}, {sam, "export", "a"}, {rita, "export", "c"}} {
		e.expect(201, "POST", base, b.user, `{"type":"`+b.typ+`","title":"`+b.title+`","businessJustification":"j"}`)
	}
	all := list(e.expect(200, "GET", base+"?order_by=title&order_ascending=true", manager, "")["airlockRequests"])
	if len(all) != 3 || m(m(all[0])["airlockRequest"])["title"] != "a" {
		t.Fatalf("ordered list = %v", all)
	}
	if got := list(e.expect(200, "GET", base+"?type=export&creator_user_id="+rita, sam, "")["airlockRequests"]); len(got) != 1 {
		t.Fatalf("filtered = %d", len(got))
	}
	if got := list(e.expect(200, "GET", base+"?status=blocked", sam, "")["airlockRequests"]); len(got) != 0 {
		t.Fatalf("the UI's blocked filter must work, got %d", len(got))
	}

	// /api/requests: your own requests, or as a manager, your workspaces' requests.
	_, mine := e.call("GET", "/api/requests", rita, "")
	_ = mine
	req := e.expectArray("GET", "/api/requests", rita)
	if len(req) != 2 {
		t.Fatalf("rita's own requests = %d", len(req))
	}
	if got := e.expectArray("GET", "/api/requests?airlock_manager=true", manager); len(got) != 3 {
		t.Fatalf("manager sees %d", len(got))
	}
	if got := e.expectArray("GET", "/api/requests?airlock_manager=true", rita); len(got) != 0 {
		t.Fatalf("non-managers see none as manager, got %d", len(got))
	}
}

func TestAirlockReviewVM(t *testing.T) {
	ws := airlockWorkspace("study1")
	e := newEnv(t, ws)
	base := "/api/workspaces/study1/requests"
	id := m(e.expect(201, "POST", base, rita, `{"type":"export","title":"Results","businessJustification":"j"}`)["airlockRequest"])["id"].(string)
	e.expect(200, "POST", base+"/"+id+"/submit", rita, "")
	e.expect(400, "POST", base+"/"+id+"/review-user-resource", manager, "")
	e.controllerMoves("study1", id, treV1.AirlockInReview)
	e.expect(422, "POST", base+"/"+id+"/review-user-resource", manager, "")

	// Configure export review VMs under a deployed Virtual Desktops service.
	svcName := m(e.expect(202, "POST", "/api/workspaces/study1/workspace-services", owner,
		`{"templateName":"virtual-desktops","properties":{"display_name":"Virtual Desktops","description":"d"}}`)["operation"])["resourceId"].(string)
	ns := controller.NamespacePrefix + "study1"
	e.setPhase(&treV1.WorkspaceService{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: svcName}}, treV1.PhaseReady)
	cur := &treV1.Workspace{}
	if err := e.c.Get(context.Background(), types.NamespacedName{Name: "study1"}, cur); err != nil {
		t.Fatal(err)
	}
	e.expect(202, "PATCH", "/api/workspaces/study1", admin,
		`{"properties":{"airlock_review_config":{"export":{"export_vm_workspace_service_id":"`+svcName+`","export_vm_user_resource_template_name":"windows-vm"}}}}`,
		"etag", cur.ResourceVersion)

	e.expect(403, "POST", base+"/"+id+"/review-user-resource", rita, "")
	out := e.expect(202, "POST", base+"/"+id+"/review-user-resource", manager, "")
	op := m(out["operation"])
	vmName := op["resourceId"].(string)
	rr := m(m(out["airlockRequest"])["reviewUserResources"])[manager]
	if rr == nil || m(rr)["userResourceId"] != vmName || m(rr)["workspaceServiceId"] != svcName {
		t.Fatalf("reviewUserResources = %v", m(out["airlockRequest"])["reviewUserResources"])
	}
	vm := &treV1.WorkspaceService{}
	if err := e.c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: vmName}, vm); err != nil {
		t.Fatal(err)
	}
	values := rawMap(vm.Spec.Values.Raw)
	if u, _ := values["airlockRequestUrl"].(string); !strings.Contains(u, "internal.blob.core.windows.net/"+id+"?") || !strings.Contains(u, "sp=rl") {
		t.Fatalf("the review VM must get a read-only link to the sealed data, got %q", u)
	}
	if vm.Spec.Owner != manager || values["username"] != "mia" {
		t.Fatalf("owner %s, values %v", vm.Spec.Owner, values)
	}
	props := m(m(e.expect(200, "GET", "/api/workspaces/study1/workspace-services/"+svcName+"/user-resources/"+vmName, manager, "")["userResource"])["properties"])
	if _, leaked := props["airlockRequestUrl"]; leaked {
		t.Fatal("the review link must not be shown as a property")
	}

	// A healthy review VM blocks a second one.
	e.setPhase(vm, treV1.PhaseReady)
	e.expect(409, "POST", base+"/"+id+"/review-user-resource", manager, "")

	// Reviewing removes the review VMs.
	e.expect(200, "POST", base+"/"+id+"/review", manager, `{"approval":false,"decisionExplanation":"Too detailed"}`)
	if err := e.c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: vmName}, vm); err == nil {
		t.Fatal("review VMs must be removed once the request is reviewed")
	}
}
