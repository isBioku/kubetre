package controller

import (
	"bytes"
	"context"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// applySpec creates obj, or updates the live object's spec and labels when they differ.
// It is used for resources whose Go types KubeTRE does not import (Cilium, Flux).
// On return obj holds the live object, including its status.
func applySpec(ctx context.Context, c client.Client, obj *unstructured.Unstructured) error {
	live := &unstructured.Unstructured{}
	live.SetGroupVersionKind(obj.GroupVersionKind())
	err := c.Get(ctx, types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, live)
	if apierrors.IsNotFound(err) {
		return c.Create(ctx, obj)
	}
	if err != nil {
		return err
	}
	if !equality.Semantic.DeepEqual(live.Object["spec"], obj.Object["spec"]) ||
		!equality.Semantic.DeepEqual(live.GetLabels(), obj.GetLabels()) {
		live.Object["spec"] = obj.Object["spec"]
		live.SetLabels(obj.GetLabels())
		live.SetOwnerReferences(obj.GetOwnerReferences())
		if err := c.Update(ctx, live); err != nil {
			return err
		}
	}
	obj.Object = live.Object
	return nil
}

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }

// applyParameters creates obj, or updates only spec.parameters and labels on the live object.
// Crossplane composite resources carry spec.crossplane, which Crossplane owns; replacing the
// whole spec would fight it.
func applyParameters(ctx context.Context, c client.Client, obj *unstructured.Unstructured) error {
	live := &unstructured.Unstructured{}
	live.SetGroupVersionKind(obj.GroupVersionKind())
	err := c.Get(ctx, types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, live)
	if apierrors.IsNotFound(err) {
		return c.Create(ctx, obj)
	}
	if err != nil {
		return err
	}
	want, _, _ := unstructured.NestedMap(obj.Object, "spec", "parameters")
	have, _, _ := unstructured.NestedMap(live.Object, "spec", "parameters")
	if !equality.Semantic.DeepEqual(want, have) || !equality.Semantic.DeepEqual(live.GetLabels(), obj.GetLabels()) {
		if err := unstructured.SetNestedMap(live.Object, want, "spec", "parameters"); err != nil {
			return err
		}
		live.SetLabels(obj.GetLabels())
		if err := c.Update(ctx, live); err != nil {
			return err
		}
	}
	obj.Object = live.Object
	return nil
}
