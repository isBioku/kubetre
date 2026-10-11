package tre

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
	"github.com/isBioku/kubetre/internal/access"
	"github.com/isBioku/kubetre/internal/controller"
	"github.com/isBioku/kubetre/internal/costs"
)

// CostGroups are the TRE's resource groups, in lower case: the platform's, the workspaces'
// Azure resources, and the AKS node resource group whose cost is shared between workspaces.
type CostGroups struct {
	Core, Workspaces, Cluster string
}

// Tags the compositions put on workspace resources (crossplane).
const (
	tagWorkspace = "kubetre-workspace"
	tagService   = "kubetre-service"
	tagVM        = "kubetre-vm"
)

type costRow struct {
	Cost     float64 `json:"cost"`
	Currency string  `json:"currency"`
	Date     *string `json:"date"`
}

// amounts are costs by day (empty without daily granularity) and currency.
type amounts map[[2]string]float64

func (a amounts) add(o amounts, factor float64) {
	for k, v := range o {
		a[k] += v * factor
	}
}

func (a amounts) rows() []costRow {
	keys := make([][2]string, 0, len(a))
	for k := range a {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i][0] != keys[j][0] {
			return keys[i][0] < keys[j][0]
		}
		return keys[i][1] < keys[j][1]
	})
	out := []costRow{}
	for _, k := range keys {
		r := costRow{Cost: a[k], Currency: k[1]}
		if k[0] != "" {
			d := k[0]
			r.Date = &d
		}
		out = append(out, r)
	}
	return out
}

// costData is one period's costs, by resource group and by tag.
type costData struct {
	groups map[string]amounts
	tags   map[string]amounts // "key=value"
	// shares are each workspace's part of the cluster, from the capacity it reserves.
	shares map[string]float64
}

func (d *costData) tag(key, value string) amounts {
	if a, ok := d.tags[key+"="+value]; ok {
		return a
	}
	return amounts{}
}

func (d *costData) group(name string) amounts {
	if a, ok := d.groups[name]; ok {
		return a
	}
	return amounts{}
}

// workspace is a workspace's Azure resources plus its share of the cluster.
func (d *costData) workspace(ws string, g CostGroups) amounts {
	out := amounts{}
	out.add(d.tag(tagWorkspace, ws), 1)
	out.add(d.group(g.Cluster), d.shares[ws])
	return out
}

// core is everything else: the platform's resource group, the unreserved part of the cluster,
// and workspace resources Azure does not tag, such as VM OS disks.
func (d *costData) core(g CostGroups, workspaces []string) amounts {
	out := amounts{}
	out.add(d.group(g.Core), 1)
	used := 0.0
	for _, ws := range workspaces {
		used += d.shares[ws]
	}
	out.add(d.group(g.Cluster), 1-used)
	out.add(d.group(g.Workspaces), 1)
	for _, ws := range workspaces {
		out.add(d.tag(tagWorkspace, ws), -1)
	}
	for k, v := range out {
		if v < 0.000001 && v > -0.000001 {
			delete(out, k)
		}
	}
	return out
}

func parseCostQuery(r *http.Request) (costs.Query, error) {
	q := costs.Query{}
	switch g := r.URL.Query().Get("granularity"); g {
	case "", "None":
	case "Daily":
		q.Daily = true
	default:
		return q, fmt.Errorf("granularity must be None or Daily")
	}
	from, to := r.URL.Query().Get("from_date"), r.URL.Query().Get("to_date")
	if from == "" && to == "" {
		return q, nil
	}
	if from == "" || to == "" {
		return q, errors.New("from_date and to_date must be set together")
	}
	var err error
	if q.From, err = parseDate(from); err != nil {
		return q, fmt.Errorf("from_date: %w", err)
	}
	if q.To, err = parseDate(to); err != nil {
		return q, fmt.Errorf("to_date: %w", err)
	}
	if !q.From.Before(q.To) {
		return q, errors.New("to_date must be after from_date")
	}
	if q.To.Sub(q.From) > 366*24*time.Hour {
		return q, errors.New("the period must not be longer than a year")
	}
	return q, nil
}

func parseDate(s string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("%q is not an ISO 8601 date", s)
}

func (s *Server) loadCosts(ctx context.Context, q costs.Query) (*costData, error) {
	d := &costData{groups: map[string]amounts{}, tags: map[string]amounts{}}
	q.ByTag = false
	rows, err := s.Costs.Costs(ctx, q)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if d.groups[r.ResourceGroup] == nil {
			d.groups[r.ResourceGroup] = amounts{}
		}
		d.groups[r.ResourceGroup][[2]string{r.Date, r.Currency}] += r.Cost
	}
	q.ByTag = true
	rows, err = s.Costs.Costs(ctx, q)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r.TagKey == "" {
			continue
		}
		k := r.TagKey + "=" + r.TagValue
		if d.tags[k] == nil {
			d.tags[k] = amounts{}
		}
		d.tags[k][[2]string{r.Date, r.Currency}] += r.Cost
	}
	d.shares, err = s.clusterShares(ctx)
	return d, err
}

// clusterShares divides the cluster between workspaces by the CPU and memory their quotas
// show as requested now, out of what the nodes can allocate. It is an estimate: it uses
// today's requests for the whole period.
func (s *Server) clusterShares(ctx context.Context) (map[string]float64, error) {
	nodes := &corev1.NodeList{}
	if err := s.Client.List(ctx, nodes); err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	cpu, mem := k8sresource.Quantity{}, k8sresource.Quantity{}
	for _, n := range nodes.Items {
		cpu.Add(n.Status.Allocatable[corev1.ResourceCPU])
		mem.Add(n.Status.Allocatable[corev1.ResourceMemory])
	}
	shares := map[string]float64{}
	if cpu.IsZero() || mem.IsZero() {
		return shares, nil
	}
	quotas := &corev1.ResourceQuotaList{}
	if err := s.Client.List(ctx, quotas, client.HasLabels{controller.LabelWorkspace}); err != nil {
		return nil, fmt.Errorf("list quotas: %w", err)
	}
	total := 0.0
	for _, rq := range quotas.Items {
		ws := rq.Labels[controller.LabelWorkspace]
		usedCPU := rq.Status.Used[corev1.ResourceRequestsCPU]
		usedMem := rq.Status.Used[corev1.ResourceRequestsMemory]
		share := (usedCPU.AsApproximateFloat64()/cpu.AsApproximateFloat64() + usedMem.AsApproximateFloat64()/mem.AsApproximateFloat64()) / 2
		shares[ws] += share
		total += share
	}
	if total > 1 {
		for ws := range shares {
			shares[ws] /= total
		}
	}
	return shares, nil
}

func (s *Server) costError(w http.ResponseWriter, err error) {
	var throttled *costs.ThrottledError
	switch {
	case errors.Is(err, costs.ErrNotSupported):
		writeError(w, http.StatusNotFound, "Azure subscription doesn't support cost management")
	case errors.As(err, &throttled):
		w.Header().Set("Retry-After", strconv.Itoa(throttled.RetryAfter))
		message := "Too many requests to Azure cost management API. Please retry."
		if throttled.Status == http.StatusServiceUnavailable {
			message = "Azure Cost Management API is temporarily unavailable. Please retry."
		}
		writeJSON(w, throttled.Status, map[string]any{"error": map[string]any{
			"code": strconv.Itoa(throttled.Status), "message": message, "retry-after": strconv.Itoa(throttled.RetryAfter)}})
	default:
		s.internal(w, "query costs", err)
	}
}

// getCosts is GET /api/costs: the TRE's costs for TRE administrators.
func (s *Server) getCosts(w http.ResponseWriter, r *http.Request, id access.Identity) {
	if s.Costs == nil {
		writeError(w, http.StatusNotFound, "cost reporting is not configured in this TRE")
		return
	}
	if !s.isAdmin(id) {
		writeError(w, http.StatusForbidden, "Only TRE administrators can see the TRE's costs")
		return
	}
	q, err := parseCostQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	d, err := s.loadCosts(r.Context(), q)
	if err != nil {
		s.costError(w, err)
		return
	}
	wsList := &treV1.WorkspaceList{}
	if err := s.Client.List(r.Context(), wsList); err != nil {
		s.internal(w, "list workspaces", err)
		return
	}
	names := []string{}
	workspaces := []map[string]any{}
	for _, ws := range wsList.Items {
		names = append(names, ws.Name)
		workspaces = append(workspaces, map[string]any{"id": ws.Name, "name": ws.Spec.DisplayName, "costs": d.workspace(ws.Name, s.CostGroups).rows()})
	}
	shared := []map[string]any{}
	sharedList := &treV1.WorkspaceServiceList{}
	if err := s.Client.List(r.Context(), sharedList, client.InNamespace(controller.SharedNamespace)); err == nil {
		for _, svc := range sharedList.Items {
			shared = append(shared, map[string]any{"id": svc.Name, "name": svc.Spec.DisplayName, "costs": d.tag(tagService, svc.Name).rows()})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"core_services": d.core(s.CostGroups, names).rows(), "shared_services": shared, "workspaces": workspaces})
}

// getWorkspaceCosts is GET /api/workspaces/{ws}/costs, for its owners and TRE administrators.
func (s *Server) getWorkspaceCosts(w http.ResponseWriter, r *http.Request, id access.Identity) {
	if s.Costs == nil {
		writeError(w, http.StatusNotFound, "cost reporting is not configured in this TRE")
		return
	}
	ws, roles, ok := s.loadWorkspace(w, r, id)
	if !ok {
		return
	}
	if !roles[access.RoleOwner] && !s.isAdmin(id) {
		writeError(w, http.StatusForbidden, "Only workspace owners and TRE administrators can see a workspace's costs")
		return
	}
	q, err := parseCostQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	d, err := s.loadCosts(r.Context(), q)
	if err != nil {
		s.costError(w, err)
		return
	}
	items, err := s.listChildren(r, ws)
	if err != nil {
		s.internal(w, "list services", err)
		return
	}
	children := map[string][]map[string]any{}
	for _, it := range items {
		if it.Spec.ParentService == "" {
			continue
		}
		c := amounts{}
		c.add(d.tag(tagVM, it.Name), 1)
		c.add(d.tag(tagService, it.Name), 1)
		children[it.Spec.ParentService] = append(children[it.Spec.ParentService],
			map[string]any{"id": it.Name, "name": it.Spec.DisplayName, "costs": c.rows()})
	}
	services := []map[string]any{}
	for _, it := range items {
		if it.Spec.ParentService != "" {
			continue
		}
		urs := children[it.Name]
		if urs == nil {
			urs = []map[string]any{}
		}
		services = append(services, map[string]any{"id": it.Name, "name": it.Spec.DisplayName,
			"costs": d.tag(tagService, it.Name).rows(), "user_resources": urs})
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": ws.Name, "name": ws.Spec.DisplayName,
		"costs": d.workspace(ws.Name, s.CostGroups).rows(), "workspace_services": services})
}
