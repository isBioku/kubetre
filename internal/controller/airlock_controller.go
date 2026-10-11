package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
	"github.com/isBioku/kubetre/internal/airlock"
)

// AirlockFinalizer deletes a request's data when the request, or its workspace, is deleted.
const AirlockFinalizer = "kubetre.io/airlock-data"

// AirlockRequestReconciler moves airlock data between stages. The API records what users do
// (submit, review, cancel); this completes the in-progress statuses: it seals a submitted
// file, waits for the malware scan, copies approved data out or in, and sends notifications.
type AirlockRequestReconciler struct {
	client.Client
	Store           airlock.Store
	Notifier        airlock.Notifier
	MalwareScanning bool
	UIURL           string
	// ScanPoll is how often a request waiting for its scan result is checked; default 20s.
	ScanPoll time.Duration
	Now      func() time.Time
}

// +kubebuilder:rbac:groups=kubetre.io,resources=airlockrequests,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=kubetre.io,resources=airlockrequests/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kubetre.io,resources=airlockrequests/finalizers,verbs=update

func (r *AirlockRequestReconciler) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// Reconcile drives one airlock request.
func (r *AirlockRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ar := &treV1.AirlockRequest{}
	if err := r.Get(ctx, req.NamespacedName, ar); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !ar.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(ar, AirlockFinalizer) {
			if err := r.deleteAll(ctx, ar); err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(ar, AirlockFinalizer)
			return ctrl.Result{}, r.Update(ctx, ar)
		}
		return ctrl.Result{}, nil
	}
	if !controllerutil.ContainsFinalizer(ar, AirlockFinalizer) {
		controllerutil.AddFinalizer(ar, AirlockFinalizer)
		if err := r.Update(ctx, ar); err != nil {
			return ctrl.Result{}, err
		}
	}

	requeue, err := r.advance(ctx, ar)
	if err != nil {
		return ctrl.Result{}, err
	}
	if err := r.notify(ctx, ar); err != nil {
		log.FromContext(ctx).Error(err, "airlock notification failed", "request", ar.Name)
	}
	return ctrl.Result{RequeueAfter: requeue}, nil
}

// advance performs the storage work for the current status and moves the request on.
func (r *AirlockRequestReconciler) advance(ctx context.Context, ar *treV1.AirlockRequest) (time.Duration, error) {
	id, typ := ar.Name, ar.Spec.Type
	switch ar.Status.Status {
	case treV1.AirlockDraft:
		acct, ctr := airlock.Location(typ, treV1.AirlockDraft, id)
		return 0, r.Store.EnsureContainer(ctx, acct, ctr)

	case treV1.AirlockSubmitted:
		return r.seal(ctx, ar)

	case treV1.AirlockBlockingInProgress:
		return 0, r.move(ctx, ar, treV1.AirlockBlockedByScan, "")

	case treV1.AirlockRejectionInProgress:
		return 0, r.move(ctx, ar, treV1.AirlockRejected, "")

	case treV1.AirlockApprovalInProgress:
		fromAcct, ctr := airlock.Location(typ, treV1.AirlockInReview, id)
		toAcct, _ := airlock.Location(typ, treV1.AirlockApproved, id)
		srcFiles, err := r.Store.ListFiles(ctx, fromAcct, ctr)
		switch {
		case errors.Is(err, airlock.ErrNotFound):
			// Copied on an earlier attempt, and the source already removed.
			if _, err := r.Store.ListFiles(ctx, toAcct, ctr); err != nil {
				return 0, r.move(ctx, ar, treV1.AirlockFailed, "Request data could not be found in storage.")
			}
		case err != nil:
			return 0, err
		case len(srcFiles) > 0:
			if err := r.Store.Copy(ctx, fromAcct, ctr, toAcct, ctr); err != nil {
				return 0, fmt.Errorf("copy approved data: %w", err)
			}
			if err := r.Store.DeleteContainer(ctx, fromAcct, ctr); err != nil {
				return 0, err
			}
		}
		return 0, r.move(ctx, ar, treV1.AirlockApproved, "")

	case treV1.AirlockCancelled:
		return 0, r.deleteAll(ctx, ar)
	}
	return 0, nil
}

// seal copies the draft's single file into the sealed container and deletes the draft, so
// links issued for the draft stop working. Then it waits for the malware verdict, if scanning
// is on, before the request goes to review.
func (r *AirlockRequestReconciler) seal(ctx context.Context, ar *treV1.AirlockRequest) (time.Duration, error) {
	id := ar.Name
	acct, draft := airlock.Location(ar.Spec.Type, treV1.AirlockDraft, id)
	sealed := airlock.SealedContainer(id)

	files, err := r.Store.ListFiles(ctx, acct, draft)
	switch {
	case errors.Is(err, airlock.ErrNotFound):
		// Sealed on an earlier attempt?
		if files, err = r.Store.ListFiles(ctx, acct, sealed); err != nil {
			return 0, r.move(ctx, ar, treV1.AirlockFailed, "Request data could not be found in storage.")
		}
	case err != nil:
		return 0, err
	default:
		if len(ar.Status.Files) == 0 {
			ar.Status.Files = toFiles(files)
		}
		switch {
		case len(files) == 0:
			return 0, r.move(ctx, ar, treV1.AirlockFailed, "Request did not contain any files.")
		case len(files) > 1:
			return 0, r.move(ctx, ar, treV1.AirlockFailed, "Request contained more than 1 file.")
		}
		if err := r.Store.Copy(ctx, acct, draft, acct, sealed); err != nil {
			return 0, fmt.Errorf("seal: %w", err)
		}
		if err := r.Store.DeleteContainer(ctx, acct, draft); err != nil {
			return 0, err
		}
	}
	if len(ar.Status.Files) == 0 {
		ar.Status.Files = toFiles(files)
	}

	if !r.MalwareScanning {
		return 0, r.move(ctx, ar, treV1.AirlockInReview, "")
	}
	verdicts, err := r.Store.ScanVerdicts(ctx, acct, sealed)
	if err != nil {
		return 0, err
	}
	verdict, scanned := verdicts[files[0].Name]
	if !scanned {
		// Defender scans asynchronously, usually within a minute.
		poll := r.ScanPoll
		if poll == 0 {
			poll = 20 * time.Second
		}
		return poll, r.Status().Update(ctx, ar)
	}
	ar.Status.ScanResult = &treV1.AirlockScanResult{Clean: verdict == airlock.ScanClean}
	if verdict != airlock.ScanClean {
		ar.Status.ScanResult.Message = verdict
		return 0, r.move(ctx, ar, treV1.AirlockBlockingInProgress, verdict)
	}
	return 0, r.move(ctx, ar, treV1.AirlockInReview, "")
}

func toFiles(in []airlock.File) []treV1.AirlockFile {
	out := make([]treV1.AirlockFile, 0, len(in))
	for _, f := range in {
		out = append(out, treV1.AirlockFile{Name: f.Name, Size: f.Size})
	}
	return out
}

// move applies a platform-driven transition and saves the status.
func (r *AirlockRequestReconciler) move(ctx context.Context, ar *treV1.AirlockRequest, to, message string) error {
	if err := airlock.Transition(ar, to, nil, r.now()); err != nil {
		return err
	}
	if message != "" {
		ar.Status.StatusMessage = message
	}
	return r.Status().Update(ctx, ar)
}

// deleteAll removes every container the request could have used.
func (r *AirlockRequestReconciler) deleteAll(ctx context.Context, ar *treV1.AirlockRequest) error {
	for _, acct := range []airlock.Account{airlock.External, airlock.Internal} {
		for _, ctr := range []string{airlock.DraftContainer(ar.Name), airlock.SealedContainer(ar.Name)} {
			if err := r.Store.DeleteContainer(ctx, acct, ctr); err != nil {
				return err
			}
		}
	}
	return nil
}

// notify sends the notifications for the request's status once.
func (r *AirlockRequestReconciler) notify(ctx context.Context, ar *treV1.AirlockRequest) error {
	if ar.Status.Status == "" || ar.Status.NotifiedStatus == ar.Status.Status {
		return nil
	}
	if r.Notifier != nil {
		ws, err := r.workspaceOf(ctx, ar)
		if err != nil {
			return err
		}
		for _, m := range airlock.Messages(ar, ws, r.UIURL) {
			if err := r.Notifier.Send(ctx, m); err != nil {
				return err
			}
		}
	}
	ar.Status.NotifiedStatus = ar.Status.Status
	return r.Status().Update(ctx, ar)
}

func (r *AirlockRequestReconciler) workspaceOf(ctx context.Context, ar *treV1.AirlockRequest) (*treV1.Workspace, error) {
	name := strings.TrimPrefix(ar.Namespace, NamespacePrefix)
	ws := &treV1.Workspace{}
	if err := r.Get(ctx, types.NamespacedName{Name: name}, ws); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("workspace %s not found", name)
		}
		return nil, err
	}
	return ws, nil
}

// SetupWithManager registers the reconciler.
func (r *AirlockRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).For(&treV1.AirlockRequest{}).Named("airlockrequest").Complete(r)
}
