package tre

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
	"github.com/isBioku/kubetre/internal/access"
	"github.com/isBioku/kubetre/internal/controller"
	"github.com/isBioku/kubetre/internal/schema"
)

// Shared services are WorkspaceServices in the shared namespace, created from SharedService
// templates. Every signed-in user can see them, as in AzureTRE; only TRE administrators can
// change them.

const typeSharedService = "shared-service"

func sharedPath(name string) string { return "/shared-services/" + name }

func (s *Server) sharedJSON(r *http.Request, svc *treV1.WorkspaceService, tmpl *treV1.ServiceTemplate) resource {
	props := rawMap(rawOf(svc.Spec.Values))
	props["display_name"] = svc.Spec.DisplayName
	props["description"] = svc.Spec.Description
	if o := svc.Annotations[annOverview]; o != "" {
		props["overview"] = o
	}
	for k, v := range s.sharedEndpoints(r, svc.Name) {
		props[k] = v
	}
	version := ""
	if tmpl != nil {
		version = templateVersion(tmpl)
	}
	return resource{
		ID: svc.Name, IsEnabled: enabled(svc.Spec.Enabled), ResourcePath: sharedPath(svc.Name),
		ResourceVersion: svc.Generation, ResourceType: typeSharedService, TemplateName: svc.Spec.TemplateRef,
		TemplateVersion: version, AvailableUpgrades: []any{},
		DeploymentStatus: deploymentStatus(svc, svc.Status.Phase, svc.Generation),
		UpdatedWhen:      updatedWhen(svc, svc.Status.Conditions), User: createdBy(svc),
		Etag: svc.ResourceVersion, Properties: props,
	}
}

// sharedEndpoints reports where workspaces reach a shared service: its internal load balancer
// address for VMs, and its cluster name for pods.
func (s *Server) sharedEndpoints(r *http.Request, release string) map[string]any {
	list := &corev1.ServiceList{}
	if err := s.Client.List(r.Context(), list, client.InNamespace(controller.SharedNamespace),
		client.MatchingLabels{"kubetre.io/service": release}); err != nil {
		return nil
	}
	for _, svc := range list.Items {
		out := map[string]any{"cluster_address": fmt.Sprintf("http://%s.%s.svc.cluster.local", svc.Name, svc.Namespace)}
		if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
			for _, ing := range svc.Status.LoadBalancer.Ingress {
				if ing.IP != "" {
					out["private_ip"] = ing.IP
					out["address"] = "http://" + ing.IP
				}
			}
		}
		return out
	}
	return nil
}

func (s *Server) loadShared(w http.ResponseWriter, r *http.Request) (*treV1.WorkspaceService, bool) {
	svc := &treV1.WorkspaceService{}
	err := s.Client.Get(r.Context(), types.NamespacedName{Namespace: controller.SharedNamespace, Name: r.PathValue("ss")}, svc)
	if apierrors.IsNotFound(err) {
		writeError(w, http.StatusNotFound, "shared service not found")
		return nil, false
	}
	if err != nil {
		s.internal(w, "get shared service", err)
		return nil, false
	}
	return svc, true
}

func (s *Server) listSharedServices(w http.ResponseWriter, r *http.Request, _ access.Identity) {
	list := &treV1.WorkspaceServiceList{}
	if err := s.Client.List(r.Context(), list, client.InNamespace(controller.SharedNamespace)); err != nil {
		s.internal(w, "list shared services", err)
		return
	}
	tmpls := s.serviceTemplates(r)
	out := []resource{}
	for i := range list.Items {
		out = append(out, s.sharedJSON(r, &list.Items[i], tmpls[list.Items[i].Spec.TemplateRef]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"sharedServices": out})
}

func (s *Server) getSharedService(w http.ResponseWriter, r *http.Request, _ access.Identity) {
	svc, ok := s.loadShared(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sharedService": s.sharedJSON(r, svc, s.serviceTemplates(r)[svc.Spec.TemplateRef])})
}

func (s *Server) requireAdmin(w http.ResponseWriter, id access.Identity, what string) bool {
	if !s.isAdmin(id) {
		writeError(w, http.StatusForbidden, "only TRE administrators can "+what)
		return false
	}
	return true
}

func (s *Server) createSharedService(w http.ResponseWriter, r *http.Request, id access.Identity) {
	if !s.requireAdmin(w, id, "add shared services") {
		return
	}
	b, ok := readBody(w, r)
	if !ok {
		return
	}
	tmpl := &treV1.ServiceTemplate{}
	if err := s.Client.Get(r.Context(), types.NamespacedName{Name: b.TemplateName}, tmpl); err != nil || !tmpl.IsSharedService() {
		if err == nil || apierrors.IsNotFound(err) {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("%q is not a shared service template", b.TemplateName))
			return
		}
		s.internal(w, "get template", err)
		return
	}
	system, values := splitServiceProps(b.Properties)
	display := strings.TrimSpace(str(system["display_name"]))
	if display == "" {
		writeError(w, http.StatusUnprocessableEntity, "display_name is required")
		return
	}
	raw, _ := json.Marshal(values)
	if err := schema.Validate("service-"+tmpl.Name, rawOf(tmpl.Spec.ValuesSchema), raw); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "properties do not match the template: "+err.Error())
		return
	}
	svc := &treV1.WorkspaceService{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: slug(display, 30), Namespace: controller.SharedNamespace,
			Annotations: map[string]string{annCreatedBy: id.Principal()},
		},
		Spec: treV1.WorkspaceServiceSpec{
			TemplateRef: tmpl.Name, DisplayName: display, Description: str(system["description"]),
			Values: &apiextensionsv1.JSON{Raw: raw},
		},
	}
	if o := str(system["overview"]); o != "" {
		svc.Annotations[annOverview] = o
	}
	if err := s.Client.Create(r.Context(), svc); err != nil {
		if apierrors.IsInvalid(err) {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		s.internal(w, "create shared service", err)
		return
	}
	s.logger().Info("shared service created", "name", svc.Name, "template", tmpl.Name, "by", id.Principal())
	writeJSON(w, http.StatusAccepted, map[string]any{"operation": newOperation(svc.UID, actionInstall, 1,
		sharedPath(svc.Name), svc.Name, typeSharedService, tmpl.Name, "awaiting_deployment", "", id, s.now())})
}

func (s *Server) patchSharedService(w http.ResponseWriter, r *http.Request, id access.Identity) {
	if !s.requireAdmin(w, id, "update shared services") {
		return
	}
	svc, ok := s.loadShared(w, r)
	if !ok {
		return
	}
	s.patch(w, r, id, nil, svc, sharedPath(svc.Name), typeSharedService)
}

func (s *Server) deleteSharedService(w http.ResponseWriter, r *http.Request, id access.Identity) {
	if !s.requireAdmin(w, id, "delete shared services") {
		return
	}
	svc, ok := s.loadShared(w, r)
	if !ok {
		return
	}
	s.remove(w, r, id, svc, sharedPath(svc.Name), typeSharedService)
}

// sharedTarget resolves a shared service for operations and history requests.
func (s *Server) sharedTarget(w http.ResponseWriter, r *http.Request) (*target, bool, bool) {
	svc := &treV1.WorkspaceService{}
	err := s.Client.Get(r.Context(), types.NamespacedName{Namespace: controller.SharedNamespace, Name: r.PathValue("ss")}, svc)
	if apierrors.IsNotFound(err) {
		return nil, false, true
	}
	if err != nil {
		s.internal(w, "get shared service", err)
		return nil, false, false
	}
	return &target{obj: svc, phase: svc.Status.Phase, conditions: svc.Status.Conditions, observed: svc.Status.ObservedGeneration,
		path: sharedPath(svc.Name), resourceType: typeSharedService, template: svc.Spec.TemplateRef}, true, true
}

func (s *Server) listSharedOperations(w http.ResponseWriter, r *http.Request, _ access.Identity) {
	t, found, ok := s.sharedTarget(w, r)
	if !ok {
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "shared service not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"operations": targetOperations(t)})
}

func (s *Server) getSharedOperation(w http.ResponseWriter, r *http.Request, _ access.Identity) {
	uid, action, gen, ok := parseOperationID(r.PathValue("op"))
	if !ok {
		writeError(w, http.StatusNotFound, "operation not found")
		return
	}
	t, found, ok := s.sharedTarget(w, r)
	if !ok {
		return
	}
	path := sharedPath(r.PathValue("ss"))
	op := buildOperation(uid, action, gen, path, r.PathValue("ss"), typeSharedService, "",
		statusFor(t, found, uid, action, gen), "", userOf(""), unix(s.now()), unix(s.now()))
	if found {
		op = buildOperation(uid, action, gen, path, t.obj.GetName(), typeSharedService, t.template,
			statusFor(t, true, uid, action, gen), readyMessage(t.conditions), createdBy(t.obj),
			unix(t.obj.GetCreationTimestamp().Time), updatedWhen(t.obj, t.conditions))
	}
	writeJSON(w, http.StatusOK, map[string]any{"operation": op})
}

func (s *Server) getSharedHistory(w http.ResponseWriter, r *http.Request, _ access.Identity) {
	if _, found, ok := s.sharedTarget(w, r); !ok {
		return
	} else if !found {
		writeError(w, http.StatusNotFound, "shared service not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"resource_history": []any{}})
}

func (s *Server) listSharedServiceTemplates(w http.ResponseWriter, r *http.Request, _ access.Identity) {
	out, err := s.serviceTemplateList(r, func(t *treV1.ServiceTemplate) bool { return t.IsSharedService() })
	if err != nil {
		s.internal(w, "list templates", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": out})
}

func (s *Server) getSharedServiceTemplate(w http.ResponseWriter, r *http.Request, _ access.Identity) {
	t, ok := s.loadServiceTemplate(w, r, r.PathValue("name"))
	if !ok {
		return
	}
	if !t.IsSharedService() {
		writeError(w, http.StatusNotFound, "template not found")
		return
	}
	writeJSON(w, http.StatusOK, serviceTemplateJSON(t, isUpdate(r)))
}
