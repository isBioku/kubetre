// Package xrdcrd builds, for tests, the CRD that Crossplane generates from an XRD.
package xrdcrd

import (
	"os"
	"strings"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// FromFile converts the first served version of an XRD file into a namespaced CRD,
// adding the fields Crossplane manages (spec.crossplane and status.conditions).
func FromFile(path string) *apiextensionsv1.CustomResourceDefinition {
	raw, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	xrd := &unstructured.Unstructured{}
	if err := yaml.Unmarshal(raw, &xrd.Object); err != nil {
		panic(err)
	}
	group, _, _ := unstructured.NestedString(xrd.Object, "spec", "group")
	kind, _, _ := unstructured.NestedString(xrd.Object, "spec", "names", "kind")
	plural, _, _ := unstructured.NestedString(xrd.Object, "spec", "names", "plural")
	versions, _, _ := unstructured.NestedSlice(xrd.Object, "spec", "versions")
	v := versions[0].(map[string]any)
	schemaRaw, _ := yaml.Marshal(v["schema"].(map[string]any)["openAPIV3Schema"])
	schema := &apiextensionsv1.JSONSchemaProps{}
	if err := yaml.Unmarshal(schemaRaw, schema); err != nil {
		panic(err)
	}
	t := true
	spec := schema.Properties["spec"]
	spec.Properties["crossplane"] = apiextensionsv1.JSONSchemaProps{Type: "object", XPreserveUnknownFields: &t}
	schema.Properties["spec"] = spec
	status := schema.Properties["status"]
	status.Properties["conditions"] = apiextensionsv1.JSONSchemaProps{Type: "array", Items: &apiextensionsv1.JSONSchemaPropsOrArray{
		Schema: &apiextensionsv1.JSONSchemaProps{Type: "object", XPreserveUnknownFields: &t}}}
	schema.Properties["status"] = status
	schema.Properties["apiVersion"] = apiextensionsv1.JSONSchemaProps{Type: "string"}
	schema.Properties["kind"] = apiextensionsv1.JSONSchemaProps{Type: "string"}
	schema.Properties["metadata"] = apiextensionsv1.JSONSchemaProps{Type: "object"}
	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: plural + "." + group},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: group, Scope: apiextensionsv1.NamespaceScoped,
			Names: apiextensionsv1.CustomResourceDefinitionNames{Kind: kind, Plural: plural, ListKind: kind + "List", Singular: strings.ToLower(kind)},
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
				Name: v["name"].(string), Served: true, Storage: true,
				Schema:       &apiextensionsv1.CustomResourceValidation{OpenAPIV3Schema: schema},
				Subresources: &apiextensionsv1.CustomResourceSubresources{Status: &apiextensionsv1.CustomResourceSubresourceStatus{}},
			}},
		},
	}
}
