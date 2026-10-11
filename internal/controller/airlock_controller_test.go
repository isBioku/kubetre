package controller

import (
	"context"
	"strings"
	"sync"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
	"github.com/isBioku/kubetre/internal/airlock"
)

var (
	airlockStore = airlock.NewMemoryStore()
	airlockMail  = &safeRecorder{}
)

type safeRecorder struct {
	mu   sync.Mutex
	sent []airlock.Message
}

func (s *safeRecorder) Send(_ context.Context, m airlock.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, m)
	return nil
}

func (s *safeRecorder) to(subjectPart string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.sent {
		if strings.Contains(m.Subject, subjectPart) {
			return m.To
		}
	}
	return nil
}

func newAirlockRequest(t *testing.T, ns, name, typ string) {
	t.Helper()
	ctx := context.Background()
	ar := &treV1.AirlockRequest{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: treV1.AirlockRequestSpec{Type: typ, Title: name, BusinessJustification: "analysis",
			CreatedBy: treV1.AirlockUser{ID: "oid-rita", Name: "Rita", Email: "rita@example.com"}},
	}
	must(t, k8s.Create(ctx, ar))
	must(t, retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cur := getAirlock(t, ns, name)
		cur.Status.Status = treV1.AirlockDraft
		return k8s.Status().Update(ctx, cur)
	}))
}

func getAirlock(t *testing.T, ns, name string) *treV1.AirlockRequest {
	t.Helper()
	ar := &treV1.AirlockRequest{}
	must(t, k8s.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: name}, ar))
	return ar
}

func setStatus(t *testing.T, ns, name, to string) {
	t.Helper()
	must(t, retry.RetryOnConflict(retry.DefaultRetry, func() error {
		ar := getAirlock(t, ns, name)
		if err := airlock.Transition(ar, to, &treV1.AirlockUser{ID: "oid-actor"}, ar.CreationTimestamp.Time); err != nil {
			return err
		}
		return k8s.Status().Update(context.Background(), ar)
	}))
}

func TestAirlockImportJourney(t *testing.T) {
	ns := readyWorkspace(t, "airlock-a", treV1.PodSecurityRestricted)
	ws := getWS(t, "airlock-a")
	ws.Spec.AirlockManagers = []string{"manager@example.com"}
	ws.Spec.Researchers = []string{"rita@example.com", "sam@example.com"}
	must(t, k8s.Update(context.Background(), ws))

	newAirlockRequest(t, ns, "imp1", treV1.AirlockImport)
	eventually(t, "draft container created", func() bool { return airlockStore.Exists(airlock.External, "imp1-draft") })

	// The researcher uploads one file; Defender has already judged its sealed copy clean.
	airlockStore.Put(airlock.External, "imp1-draft", "data.csv", []byte("a,b\n1,2\n"))
	airlockStore.SetVerdict(airlock.External, "imp1", "data.csv", airlock.ScanClean)
	setStatus(t, ns, "imp1", treV1.AirlockSubmitted)
	eventually(t, "in review", func() bool { return getAirlock(t, ns, "imp1").Status.Status == treV1.AirlockInReview })
	ar := getAirlock(t, ns, "imp1")
	if len(ar.Status.Files) != 1 || ar.Status.Files[0].Name != "data.csv" || ar.Status.Files[0].Size != 8 {
		t.Fatalf("files = %+v", ar.Status.Files)
	}
	if ar.Status.ScanResult == nil || !ar.Status.ScanResult.Clean {
		t.Fatalf("scan result = %+v", ar.Status.ScanResult)
	}
	if airlockStore.Exists(airlock.External, "imp1-draft") || !airlockStore.Exists(airlock.External, "imp1") {
		t.Fatal("submission must seal the file and delete the draft, so draft links stop working")
	}
	eventually(t, "managers told", func() bool {
		to := airlockMail.to(`"imp1" needs your review`)
		return len(to) == 1 && to[0] == "manager@example.com"
	})

	setStatus(t, ns, "imp1", treV1.AirlockApprovalInProgress)
	eventually(t, "approved", func() bool { return getAirlock(t, ns, "imp1").Status.Status == treV1.AirlockApproved })
	if files, err := airlockStore.ListFiles(context.Background(), airlock.Internal, "imp1"); err != nil || len(files) != 1 {
		t.Fatalf("approved import must be inside the TRE: %v %v", files, err)
	}
	if airlockStore.Exists(airlock.External, "imp1") {
		t.Fatal("the external copy must be removed after approval")
	}
	eventually(t, "researchers told", func() bool { return len(airlockMail.to(`"imp1" was approved`)) == 2 })

	// History records every update, and the last person to act stays as updatedBy.
	ar = getAirlock(t, ns, "imp1")
	if ar.Status.UpdatedBy.ID != "oid-actor" || len(ar.Status.History) < 4 {
		t.Fatalf("updatedBy %v, %d history items", ar.Status.UpdatedBy, len(ar.Status.History))
	}
}

func TestAirlockScanGatesReview(t *testing.T) {
	ns := readyWorkspace(t, "airlock-b", treV1.PodSecurityRestricted)
	newAirlockRequest(t, ns, "imp2", treV1.AirlockImport)
	eventually(t, "draft container", func() bool { return airlockStore.Exists(airlock.External, "imp2-draft") })
	airlockStore.Put(airlock.External, "imp2-draft", "payload.exe", []byte("MZ"))
	setStatus(t, ns, "imp2", treV1.AirlockSubmitted)

	// Not scanned yet: the request waits, sealed.
	eventually(t, "sealed while waiting", func() bool { return airlockStore.Exists(airlock.External, "imp2") })
	if s := getAirlock(t, ns, "imp2").Status.Status; s != treV1.AirlockSubmitted {
		t.Fatalf("an unscanned file must not reach review, status %s", s)
	}
	airlockStore.SetVerdict(airlock.External, "imp2", "payload.exe", "Malicious")
	eventually(t, "blocked", func() bool { return getAirlock(t, ns, "imp2").Status.Status == treV1.AirlockBlockedByScan })
	if ar := getAirlock(t, ns, "imp2"); ar.Status.StatusMessage != "Malicious" || ar.Status.ScanResult.Clean {
		t.Fatalf("status message %q, scan %+v", ar.Status.StatusMessage, ar.Status.ScanResult)
	}
}

func TestAirlockRejectsEmptyAndMultiFileRequests(t *testing.T) {
	ns := readyWorkspace(t, "airlock-c", treV1.PodSecurityRestricted)
	newAirlockRequest(t, ns, "exp-empty", treV1.AirlockExport)
	eventually(t, "export draft is internal", func() bool { return airlockStore.Exists(airlock.Internal, "exp-empty-draft") })
	setStatus(t, ns, "exp-empty", treV1.AirlockSubmitted)
	eventually(t, "failed empty", func() bool {
		ar := getAirlock(t, ns, "exp-empty")
		return ar.Status.Status == treV1.AirlockFailed && ar.Status.StatusMessage == "Request did not contain any files."
	})

	newAirlockRequest(t, ns, "exp-two", treV1.AirlockExport)
	eventually(t, "draft", func() bool { return airlockStore.Exists(airlock.Internal, "exp-two-draft") })
	airlockStore.Put(airlock.Internal, "exp-two-draft", "a.txt", []byte("a"))
	airlockStore.Put(airlock.Internal, "exp-two-draft", "b.txt", []byte("b"))
	setStatus(t, ns, "exp-two", treV1.AirlockSubmitted)
	eventually(t, "failed two", func() bool {
		return getAirlock(t, ns, "exp-two").Status.StatusMessage == "Request contained more than 1 file."
	})
}

func TestAirlockCancelDeletesData(t *testing.T) {
	ns := readyWorkspace(t, "airlock-d", treV1.PodSecurityRestricted)
	newAirlockRequest(t, ns, "exp3", treV1.AirlockExport)
	eventually(t, "draft", func() bool { return airlockStore.Exists(airlock.Internal, "exp3-draft") })
	airlockStore.Put(airlock.Internal, "exp3-draft", "results.csv", []byte("x"))
	setStatus(t, ns, "exp3", treV1.AirlockCancelled)
	eventually(t, "data deleted", func() bool { return !airlockStore.Exists(airlock.Internal, "exp3-draft") })
}
