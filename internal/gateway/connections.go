package gateway

import (
	"context"
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
	"github.com/isBioku/kubetre/internal/access"
	"github.com/isBioku/kubetre/internal/controller"
)

// Labels and annotations of the connection convention. A chart that offers a remote
// session creates a Secret with LabelConnection and keys protocol, port, password,
// optionally username, and either hostname or AnnotationHostFromVM.
const (
	LabelConnection      = "kubetre.io/connection"
	LabelService         = "kubetre.io/service"
	AnnotationHostFromVM = "kubetre.io/host-from-researchvm"
	// AnnotationHelmRelease is set by Helm itself on every object it installs; a chart cannot
	// override it. KubeTRE names each Helm release after its WorkspaceService.
	AnnotationHelmRelease = "meta.helm.sh/release-name"
)

var researchVMGVK = schema.GroupVersionKind{Group: "platform.kubetre.io", Version: "v1alpha1", Kind: "ResearchVM"}

// Connection is a remote session the user may open.
type Connection struct {
	Workspace   string
	Name        string // the connection Secret's name, unique within the workspace
	Service     string
	DisplayName string
	Protocol    string
	Hostname    string
	Port        int
	Username    string
	Password    string
	Ready       bool
	Reason      string // why it is not ready
}

// ID identifies the connection in URLs and in the Guacamole payload.
func (c Connection) ID() string { return c.Workspace + "/" + c.Name }

// Resolver finds the connections a user may open.
type Resolver struct {
	Client client.Reader
}

// Connections returns the user's own personal sessions in workspaces they belong to.
// Ownership comes from WorkspaceService.spec.owner, which only the API sets, never from
// anything a chart writes. Shared services are not exposed through the gateway.
func (r *Resolver) Connections(ctx context.Context, id access.Identity) ([]Connection, error) {
	workspaces := &treV1.WorkspaceList{}
	if err := r.Client.List(ctx, workspaces); err != nil {
		return nil, err
	}
	var out []Connection
	for i := range workspaces.Items {
		ws := &workspaces.Items[i]
		if !access.IsMember(ws, id) || ws.Status.Phase != treV1.PhaseReady {
			continue
		}
		conns, err := r.workspaceConnections(ctx, ws, id)
		if err != nil {
			return nil, fmt.Errorf("workspace %s: %w", ws.Name, err)
		}
		out = append(out, conns...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out, nil
}

func (r *Resolver) workspaceConnections(ctx context.Context, ws *treV1.Workspace, id access.Identity) ([]Connection, error) {
	ns := controller.NamespaceFor(ws)
	services := &treV1.WorkspaceServiceList{}
	if err := r.Client.List(ctx, services, client.InNamespace(ns)); err != nil {
		return nil, err
	}
	owned := map[string]*treV1.WorkspaceService{}
	for i := range services.Items {
		s := &services.Items[i]
		if s.DeletionTimestamp.IsZero() && s.Spec.Owner != "" && id.Matches(s.Spec.Owner) {
			owned[s.Name] = s
		}
	}
	if len(owned) == 0 {
		return nil, nil
	}

	secrets := &corev1.SecretList{}
	if err := r.Client.List(ctx, secrets, client.InNamespace(ns), client.MatchingLabels{LabelConnection: "true"}); err != nil {
		return nil, err
	}
	var out []Connection
	for i := range secrets.Items {
		sec := &secrets.Items[i]
		// The owning service comes from Helm's release annotation, not from a chart-written
		// label, so one service's chart cannot publish a connection for another's owner.
		release := sec.Annotations[AnnotationHelmRelease]
		if release == "" || sec.Labels[LabelService] != release {
			continue
		}
		svc, ok := owned[release]
		if !ok {
			continue
		}
		c := Connection{
			Workspace: ws.Name, Name: sec.Name, Service: svc.Name, DisplayName: svc.Spec.DisplayName,
			Protocol: string(sec.Data["protocol"]), Username: string(sec.Data["username"]), Password: string(sec.Data["password"]),
		}
		if err := r.resolveTarget(ctx, ws, sec, &c); err != nil {
			c.Reason = err.Error()
		} else {
			c.Ready = true
		}
		out = append(out, c)
	}
	return out, nil
}

// resolveTarget validates protocol and port, and resolves the host. A target must lie inside
// the same workspace: a Service in its namespace, or an address in its VM subnet. A chart
// therefore cannot point the gateway at another workspace or anywhere else.
func (r *Resolver) resolveTarget(ctx context.Context, ws *treV1.Workspace, sec *corev1.Secret, c *Connection) error {
	switch c.Protocol {
	case "rdp", "ssh", "vnc":
	default:
		return fmt.Errorf("unsupported protocol %q", c.Protocol)
	}
	port, err := strconv.Atoi(string(sec.Data["port"]))
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid port %q", sec.Data["port"])
	}
	c.Port = port

	if vmName := sec.Annotations[AnnotationHostFromVM]; vmName != "" {
		vm := &unstructured.Unstructured{}
		vm.SetGroupVersionKind(researchVMGVK)
		if err := r.Client.Get(ctx, types.NamespacedName{Namespace: sec.Namespace, Name: vmName}, vm); err != nil {
			if apierrors.IsNotFound(err) {
				return fmt.Errorf("virtual machine %s not found", vmName)
			}
			return err
		}
		ipText, _, _ := unstructured.NestedString(vm.Object, "status", "privateIp")
		if ipText == "" {
			return fmt.Errorf("waiting for the virtual machine's address")
		}
		ip, err := netip.ParseAddr(ipText)
		if err != nil {
			return fmt.Errorf("virtual machine reported an invalid address")
		}
		if ws.Status.VMNetwork == nil {
			return fmt.Errorf("workspace has no VM network")
		}
		subnet, err := netip.ParsePrefix(ws.Status.VMNetwork.AddressPrefix)
		if err != nil || !subnet.Contains(ip) {
			return fmt.Errorf("virtual machine address is outside the workspace subnet")
		}
		c.Hostname = ip.String()
		return nil
	}

	host := string(sec.Data["hostname"])
	suffix := "." + sec.Namespace + ".svc.cluster.local"
	name := strings.TrimSuffix(host, suffix)
	if !strings.HasSuffix(host, suffix) || name == "" || strings.Contains(name, ".") {
		return fmt.Errorf("hostname must be a Service in the workspace namespace")
	}
	c.Hostname = host
	return nil
}

// HardenedParameters returns Guacamole connection parameters with every channel that could
// move data in or out of the workspace switched off: clipboard both ways, drive mapping,
// file transfer, SFTP, printing and audio input.
func HardenedParameters(c Connection) map[string]string {
	p := map[string]string{
		"hostname":      c.Hostname,
		"port":          strconv.Itoa(c.Port),
		"disable-copy":  "true",
		"disable-paste": "true",
		"enable-sftp":   "false",
	}
	if c.Username != "" {
		p["username"] = c.Username
	}
	if c.Password != "" {
		p["password"] = c.Password
	}
	if c.Protocol == "rdp" {
		p["enable-drive"] = "false"
		p["disable-download"] = "true"
		p["disable-upload"] = "true"
		p["enable-printing"] = "false"
		p["enable-audio-input"] = "false"
		// VMs present self-signed RDP certificates; traffic stays on the private network.
		p["ignore-cert"] = "true"
		p["security"] = "nla"
	}
	return p
}
