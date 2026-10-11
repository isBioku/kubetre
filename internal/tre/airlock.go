package tre

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
	"github.com/isBioku/kubetre/internal/access"
	"github.com/isBioku/kubetre/internal/airlock"
	"github.com/isBioku/kubetre/internal/controller"
)

// AzureTRE's airlock API. Requests are AirlockRequest resources in the workspace namespace;
// the controller moves their data between stages.

const (
	// linkValidity matches AzureTRE's default for links users ask for.
	linkValidity = time.Hour
	// reviewLinkValidity is longer: a review VM takes minutes to deploy before it is used.
	reviewLinkValidity = 8 * time.Hour
)

type airlockUserJSON struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Email           string   `json:"email"`
	Roles           []string `json:"roles"`
	RoleAssignments []any    `json:"roleAssignments"`
}

func airlockUser(u treV1.AirlockUser) airlockUserJSON {
	return airlockUserJSON{ID: u.ID, Name: u.Name, Email: u.Email, Roles: []string{}, RoleAssignments: []any{}}
}

func actorOf(id access.Identity) treV1.AirlockUser {
	name := id.Name
	if name == "" {
		name = id.Principal()
	}
	return treV1.AirlockUser{ID: userID(id), Name: name, Email: id.Email}
}

func airlockJSON(ar *treV1.AirlockRequest) map[string]any {
	created := unix(ar.CreationTimestamp.Time)
	updated := created
	if ar.Status.UpdatedWhen != nil {
		updated = unix(ar.Status.UpdatedWhen.Time)
	}
	updatedBy := ar.Status.UpdatedBy
	if updatedBy.ID == "" {
		updatedBy = ar.Spec.CreatedBy
	}
	history := []map[string]any{}
	for _, h := range ar.Status.History {
		by := h.UpdatedBy
		if by.ID == "" {
			by = ar.Spec.CreatedBy
		}
		history = append(history, map[string]any{"resourceVersion": h.Version, "updatedWhen": unix(h.When.Time),
			"updatedBy": airlockUser(by), "properties": map[string]any{"previousStatus": h.PreviousStatus}})
	}
	files := []map[string]any{}
	for _, f := range ar.Status.Files {
		files = append(files, map[string]any{"name": f.Name, "size": f.Size})
	}
	reviews := []map[string]any{}
	for _, r := range ar.Status.Reviews {
		reviews = append(reviews, map[string]any{"id": r.ID, "reviewer": airlockUser(r.Reviewer), "dateCreated": unix(r.When.Time),
			"reviewDecision": r.Decision, "decisionExplanation": r.Explanation})
	}
	reviewResources := map[string]any{}
	for user, rr := range ar.Status.ReviewResources {
		reviewResources[user] = map[string]any{"workspaceId": rr.Workspace, "workspaceServiceId": rr.Service, "userResourceId": rr.UserResource}
	}
	out := map[string]any{
		"id": ar.Name, "resourceVersion": ar.Status.Version, "createdBy": airlockUser(ar.Spec.CreatedBy), "createdWhen": created,
		"updatedBy": airlockUser(updatedBy), "updatedWhen": updated, "history": history,
		"workspaceId": strings.TrimPrefix(ar.Namespace, controller.NamespacePrefix), "type": ar.Spec.Type, "files": files,
		"title": ar.Spec.Title, "businessJustification": ar.Spec.BusinessJustification, "status": ar.Status.Status,
		"reviews": reviews, "reviewUserResources": reviewResources, "_etag": ar.ResourceVersion, "airlock_version": 2,
	}
	if ar.Status.StatusMessage != "" {
		out["statusMessage"] = ar.Status.StatusMessage
	}
	if ar.Status.ScanResult != nil {
		out["scanResult"] = map[string]any{"clean": ar.Status.ScanResult.Clean, "message": ar.Status.ScanResult.Message}
	}
	return out
}

func withActions(ar *treV1.AirlockRequest, roles map[string]bool) map[string]any {
	return map[string]any{"airlockRequest": airlockJSON(ar), "allowedUserActions": airlock.AllowedActions(ar.Status.Status, roles)}
}

// airlockWorkspace loads the workspace and the caller's roles, requiring one of the given roles.
// Unlike most of the API, TRE administrators without a workspace role are not let in, as in AzureTRE.
func (s *Server) airlockWorkspace(w http.ResponseWriter, r *http.Request, id access.Identity, allowed ...string) (*treV1.Workspace, map[string]bool, bool) {
	if s.Airlock == nil {
		writeError(w, http.StatusNotFound, "the airlock is not configured in this TRE")
		return nil, nil, false
	}
	ws, roles, ok := s.loadWorkspace(w, r, id)
	if !ok {
		return nil, nil, false
	}
	for _, role := range allowed {
		if roles[role] {
			return ws, roles, true
		}
	}
	writeError(w, http.StatusForbidden, fmt.Sprintf("The user is missing a required role: %v", allowed))
	return nil, nil, false
}

func (s *Server) loadAirlock(w http.ResponseWriter, r *http.Request, ws *treV1.Workspace) (*treV1.AirlockRequest, bool) {
	ar := &treV1.AirlockRequest{}
	err := s.Client.Get(r.Context(), types.NamespacedName{Namespace: controller.NamespaceFor(ws), Name: r.PathValue("req")}, ar)
	if apierrors.IsNotFound(err) {
		writeError(w, http.StatusNotFound, "Airlock request does not exist")
		return nil, false
	}
	if err != nil {
		s.internal(w, "get airlock request", err)
		return nil, false
	}
	return ar, true
}

var members = []string{access.RoleOwner, access.RoleResearcher}
var anyAirlockRole = []string{access.RoleOwner, access.RoleResearcher, access.RoleAirlockManager}

func deployed(ws *treV1.Workspace) bool { return ws.Status.Phase == treV1.PhaseReady }

// airlockSort orders requests as the UI asks. Unknown fields fall back to updatedWhen; the UI's
// "createdTime" means createdWhen.
func airlockSort(list []treV1.AirlockRequest, orderBy string, ascending bool) {
	key := func(a *treV1.AirlockRequest) string {
		switch orderBy {
		case "createdWhen", "createdTime":
			return a.CreationTimestamp.UTC().Format(time.RFC3339Nano)
		case "title":
			return strings.ToLower(a.Spec.Title)
		case "status":
			return a.Status.Status
		case "type":
			return a.Spec.Type
		}
		if a.Status.UpdatedWhen != nil {
			return a.Status.UpdatedWhen.UTC().Format(time.RFC3339Nano)
		}
		return a.CreationTimestamp.UTC().Format(time.RFC3339Nano)
	}
	sort.SliceStable(list, func(i, j int) bool {
		if ascending {
			return key(&list[i]) < key(&list[j])
		}
		return key(&list[i]) > key(&list[j])
	})
}

func airlockFilter(r *http.Request) func(*treV1.AirlockRequest) bool {
	q := r.URL.Query()
	creator, typ, status := q.Get("creator_user_id"), q.Get("type"), q.Get("status")
	if status == "blocked" {
		status = treV1.AirlockBlockedByScan
	}
	if q.Get("awaiting_current_user_review") == "true" {
		status = treV1.AirlockInReview
	}
	return func(a *treV1.AirlockRequest) bool {
		return (creator == "" || a.Spec.CreatedBy.ID == creator) && (typ == "" || a.Spec.Type == typ) &&
			(status == "" || a.Status.Status == status)
	}
}

func (s *Server) listWorkspaceAirlock(r *http.Request, ws *treV1.Workspace) ([]treV1.AirlockRequest, error) {
	list := &treV1.AirlockRequestList{}
	if err := s.Client.List(r.Context(), list, client.InNamespace(controller.NamespaceFor(ws))); err != nil {
		return nil, err
	}
	keep := airlockFilter(r)
	out := []treV1.AirlockRequest{}
	for _, a := range list.Items {
		if keep(&a) {
			out = append(out, a)
		}
	}
	airlockSort(out, r.URL.Query().Get("order_by"), r.URL.Query().Get("order_ascending") != "false")
	return out, nil
}

func (s *Server) listAirlockRequests(w http.ResponseWriter, r *http.Request, id access.Identity) {
	ws, roles, ok := s.airlockWorkspace(w, r, id, anyAirlockRole...)
	if !ok {
		return
	}
	items, err := s.listWorkspaceAirlock(r, ws)
	if err != nil {
		s.internal(w, "list airlock requests", err)
		return
	}
	out := []map[string]any{}
	for i := range items {
		out = append(out, withActions(&items[i], roles))
	}
	writeJSON(w, http.StatusOK, map[string]any{"airlockRequests": out})
}

func (s *Server) getAirlockRequest(w http.ResponseWriter, r *http.Request, id access.Identity) {
	ws, roles, ok := s.airlockWorkspace(w, r, id, anyAirlockRole...)
	if !ok {
		return
	}
	ar, ok := s.loadAirlock(w, r, ws)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, withActions(ar, roles))
}

func (s *Server) createAirlockRequest(w http.ResponseWriter, r *http.Request, id access.Identity) {
	ws, roles, ok := s.airlockWorkspace(w, r, id, members...)
	if !ok {
		return
	}
	if !deployed(ws) {
		writeError(w, http.StatusConflict, "Workspace is not deployed.")
		return
	}
	if !ws.Spec.Airlock.Enabled {
		writeError(w, http.StatusMethodNotAllowed, "Airlock is not enabled in this workspace.")
		return
	}
	var body struct {
		Type                  string `json:"type"`
		Title                 string `json:"title"`
		BusinessJustification string `json:"businessJustification"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid request body: "+err.Error())
		return
	}
	if body.Type != treV1.AirlockImport && body.Type != treV1.AirlockExport {
		writeError(w, http.StatusUnprocessableEntity, "type must be import or export")
		return
	}
	if strings.TrimSpace(body.Title) == "" {
		body.Title = "Airlock Request"
	}
	ar := &treV1.AirlockRequest{
		ObjectMeta: metav1.ObjectMeta{Name: uuid.NewString(), Namespace: controller.NamespaceFor(ws)},
		Spec: treV1.AirlockRequestSpec{Type: body.Type, Title: strings.TrimSpace(body.Title),
			BusinessJustification: strings.TrimSpace(body.BusinessJustification), CreatedBy: actorOf(id)},
	}
	if err := s.Client.Create(r.Context(), ar); err != nil {
		if apierrors.IsInvalid(err) {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		s.internal(w, "create airlock request", err)
		return
	}
	now := metav1.NewTime(s.now())
	ar.Status = treV1.AirlockRequestStatus{Status: treV1.AirlockDraft, UpdatedBy: ar.Spec.CreatedBy, UpdatedWhen: &now}
	if err := s.Client.Status().Update(r.Context(), ar); err != nil {
		s.internal(w, "start airlock request", err)
		return
	}
	// The draft container exists before the response, so the upload link works at once.
	acct, ctr := airlock.Location(ar.Spec.Type, treV1.AirlockDraft, ar.Name)
	if err := s.Airlock.EnsureContainer(r.Context(), acct, ctr); err != nil {
		s.internal(w, "create draft container", err)
		return
	}
	s.logger().Info("airlock request created", "workspace", ws.Name, "request", ar.Name, "type", ar.Spec.Type, "by", id.Principal())
	writeJSON(w, http.StatusCreated, withActions(ar, roles))
}

// act applies a user action to the request, retrying on conflicts with the controller.
func (s *Server) act(w http.ResponseWriter, r *http.Request, ws *treV1.Workspace, roles map[string]bool,
	change func(ar *treV1.AirlockRequest) error) (*treV1.AirlockRequest, bool) {
	var ar *treV1.AirlockRequest
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cur := &treV1.AirlockRequest{}
		if err := s.Client.Get(r.Context(), types.NamespacedName{Namespace: controller.NamespaceFor(ws), Name: r.PathValue("req")}, cur); err != nil {
			return err
		}
		if err := change(cur); err != nil {
			return err
		}
		ar = cur
		return s.Client.Status().Update(r.Context(), cur)
	})
	switch {
	case err == nil:
		return ar, true
	case apierrors.IsNotFound(err):
		writeError(w, http.StatusNotFound, "Airlock request does not exist")
	case errors.Is(err, airlock.ErrIllegalTransition):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, errBadRequest):
		writeError(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), errBadRequest.Error()+": "))
	default:
		s.internal(w, "update airlock request", err)
	}
	return nil, false
}

var errBadRequest = errors.New("bad request")

func (s *Server) submitAirlockRequest(w http.ResponseWriter, r *http.Request, id access.Identity) {
	ws, roles, ok := s.airlockWorkspace(w, r, id, members...)
	if !ok {
		return
	}
	actor := actorOf(id)
	ar, ok := s.act(w, r, ws, roles, func(ar *treV1.AirlockRequest) error {
		return airlock.Transition(ar, treV1.AirlockSubmitted, &actor, s.now())
	})
	if ok {
		writeJSON(w, http.StatusOK, withActions(ar, roles))
	}
}

func (s *Server) cancelAirlockRequest(w http.ResponseWriter, r *http.Request, id access.Identity) {
	ws, roles, ok := s.airlockWorkspace(w, r, id, members...)
	if !ok {
		return
	}
	actor := actorOf(id)
	ar, ok := s.act(w, r, ws, roles, func(ar *treV1.AirlockRequest) error {
		return airlock.Transition(ar, treV1.AirlockCancelled, &actor, s.now())
	})
	if ok {
		s.removeReviewResources(r, ar)
		writeJSON(w, http.StatusOK, withActions(ar, roles))
	}
}

func (s *Server) revokeAirlockRequest(w http.ResponseWriter, r *http.Request, id access.Identity) {
	ws, roles, ok := s.airlockWorkspace(w, r, id, access.RoleAirlockManager)
	if !ok {
		return
	}
	var body struct {
		Reason *string `json:"reason"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil || body.Reason == nil {
		writeError(w, http.StatusUnprocessableEntity, "reason is required")
		return
	}
	actor := actorOf(id)
	ar, ok := s.act(w, r, ws, roles, func(ar *treV1.AirlockRequest) error {
		if err := airlock.Transition(ar, treV1.AirlockRevoked, &actor, s.now()); err != nil {
			return err
		}
		ar.Status.Reviews = append(ar.Status.Reviews, treV1.AirlockReview{ID: uuid.NewString(), Decision: "revoked",
			Explanation: *body.Reason, Reviewer: actor, When: metav1.NewTime(s.now())})
		return nil
	})
	if ok {
		writeJSON(w, http.StatusOK, withActions(ar, roles))
	}
}

func (s *Server) reviewAirlockRequest(w http.ResponseWriter, r *http.Request, id access.Identity) {
	ws, roles, ok := s.airlockWorkspace(w, r, id, access.RoleAirlockManager)
	if !ok {
		return
	}
	if !deployed(ws) {
		writeError(w, http.StatusConflict, "Workspace is not deployed.")
		return
	}
	var body struct {
		Approval            *bool  `json:"approval"`
		DecisionExplanation string `json:"decisionExplanation"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil || body.Approval == nil {
		writeError(w, http.StatusUnprocessableEntity, "approval is required")
		return
	}
	if body.DecisionExplanation == "" {
		body.DecisionExplanation = "Decision Explanation"
	}
	actor := actorOf(id)
	to, decision := treV1.AirlockRejectionInProgress, "rejected"
	if *body.Approval {
		to, decision = treV1.AirlockApprovalInProgress, "approved"
	}
	ar, ok := s.act(w, r, ws, roles, func(ar *treV1.AirlockRequest) error {
		if err := airlock.Transition(ar, to, &actor, s.now()); err != nil {
			return err
		}
		ar.Status.Reviews = append(ar.Status.Reviews, treV1.AirlockReview{ID: uuid.NewString(), Decision: decision,
			Explanation: body.DecisionExplanation, Reviewer: actor, When: metav1.NewTime(s.now())})
		return nil
	})
	if ok {
		s.logger().Info("airlock request reviewed", "workspace", ws.Name, "request", ar.Name, "decision", decision, "by", id.Principal())
		s.removeReviewResources(r, ar)
		writeJSON(w, http.StatusOK, withActions(ar, roles))
	}
}

func (s *Server) airlockLink(w http.ResponseWriter, r *http.Request, id access.Identity) {
	ws, roles, ok := s.airlockWorkspace(w, r, id, anyAirlockRole...)
	if !ok {
		return
	}
	if !deployed(ws) {
		writeError(w, http.StatusConflict, "Workspace is not deployed.")
		return
	}
	ar, ok := s.loadAirlock(w, r, ws)
	if !ok {
		return
	}
	perm, err := airlock.LinkPermissions(ar.Status.Status, roles)
	if errors.Is(err, airlock.ErrLinkForbidden) {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	acct, ctr := airlock.Location(ar.Spec.Type, ar.Status.Status, ar.Name)
	url, err := s.Airlock.ContainerURL(r.Context(), acct, ctr, perm, linkValidity)
	if err != nil {
		s.internal(w, "sign airlock link", err)
		return
	}
	s.logger().Info("airlock link issued", "workspace", ws.Name, "request", ar.Name, "status", ar.Status.Status,
		"permissions", perm.String(), "by", id.Principal())
	writeJSON(w, http.StatusOK, map[string]any{"containerUrl": url})
}

// listMyAirlockRequests is GET /api/requests: the caller's own requests, or with
// airlock_manager=true, the requests of every workspace they manage the airlock for.
func (s *Server) listMyAirlockRequests(w http.ResponseWriter, r *http.Request, id access.Identity) {
	out := []map[string]any{}
	if s.Airlock == nil {
		writeJSON(w, http.StatusOK, out)
		return
	}
	wsList := &treV1.WorkspaceList{}
	if err := s.Client.List(r.Context(), wsList); err != nil {
		s.internal(w, "list workspaces", err)
		return
	}
	manager := r.URL.Query().Get("airlock_manager") == "true"
	me := userID(id)
	for i := range wsList.Items {
		ws := &wsList.Items[i]
		roles := rolesOf(ws, id)
		if manager && !roles[access.RoleAirlockManager] {
			continue
		}
		if !manager && len(roles) == 0 {
			continue
		}
		items, err := s.listWorkspaceAirlock(r, ws)
		if err != nil {
			s.internal(w, "list airlock requests", err)
			return
		}
		for j := range items {
			if manager || items[j].Spec.CreatedBy.ID == me {
				out = append(out, airlockJSON(&items[j]))
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// createReviewResource creates a review VM for the airlock manager, with a read-only link to the
// request's data passed in as a value.
func (s *Server) createReviewResource(w http.ResponseWriter, r *http.Request, id access.Identity) {
	ws, roles, ok := s.airlockWorkspace(w, r, id, access.RoleAirlockManager)
	if !ok {
		return
	}
	if !deployed(ws) {
		writeError(w, http.StatusConflict, "Workspace is not deployed.")
		return
	}
	ar, ok := s.loadAirlock(w, r, ws)
	if !ok {
		return
	}
	if ar.Status.Status != treV1.AirlockInReview {
		writeError(w, http.StatusBadRequest, "Airlock request must be in 'in_review' status to create a Review User Resource")
		return
	}
	cfg := ws.Spec.Airlock.Review.Export
	targetWS := ws
	if ar.Spec.Type == treV1.AirlockImport {
		cfg = ws.Spec.Airlock.Review.Import
		if cfg != nil {
			targetWS = &treV1.Workspace{}
			if err := s.Client.Get(r.Context(), types.NamespacedName{Name: cfg.Workspace}, targetWS); err != nil {
				cfg = nil
			}
		}
	}
	if cfg == nil {
		writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf(
			"Failed to retrieve Airlock Review configuration for workspace %s. Please ask your TRE administrator to check the configuration.", ws.Name))
		return
	}
	me := userID(id)
	if existing, has := ar.Status.ReviewResources[me]; has {
		svc := &treV1.WorkspaceService{}
		err := s.Client.Get(r.Context(), types.NamespacedName{Namespace: controller.NamespacePrefix + existing.Workspace, Name: existing.UserResource}, svc)
		if err == nil && svc.DeletionTimestamp.IsZero() && enabled(svc.Spec.Enabled) && svc.Status.Phase == treV1.PhaseReady {
			writeError(w, http.StatusConflict, "A healthy review resource is already deployed for the current user. You may only have a single review resource.")
			return
		}
		if err == nil {
			s.retire(r, svc)
		}
	}

	parent := &treV1.WorkspaceService{}
	if err := s.Client.Get(r.Context(), types.NamespacedName{Namespace: controller.NamespaceFor(targetWS), Name: cfg.Service}, parent); err != nil {
		writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf(
			"Failed to retrieve Airlock Review configuration for workspace %s. Please ask your TRE administrator to check the configuration. Details: %v", ws.Name, err))
		return
	}
	tmpl := &treV1.ServiceTemplate{}
	if err := s.Client.Get(r.Context(), types.NamespacedName{Name: cfg.Template}, tmpl); err != nil ||
		!tmpl.IsUserResource() || tmpl.Spec.ParentTemplate != parent.Spec.TemplateRef {
		writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf(
			"Failed to retrieve Airlock Review configuration for workspace %s. Please ask your TRE administrator to check the configuration. Details: %q is not a user resource template of %q",
			ws.Name, cfg.Template, parent.Spec.TemplateRef))
		return
	}
	acct, ctr := airlock.Location(ar.Spec.Type, treV1.AirlockInReview, ar.Name)
	url, err := s.Airlock.ContainerURL(r.Context(), acct, ctr, airlock.Permissions{Read: true, List: true}, reviewLinkValidity)
	if err != nil {
		s.internal(w, "sign review link", err)
		return
	}
	values := map[string]any{access.AirlockRequestURLValue: url}
	raw, _ := json.Marshal(values)
	if tmpl.Spec.OwnerAccount {
		if raw, err = access.WithVMUsername(raw, id.Principal()); err != nil {
			s.internal(w, "set VM username", err)
			return
		}
	}
	vm := &treV1.WorkspaceService{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "airlock-review-", Namespace: controller.NamespaceFor(targetWS),
			Annotations: map[string]string{annCreatedBy: id.Principal(), annOwnerID: me, "kubetre.io/airlock-request": ar.Name},
		},
		Spec: treV1.WorkspaceServiceSpec{TemplateRef: tmpl.Name, DisplayName: "Airlock Review VM",
			Description: fmt.Sprintf("%s (ID %s)", ar.Spec.Title, ar.Name), ParentService: parent.Name,
			Owner: id.Principal(), Values: &apiextensionsv1.JSON{Raw: raw}},
	}
	if err := s.Client.Create(r.Context(), vm); err != nil {
		s.internal(w, "create review VM", err)
		return
	}
	updated, ok := s.act(w, r, ws, roles, func(cur *treV1.AirlockRequest) error {
		airlock.Touch(cur, nil, s.now())
		if cur.Status.ReviewResources == nil {
			cur.Status.ReviewResources = map[string]treV1.AirlockReviewResource{}
		}
		cur.Status.ReviewResources[me] = treV1.AirlockReviewResource{Workspace: targetWS.Name, Service: parent.Name, UserResource: vm.Name}
		return nil
	})
	if !ok {
		return
	}
	path := userResourcePath(targetWS.Name, parent.Name, vm.Name)
	op := newOperation(vm.UID, actionInstall, 1, path, vm.Name, typeUserResource, tmpl.Name, "awaiting_deployment", "", id, s.now())
	w.Header().Set("Location", "/api"+path+"/operations/"+op.ID)
	s.logger().Info("airlock review VM created", "request", ar.Name, "workspace", targetWS.Name, "vm", vm.Name, "by", id.Principal())
	writeJSON(w, http.StatusAccepted, map[string]any{"airlockRequest": airlockJSON(updated), "operation": op})
}

// removeReviewResources disables and deletes a request's review VMs, skipping any already gone.
func (s *Server) removeReviewResources(r *http.Request, ar *treV1.AirlockRequest) {
	for _, rr := range ar.Status.ReviewResources {
		svc := &treV1.WorkspaceService{}
		if err := s.Client.Get(r.Context(), types.NamespacedName{Namespace: controller.NamespacePrefix + rr.Workspace, Name: rr.UserResource}, svc); err == nil {
			s.retire(r, svc)
		}
	}
}

func (s *Server) retire(r *http.Request, svc *treV1.WorkspaceService) {
	f := false
	svc.Spec.Enabled = &f
	if err := s.Client.Update(r.Context(), svc); err != nil {
		s.logger().Warn("could not disable review VM", "vm", svc.Name, "error", err)
	}
	if err := s.Client.Delete(r.Context(), svc); client.IgnoreNotFound(err) != nil {
		s.logger().Warn("could not delete review VM", "vm", svc.Name, "error", err)
	}
}
