package controller

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
)

// Labels and names shared by everything the controller creates.
const (
	LabelWorkspace      = "kubetre.io/workspace"
	LabelManagedBy      = "app.kubernetes.io/managed-by"
	ManagedByValue      = "kubetre"
	LabelSharedServices = "kubetre.io/shared-services"
	LabelIngress        = "kubetre.io/ingress"

	NamespacePrefix = "ws-"
	QuotaName       = "kubetre-quota"
	LimitRangeName  = "kubetre-defaults"
	EgressPolicy    = "kubetre-egress-fqdn"

	// DeployerName is the per-workspace service account Flux impersonates to install charts.
	DeployerName = "kubetre-deployer"
	// DeployerClusterRole is the namespace-scoped permission set granted to the deployer.
	DeployerClusterRole = "kubetre-workspace-deployer"
)

// NamespaceFor returns the namespace that backs a workspace.
func NamespaceFor(ws *treV1.Workspace) string { return NamespacePrefix + ws.Name }

func commonLabels(ws *treV1.Workspace) map[string]string {
	return map[string]string{LabelWorkspace: ws.Name, LabelManagedBy: ManagedByValue}
}

// LabelPodSecurity is the namespace label Kubernetes uses to enforce Pod Security Standards.
const LabelPodSecurity = "pod-security.kubernetes.io/enforce"

// namespaceLabels enforces the template's Pod Security Standard, restricted by default.
// Warnings are always at "restricted" so loosened workspaces still surface risky pods.
func namespaceLabels(ws *treV1.Workspace, tmpl *treV1.WorkspaceTemplate) map[string]string {
	l := commonLabels(ws)
	l[LabelPodSecurity] = string(tmpl.Spec.PodSecurity.OrDefault())
	l["pod-security.kubernetes.io/enforce-version"] = "latest"
	l["pod-security.kubernetes.io/warn"] = "restricted"
	return l
}

// defaultLimits lets pods without explicit resources run under a ResourceQuota.
func defaultLimits() []corev1.LimitRangeItem {
	return []corev1.LimitRangeItem{{
		Type: corev1.LimitTypeContainer,
		DefaultRequest: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("250m"),
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		},
		Default: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		},
	}}
}

// desiredNetworkPolicies returns the baseline policies for a workspace namespace.
// The model is default-deny: only traffic listed here, or in the FQDN egress policy, flows.
func desiredNetworkPolicies(ws *treV1.Workspace) []networkingv1.NetworkPolicy {
	ns := NamespaceFor(ws)
	all := metav1.LabelSelector{}
	udp, tcp := corev1.ProtocolUDP, corev1.ProtocolTCP
	dnsPort := intstr.FromInt32(53)
	meta := func(name string) metav1.ObjectMeta {
		return metav1.ObjectMeta{Name: name, Namespace: ns, Labels: commonLabels(ws)}
	}
	nsSelector := func(k, v string) *metav1.LabelSelector {
		return &metav1.LabelSelector{MatchLabels: map[string]string{k: v}}
	}
	return []networkingv1.NetworkPolicy{
		{
			ObjectMeta: meta("kubetre-default-deny"),
			Spec: networkingv1.NetworkPolicySpec{
				PodSelector: all,
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress},
			},
		},
		{
			ObjectMeta: meta("kubetre-allow-same-namespace"),
			Spec: networkingv1.NetworkPolicySpec{
				PodSelector: all,
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress},
				Ingress:     []networkingv1.NetworkPolicyIngressRule{{From: []networkingv1.NetworkPolicyPeer{{PodSelector: &all}}}},
				Egress:      []networkingv1.NetworkPolicyEgressRule{{To: []networkingv1.NetworkPolicyPeer{{PodSelector: &all}}}},
			},
		},
		{
			ObjectMeta: meta("kubetre-allow-dns"),
			Spec: networkingv1.NetworkPolicySpec{
				PodSelector: all,
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
				Egress: []networkingv1.NetworkPolicyEgressRule{{
					To: []networkingv1.NetworkPolicyPeer{{
						NamespaceSelector: nsSelector("kubernetes.io/metadata.name", "kube-system"),
						PodSelector:       &metav1.LabelSelector{MatchLabels: map[string]string{"k8s-app": "kube-dns"}},
					}},
					Ports: []networkingv1.NetworkPolicyPort{{Protocol: &udp, Port: &dnsPort}, {Protocol: &tcp, Port: &dnsPort}},
				}},
			},
		},
		{
			ObjectMeta: meta("kubetre-allow-shared-services"),
			Spec: networkingv1.NetworkPolicySpec{
				PodSelector: all,
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
				Egress: []networkingv1.NetworkPolicyEgressRule{{
					To: []networkingv1.NetworkPolicyPeer{{NamespaceSelector: nsSelector(LabelSharedServices, "true")}},
				}},
			},
		},
		{
			// Only the access gateway's guacd pods may open sessions to workspace desktops.
			ObjectMeta: meta("kubetre-allow-ingress-gateway"),
			Spec: networkingv1.NetworkPolicySpec{
				PodSelector: all,
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
				Ingress: []networkingv1.NetworkPolicyIngressRule{{
					From: []networkingv1.NetworkPolicyPeer{{
						NamespaceSelector: nsSelector(LabelIngress, "true"),
						PodSelector:       &metav1.LabelSelector{MatchLabels: map[string]string{"app.kubernetes.io/name": "guacd"}},
					}},
				}},
			},
		},
	}
}

// AllowedFQDNs merges the template and workspace allowlists, sorted and de-duplicated.
func AllowedFQDNs(tmpl *treV1.WorkspaceTemplate, ws *treV1.Workspace) []string {
	seen := map[string]bool{}
	var out []string
	for _, list := range [][]string{tmpl.Spec.Egress.AllowedFQDNs, ws.Spec.Egress.AllowedFQDNs} {
		for _, f := range list {
			f = strings.ToLower(strings.TrimSpace(f))
			if f != "" && !seen[f] {
				seen[f] = true
				out = append(out, f)
			}
		}
	}
	sortStrings(out)
	return out
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// desiredCiliumPolicy builds a CiliumNetworkPolicy allowing HTTPS to the given FQDNs.
// Cilium needs to see DNS answers to enforce toFQDNs, so DNS to kube-dns is proxied.
func desiredCiliumPolicy(ws *treV1.Workspace, fqdns []string) *unstructured.Unstructured {
	selectors := make([]any, 0, len(fqdns))
	for _, f := range fqdns {
		if strings.Contains(f, "*") {
			selectors = append(selectors, map[string]any{"matchPattern": f})
		} else {
			selectors = append(selectors, map[string]any{"matchName": f})
		}
	}
	u := &unstructured.Unstructured{}
	u.SetAPIVersion("cilium.io/v2")
	u.SetKind("CiliumNetworkPolicy")
	u.SetName(EgressPolicy)
	u.SetNamespace(NamespaceFor(ws))
	u.SetLabels(commonLabels(ws))
	u.Object["spec"] = map[string]any{
		"endpointSelector": map[string]any{},
		"egress": []any{
			map[string]any{
				"toEndpoints": []any{map[string]any{"matchLabels": map[string]any{
					"k8s:io.kubernetes.pod.namespace": "kube-system",
					"k8s:k8s-app":                     "kube-dns",
				}}},
				"toPorts": []any{map[string]any{
					"ports": []any{map[string]any{"port": "53", "protocol": "ANY"}},
					"rules": map[string]any{"dns": []any{map[string]any{"matchPattern": "*"}}},
				}},
			},
			map[string]any{
				"toFQDNs": selectors,
				"toPorts": []any{map[string]any{
					"ports": []any{map[string]any{"port": "443", "protocol": "TCP"}},
				}},
			},
		},
	}
	return u
}
