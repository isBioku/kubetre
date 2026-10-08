package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
)

const desktopSchema = `{"type":"object","properties":{"size":{"type":"string","enum":["small","large"]}},"additionalProperties":false}`

func serviceFixture(t *testing.T) *httptest.Server {
	ws := existing("study", []string{"olive@example.com"}, []string{"rita@example.com", "ravi@example.com"})
	ws.Status.Phase = treV1.PhaseReady
	pending := existing("pending", []string{"olive@example.com"}, []string{"rita@example.com"})
	desktop := &treV1.ServiceTemplate{ObjectMeta: metav1.ObjectMeta{Name: "linux-desktop"}, Spec: treV1.ServiceTemplateSpec{
		DisplayName: "Linux desktop", PerUser: true,
		Chart:        &treV1.ChartRef{URL: "oci://x.azurecr.io/charts/linux-desktop", Version: "0.1.0"},
		ValuesSchema: &apiextensionsv1.JSON{Raw: []byte(desktopSchema)},
	}}
	gitea := &treV1.ServiceTemplate{ObjectMeta: metav1.ObjectMeta{Name: "gitea"}, Spec: treV1.ServiceTemplateSpec{
		DisplayName: "Gitea", Chart: &treV1.ChartRef{URL: "oci://x.azurecr.io/charts/gitea", Version: "1.0.0"},
	}}
	return newTestServer(t, ws, pending, desktop, gitea)
}

func TestResearcherCreatesOwnDesktop(t *testing.T) {
	srv := serviceFixture(t)
	code, body := call(t, srv, "POST", "/api/v1/workspaces/study/services", "rita@example.com", "TREUser",
		`{"name":"rita-desktop","templateRef":"linux-desktop","displayName":"Rita","values":{"size":"large"}}`)
	if code != http.StatusAccepted || body["owner"] != "rita@example.com" {
		t.Fatalf("code = %d body = %v", code, body)
	}
}

func TestServiceValuesValidated(t *testing.T) {
	srv := serviceFixture(t)
	for name, values := range map[string]string{"enum": `{"size":"huge"}`, "extra": `{"image":"evil"}`, "not object": `[1]`} {
		t.Run(name, func(t *testing.T) {
			code, _ := call(t, srv, "POST", "/api/v1/workspaces/study/services", "rita@example.com", "TREUser",
				`{"name":"d","templateRef":"linux-desktop","displayName":"D","values":`+values+`}`)
			if code != http.StatusUnprocessableEntity {
				t.Fatalf("code = %d", code)
			}
		})
	}
}

func TestOnlyOwnersCreateSharedServices(t *testing.T) {
	srv := serviceFixture(t)
	body := `{"name":"git","templateRef":"gitea","displayName":"Git"}`
	if code, _ := call(t, srv, "POST", "/api/v1/workspaces/study/services", "rita@example.com", "TREUser", body); code != http.StatusForbidden {
		t.Fatalf("researcher code = %d", code)
	}
	code, resp := call(t, srv, "POST", "/api/v1/workspaces/study/services", "olive@example.com", "TREUser", body)
	if code != http.StatusAccepted || resp["owner"] != nil {
		t.Fatalf("owner code = %d body = %v", code, resp)
	}
}

func TestNonMembersAndUnreadyWorkspaces(t *testing.T) {
	srv := serviceFixture(t)
	body := `{"name":"d","templateRef":"linux-desktop","displayName":"D"}`
	if code, _ := call(t, srv, "POST", "/api/v1/workspaces/study/services", "eve@example.com", "TREUser", body); code != http.StatusNotFound {
		t.Fatalf("outsider code = %d", code)
	}
	if code, _ := call(t, srv, "POST", "/api/v1/workspaces/pending/services", "rita@example.com", "TREUser", body); code != http.StatusConflict {
		t.Fatalf("unready code = %d", code)
	}
}

func TestPersonalServicesArePrivate(t *testing.T) {
	srv := serviceFixture(t)
	for _, u := range []string{"rita", "ravi"} {
		if code, _ := call(t, srv, "POST", "/api/v1/workspaces/study/services", u+"@example.com", "TREUser",
			`{"name":"`+u+`-desktop","templateRef":"linux-desktop","displayName":"D"}`); code != http.StatusAccepted {
			t.Fatalf("create for %s = %d", u, code)
		}
	}
	call(t, srv, "POST", "/api/v1/workspaces/study/services", "olive@example.com", "TREUser", `{"name":"git","templateRef":"gitea","displayName":"Git"}`)

	names := func(user string) map[string]bool {
		_, body := call(t, srv, "GET", "/api/v1/workspaces/study/services", user, "TREUser", "")
		out := map[string]bool{}
		for _, s := range body["services"].([]any) {
			out[s.(map[string]any)["name"].(string)] = true
		}
		return out
	}
	if got := names("rita@example.com"); len(got) != 2 || !got["rita-desktop"] || !got["git"] {
		t.Fatalf("rita sees %v", got)
	}
	if got := names("olive@example.com"); len(got) != 3 {
		t.Fatalf("owner sees %v", got)
	}

	if code, _ := call(t, srv, "GET", "/api/v1/workspaces/study/services/ravi-desktop", "rita@example.com", "TREUser", ""); code != http.StatusNotFound {
		t.Fatalf("rita reading ravi's desktop = %d", code)
	}
	if code, _ := call(t, srv, "DELETE", "/api/v1/workspaces/study/services/ravi-desktop", "rita@example.com", "TREUser", ""); code != http.StatusNotFound {
		t.Fatalf("rita deleting ravi's desktop = %d", code)
	}
	if code, _ := call(t, srv, "DELETE", "/api/v1/workspaces/study/services/git", "rita@example.com", "TREUser", ""); code != http.StatusForbidden {
		t.Fatalf("researcher deleting shared service = %d", code)
	}
	if code, _ := call(t, srv, "DELETE", "/api/v1/workspaces/study/services/rita-desktop", "rita@example.com", "TREUser", ""); code != http.StatusAccepted {
		t.Fatalf("rita deleting own desktop = %d", code)
	}
}
