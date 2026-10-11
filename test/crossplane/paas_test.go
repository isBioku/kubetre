package crossplane

import (
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	ownerOID      = "11111111-1111-1111-1111-111111111111"
	researcherOID = "22222222-2222-2222-2222-222222222222"
)

func paasParams(extra map[string]any) map[string]any {
	p := map[string]any{
		"id": "abc123def4",
		"members": []any{
			map[string]any{"objectId": ownerOID, "email": "olu@example.com", "role": "owner"},
			map[string]any{"objectId": researcherOID, "email": "rita@example.com", "role": "researcher"},
		},
		"creator": map[string]any{"objectId": ownerOID, "email": "olu@example.com"},
	}
	for k, v := range extra {
		p[k] = v
	}
	return p
}

// observedIDs pretends Azure has created the named composed resources.
func observedIDs(names ...string) map[string]any {
	out := map[string]any{}
	for _, n := range names {
		out[n] = map[string]any{"resource": map[string]any{"status": map[string]any{"atProvider": map[string]any{
			"id": "/subscriptions/0/resourceGroups/rg/providers/x/" + n, "workspaceUrl": "adb-1.2.azuredatabricks.net"}}}}
	}
	return out
}

func fp(u *unstructured.Unstructured) map[string]any {
	m, _, _ := unstructured.NestedMap(u.Object, "spec", "forProvider")
	return m
}

func paasXR(kind string, params map[string]any) map[string]any {
	x := xr(kind, "svc1", params)
	x["metadata"].(map[string]any)["labels"] = map[string]any{"kubetre.io/service": "svc1"}
	return x
}

// roles collects role assignments as "principal=role".
func roles(got map[string]*unstructured.Unstructured) []string {
	var out []string
	for _, u := range got {
		if u.GetKind() == "RoleAssignment" {
			f := fp(u)
			out = append(out, f["principalId"].(string)[:8]+"="+f["roleDefinitionName"].(string))
		}
	}
	return out
}

func endpoints(got map[string]*unstructured.Unstructured) map[string]string {
	out := map[string]string{}
	for _, u := range got {
		if u.GetKind() == "PrivateEndpoint" {
			psc := fp(u)["privateServiceConnection"].(map[string]any)
			out[psc["subresourceNames"].([]any)[0].(string)] = psc["privateConnectionResourceId"].(string)
			sel, _, _ := unstructured.NestedStringMap(u.Object, "spec", "forProvider", "subnetIdSelector", "matchLabels")
			if sel["kubetre.io/role"] != "vm-subnet" || sel["kubetre.io/workspace"] != "study" {
				panic("private endpoints must be in the workspace's own VM subnet")
			}
		}
	}
	return out
}

func connection(t *testing.T, got map[string]*unstructured.Unstructured) (string, string) {
	t.Helper()
	c := got["connection"]
	if c == nil || c.GetKind() != "Service" || c.GetLabels()["kubetre.io/service"] != "svc1" {
		t.Fatalf("connection service = %v", c)
	}
	host, _, _ := unstructured.NestedString(c.Object, "spec", "externalName")
	return host, c.GetAnnotations()["kubetre.io/connection-uri"]
}

func TestSQLDatabaseComposition(t *testing.T) {
	params := paasParams(map[string]any{"skuName": "GP_S_Gen5_1", "maxSizeGb": float64(32), "databaseName": "research"})
	// Before the server exists there is nothing to connect a private endpoint to.
	first, _ := render(t, "composition-sqldatabase.yaml", paasXR("SQLDatabase", params), nil)
	if eps := endpoints(submit(t, first)); len(eps) != 0 {
		t.Fatalf("endpoint before the server exists: %v", eps)
	}
	composed, _ := render(t, "composition-sqldatabase.yaml", paasXR("SQLDatabase", params), observedIDs("server"))
	got := submit(t, composed)
	server := fp(got["server"])
	ad := server["azureadAdministrator"].(map[string]any)
	if server["publicNetworkAccessEnabled"] != false || ad["azureadAuthenticationOnly"] != true || ad["objectId"] != ownerOID {
		t.Fatalf("server = %v", server)
	}
	if fp(got["database"])["autoPauseDelayInMinutes"] == nil {
		t.Error("serverless databases pause when idle")
	}
	if eps := endpoints(got); eps["sqlServer"] == "" {
		t.Fatalf("endpoints = %v", eps)
	}
	if host, _ := connection(t, got); host != "sql-abc123def4.database.windows.net" {
		t.Fatalf("host = %s", host)
	}
}

func TestSQLDatabaseNeedsTheCreatorsObjectID(t *testing.T) {
	params := paasParams(map[string]any{"skuName": "S0", "maxSizeGb": float64(10), "databaseName": "research"})
	params["creator"] = map[string]any{"objectId": "", "email": "olu@example.com"}
	u := &unstructured.Unstructured{Object: paasXR("SQLDatabase", params)}
	if err := dryRunCreate(t, u); err == nil || !strings.Contains(err.Error(), "Entra administrator") {
		t.Fatalf("want a clear refusal, got %v", err)
	}
}

func TestMySQLDatabaseComposition(t *testing.T) {
	params := paasParams(map[string]any{"skuName": "B_Standard_B1ms", "storageGb": float64(20), "version": "8.0.21",
		"adminUsername": "treadmin", "passwordSecretName": "svc1-mysql", "databaseName": "research"})
	composed, _ := render(t, "composition-mysqldatabase.yaml", paasXR("MySQLDatabase", params), observedIDs("server"))
	got := submit(t, composed)
	server := fp(got["server"])
	if server["publicNetworkAccess"] != "Disabled" || server["administratorPasswordSecretRef"].(map[string]any)["name"] != "svc1-mysql" {
		t.Fatalf("server = %v", server)
	}
	if eps := endpoints(got); eps["mysqlServer"] == "" {
		t.Fatalf("endpoints = %v", eps)
	}
	if _, uri := connection(t, got); uri != "mysql://treadmin@mysql-abc123def4.mysql.database.azure.com:3306/research" {
		t.Fatalf("uri = %s", uri)
	}
}

func TestOpenAIServiceComposition(t *testing.T) {
	params := paasParams(map[string]any{"modelName": "gpt-4o-mini", "modelVersion": "2024-07-18", "deploymentSku": "GlobalStandard", "capacity": float64(10)})
	composed, _ := render(t, "composition-openaiservice.yaml", paasXR("OpenAIService", params), observedIDs("account"))
	got := submit(t, composed)
	acct := fp(got["account"])
	if acct["localAuthEnabled"] != false || acct["publicNetworkAccessEnabled"] != false || acct["kind"] != "OpenAI" {
		t.Fatalf("account = %v", acct)
	}
	if r := roles(got); len(r) != 2 || !strings.Contains(strings.Join(r, ","), "22222222=Cognitive Services OpenAI User") {
		t.Fatalf("roles = %v", r)
	}
	if eps := endpoints(got); eps["account"] == "" {
		t.Fatalf("endpoints = %v", eps)
	}
}

func TestHealthDataServiceComposition(t *testing.T) {
	composed, _ := render(t, "composition-healthdataservice.yaml", paasXR("HealthDataService", paasParams(map[string]any{"dicom": true})),
		observedIDs("workspace", "fhir", "dicom"))
	got := submit(t, composed)
	if got["fhir"] == nil || got["dicom"] == nil || fp(got["dicom"])["publicNetworkAccessEnabled"] != false {
		t.Fatalf("composed = %v", got)
	}
	if r := roles(got); len(r) != 4 {
		t.Fatalf("both members get FHIR and DICOM roles: %v", r)
	}
	if eps := endpoints(got); eps["healthcareworkspace"] == "" {
		t.Fatalf("endpoints = %v", eps)
	}
	without, _ := render(t, "composition-healthdataservice.yaml", paasXR("HealthDataService", paasParams(nil)), nil)
	for _, u := range without {
		if u.GetKind() == "HealthcareDICOMService" {
			t.Fatal("DICOM only when asked for")
		}
	}
}

func TestMLWorkspaceComposition(t *testing.T) {
	composed, _ := render(t, "composition-mlworkspace.yaml", paasXR("MLWorkspace", paasParams(nil)), observedIDs("workspace", "storage", "vault"))
	got := submit(t, composed)
	for _, n := range []string{"storage", "vault", "workspace"} {
		if fp(got[n])["publicNetworkAccessEnabled"] != false {
			t.Errorf("%s must have no public network access", n)
		}
	}
	if got["workspace"].GetLabels()["kubetre.io/service"] != "svc1" {
		t.Error("compute instances find the workspace by its service label")
	}
	eps := endpoints(got)
	for _, sub := range []string{"amlworkspace", "blob", "file", "vault"} {
		if eps[sub] == "" {
			t.Errorf("no %s endpoint: %v", sub, eps)
		}
	}
	if r := roles(got); len(r) != 6 {
		t.Fatalf("roles = %v", r)
	}
	if _, uri := connection(t, got); !strings.HasPrefix(uri, "https://ml.azure.com/?wsid=") {
		t.Fatalf("uri = %s", uri)
	}
}

func TestMLComputeInstanceComposition(t *testing.T) {
	params := paasParams(map[string]any{"parentService": "aml", "vmSize": "Standard_DS3_v2"})
	composed, _ := render(t, "composition-mlcomputeinstance.yaml", paasXR("MLComputeInstance", params), nil)
	got := submit(t, composed)
	ci := fp(got["instance"])
	if ci["nodePublicIpEnabled"] != false || ci["localAuthEnabled"] != false ||
		ci["assignToUser"].(map[string]any)["objectId"] != ownerOID {
		t.Fatalf("instance = %v", ci)
	}
	sel, _, _ := unstructured.NestedStringMap(got["instance"].Object, "spec", "forProvider", "machineLearningWorkspaceIdSelector", "matchLabels")
	if sel["kubetre.io/service"] != "aml" {
		t.Fatalf("workspace selector = %v", sel)
	}
}

func TestDatabricksWorkspaceComposition(t *testing.T) {
	params := paasParams(map[string]any{"subnets": []any{"10.240.1.0/26", "10.240.1.64/26"}, "firewallPriority": float64(1010)})
	first, _ := render(t, "composition-databricksworkspace.yaml", paasXR("DatabricksWorkspace", params), nil)
	got := submit(t, first)
	if got["workspace"] != nil {
		t.Fatal("the workspace waits for its subnets' security group associations")
	}
	if got["subnet-host"] == nil || got["subnet-container"] == nil || got["egress-rules"] == nil {
		t.Fatalf("composed = %v", got)
	}
	if p := fp(got["egress-rules"])["priority"]; p != float64(1010) && p != int64(1010) {
		t.Fatalf("firewall priority = %v", p)
	}
	composed, _ := render(t, "composition-databricksworkspace.yaml", paasXR("DatabricksWorkspace", params),
		observedIDs("nsg-host", "nsg-container", "workspace"))
	got = submit(t, composed)
	w := fp(got["workspace"])
	if w["publicNetworkAccessEnabled"] != false || w["networkSecurityGroupRulesRequired"] != "NoAzureDatabricksRules" ||
		w["customParameters"].(map[string]any)["noPublicIp"] != true {
		t.Fatalf("workspace = %v", w)
	}
	eps := endpoints(got)
	if eps["databricks_ui_api"] == "" || eps["browser_authentication"] == "" {
		t.Fatalf("endpoints = %v", eps)
	}
	if r := roles(got); len(r) != 1 || !strings.HasPrefix(r[0], "11111111=") {
		t.Fatalf("only owners become Databricks admins: %v", r)
	}
	if _, uri := connection(t, got); uri != "https://adb-1.2.azuredatabricks.net/" {
		t.Fatalf("uri = %s", uri)
	}
}

// The PaaS activation policy activates exactly the kinds the PaaS compositions add.
func TestPaaSActivationPolicyCoversItsKinds(t *testing.T) {
	cases := []struct {
		file, kind string
		params     map[string]any
		observed   []string
	}{
		{"composition-sqldatabase.yaml", "SQLDatabase", map[string]any{"skuName": "S0", "maxSizeGb": float64(10), "databaseName": "d"}, []string{"server"}},
		{"composition-mysqldatabase.yaml", "MySQLDatabase", map[string]any{"skuName": "B_Standard_B1ms", "storageGb": float64(20), "version": "8.0.21",
			"adminUsername": "a", "passwordSecretName": "s", "databaseName": "d"}, []string{"server"}},
		{"composition-openaiservice.yaml", "OpenAIService", map[string]any{"modelName": "m", "modelVersion": "1", "deploymentSku": "Standard", "capacity": float64(1)}, []string{"account"}},
		{"composition-healthdataservice.yaml", "HealthDataService", map[string]any{"dicom": true}, []string{"workspace", "fhir", "dicom"}},
		{"composition-mlworkspace.yaml", "MLWorkspace", nil, []string{"workspace", "storage", "vault"}},
		{"composition-mlcomputeinstance.yaml", "MLComputeInstance", map[string]any{"parentService": "p", "vmSize": "Standard_DS3_v2"}, nil},
		{"composition-databricksworkspace.yaml", "DatabricksWorkspace", map[string]any{"subnets": []any{"10.240.1.0/26", "10.240.1.64/26"}, "firewallPriority": float64(1010)},
			[]string{"nsg-host", "nsg-container", "workspace"}},
	}
	platform := map[string]bool{}
	list, _, _ := unstructured.NestedStringSlice(readYAML(filepath.Join(root, "crossplane", "install", "activation.yaml"))[0].Object, "spec", "activate")
	for _, a := range list {
		platform[a] = true
	}
	want := map[string]bool{}
	for _, c := range cases {
		composed, _ := render(t, c.file, paasXR(c.kind, paasParams(c.params)), observedIDs(c.observed...))
		for _, u := range composed {
			gvk := u.GroupVersionKind()
			if !strings.HasSuffix(gvk.Group, ".upbound.io") {
				continue
			}
			m, err := k8s.RESTMapper().RESTMapping(gvk.GroupKind(), gvk.Version)
			if err != nil {
				t.Fatalf("no CRD for %v: %v", gvk, err)
			}
			if name := m.Resource.Resource + "." + gvk.Group; !platform[name] {
				want[name] = true
			}
		}
	}
	got := map[string]bool{}
	paas, _, _ := unstructured.NestedStringSlice(readYAML(filepath.Join(root, "crossplane", "paas", "install", "activation.yaml"))[0].Object, "spec", "activate")
	for _, a := range paas {
		got[a] = true
	}
	for k := range want {
		if !got[k] {
			t.Errorf("composed kind %s is not activated", k)
		}
	}
	for k := range got {
		if !want[k] {
			t.Errorf("%s is activated but no PaaS composition creates it", k)
		}
	}
}
