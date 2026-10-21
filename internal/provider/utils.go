/*
Copyright 2024 Digitalis.IO.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package provider

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/logging"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var valsSecretGVR = k8sschema.GroupVersionResource{
	Group:    "digitalis.io",
	Version:  "v1",
	Resource: "valssecrets",
}

var valsSecretGVK = k8sschema.GroupVersionKind{
	Group:   "digitalis.io",
	Version: "v1",
	Kind:    "ValsSecret",
}

func GetValsSecret(ctx context.Context, client dynamic.Interface, secretName string, namespace string) (*ValsSecret, error) {
	obj, err := client.Resource(valsSecretGVR).Namespace(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	var secret ValsSecret
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), &secret); err != nil {
		return nil, err
	}

	return &secret, nil
}

// valsSecretObject renders the desired ValsSecret as an unstructured object.
func valsSecretObject(plan ValsSecretResourceModel) *unstructured.Unstructured {
	refs := make(map[string]interface{})
	for _, r := range plan.SecretRef {
		refs[r.Name] = map[string]interface{}{
			"ref":      r.Ref,
			"encoding": r.Encoding,
		}
	}

	templates := make(map[string]interface{})
	for _, r := range plan.Template {
		templates[r.Name] = r.Value
	}

	rollout := make([]interface{}, 0, len(plan.Rollout))
	for _, r := range plan.Rollout {
		rollout = append(rollout, map[string]interface{}{
			"kind": r.Kind,
			"name": r.Name,
		})
	}

	spec := map[string]interface{}{
		"name":     plan.Name.ValueString(),
		"ttl":      plan.Ttl.ValueInt64(),
		"type":     plan.Type.ValueString(),
		"data":     refs,
		"template": templates,
	}
	if len(rollout) > 0 {
		spec["rollout"] = rollout
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "digitalis.io/v1",
			"kind":       "ValsSecret",
			"metadata": map[string]interface{}{
				"name":      plan.Name.ValueString(),
				"namespace": plan.Namespace.ValueString(),
			},
			"spec": spec,
		},
	}
	obj.SetGroupVersionKind(valsSecretGVK)
	return obj
}

func CreateValsSecret(ctx context.Context, client dynamic.Interface, plan ValsSecretResourceModel) (*ValsSecret, error) {
	name := plan.Name.ValueString()
	namespace := plan.Namespace.ValueString()

	obj := valsSecretObject(plan)
	printDebug("creating valssecret", namespace, name)
	out, err := client.Resource(valsSecretGVR).Namespace(namespace).Create(ctx, obj, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("valssecret %q already exists in namespace %q; import it with `terraform import` instead of creating it", name, namespace)
	}
	if err != nil {
		return nil, err
	}

	var secret ValsSecret
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(out.UnstructuredContent(), &secret); err != nil {
		return nil, err
	}
	return &secret, nil
}

func UpdateValsSecret(ctx context.Context, client dynamic.Interface, plan ValsSecretResourceModel) (*ValsSecret, error) {
	name := plan.Name.ValueString()
	namespace := plan.Namespace.ValueString()

	current, err := client.Resource(valsSecretGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	obj := valsSecretObject(plan)
	// Preserve the resource version and any metadata (labels/annotations)
	// managed by other controllers so the update does not clobber them.
	obj.SetResourceVersion(current.GetResourceVersion())
	obj.SetLabels(current.GetLabels())
	obj.SetAnnotations(current.GetAnnotations())

	printDebug("updating valssecret", namespace, name)
	out, err := client.Resource(valsSecretGVR).Namespace(namespace).Update(ctx, obj, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}

	var secret ValsSecret
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(out.UnstructuredContent(), &secret); err != nil {
		return nil, err
	}
	return &secret, nil
}

var dbSecretGVR = k8sschema.GroupVersionResource{
	Group:    "digitalis.io",
	Version:  "v1beta1",
	Resource: "dbsecrets",
}

var dbSecretGVK = k8sschema.GroupVersionKind{
	Group:   "digitalis.io",
	Version: "v1beta1",
	Kind:    "DbSecret",
}

func GetDbSecret(ctx context.Context, client dynamic.Interface, name string, namespace string) (*DbSecret, error) {
	obj, err := client.Resource(dbSecretGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	var secret DbSecret
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), &secret); err != nil {
		return nil, err
	}

	return &secret, nil
}

// dbSecretObject renders the desired DbSecret as an unstructured object.
func dbSecretObject(plan DbSecretResourceModel) (*unstructured.Unstructured, error) {
	// Build vault config - required, take first block entry
	if len(plan.Vault) == 0 {
		return nil, fmt.Errorf("vault block is required")
	}
	vault := map[string]interface{}{
		"role":  plan.Vault[0].Role,
		"mount": plan.Vault[0].Mount,
	}

	spec := map[string]interface{}{
		"vault": vault,
		"renew": plan.Renew.ValueBool(),
	}

	if !plan.SecretName.IsNull() && plan.SecretName.ValueString() != "" {
		spec["secretName"] = plan.SecretName.ValueString()
	}

	if len(plan.Template) > 0 {
		templates := make(map[string]interface{})
		for _, r := range plan.Template {
			templates[r.Name] = r.Value
		}
		spec["template"] = templates
	}

	if len(plan.Rollout) > 0 {
		rollout := make([]interface{}, 0, len(plan.Rollout))
		for _, r := range plan.Rollout {
			rollout = append(rollout, map[string]interface{}{
				"kind": r.Kind,
				"name": r.Name,
			})
		}
		spec["rollout"] = rollout
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "digitalis.io/v1beta1",
			"kind":       "DbSecret",
			"metadata": map[string]interface{}{
				"name":      plan.Name.ValueString(),
				"namespace": plan.Namespace.ValueString(),
			},
			"spec": spec,
		},
	}
	obj.SetGroupVersionKind(dbSecretGVK)
	return obj, nil
}

func CreateDbSecret(ctx context.Context, client dynamic.Interface, plan DbSecretResourceModel) (*DbSecret, error) {
	name := plan.Name.ValueString()
	namespace := plan.Namespace.ValueString()

	obj, err := dbSecretObject(plan)
	if err != nil {
		return nil, err
	}

	printDebug("creating dbsecret", namespace, name)
	out, err := client.Resource(dbSecretGVR).Namespace(namespace).Create(ctx, obj, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("dbsecret %q already exists in namespace %q; import it with `terraform import` instead of creating it", name, namespace)
	}
	if err != nil {
		return nil, err
	}

	var secret DbSecret
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(out.UnstructuredContent(), &secret); err != nil {
		return nil, err
	}
	return &secret, nil
}

func UpdateDbSecret(ctx context.Context, client dynamic.Interface, plan DbSecretResourceModel) (*DbSecret, error) {
	name := plan.Name.ValueString()
	namespace := plan.Namespace.ValueString()

	obj, err := dbSecretObject(plan)
	if err != nil {
		return nil, err
	}

	current, err := client.Resource(dbSecretGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	// Preserve the resource version and metadata managed by other controllers.
	obj.SetResourceVersion(current.GetResourceVersion())
	obj.SetLabels(current.GetLabels())
	obj.SetAnnotations(current.GetAnnotations())

	printDebug("updating dbsecret", namespace, name)
	out, err := client.Resource(dbSecretGVR).Namespace(namespace).Update(ctx, obj, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}

	var secret DbSecret
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(out.UnstructuredContent(), &secret); err != nil {
		return nil, err
	}
	return &secret, nil
}

func DeleteDbSecret(ctx context.Context, client dynamic.Interface, name string, namespace string) error {
	return client.Resource(dbSecretGVR).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func DeleteValsSecret(ctx context.Context, client dynamic.Interface, secretName string, namespace string) error {
	gvr := k8sschema.GroupVersionResource{
		Group:    "digitalis.io",
		Version:  "v1",
		Resource: "valssecrets",
	}
	return client.Resource(gvr).Namespace(namespace).Delete(ctx, secretName, metav1.DeleteOptions{})
}

// orderedKeys returns the keys of present ordered so that keys named in prior
// keep their original position, followed by any new keys in sorted order. This
// keeps map-backed nested blocks stable across reads and avoids perpetual diffs.
func orderedKeys(prior []string, present map[string]bool) []string {
	result := make([]string, 0, len(present))
	seen := make(map[string]bool, len(prior))

	for _, name := range prior {
		if present[name] && !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}

	added := make([]string, 0)
	for name := range present {
		if !seen[name] {
			added = append(added, name)
		}
	}
	sort.Strings(added)
	return append(result, added...)
}

func keyPresence[V any](m map[string]V) map[string]bool {
	present := make(map[string]bool, len(m))
	for k := range m {
		present[k] = true
	}
	return present
}

func priorNames[T any](prior []T, name func(T) string) []string {
	names := make([]string, len(prior))
	for i, p := range prior {
		names[i] = name(p)
	}
	return names
}

// refreshSecretRefs reconciles the secret_ref blocks in state against the live
// data map, preserving existing order and appending new keys sorted.
func refreshSecretRefs(prior []ValsSecretReference, data map[string]DataSource) []ValsSecretReference {
	keys := orderedKeys(priorNames(prior, func(r ValsSecretReference) string { return r.Name }), keyPresence(data))
	if len(keys) == 0 {
		return nil
	}
	result := make([]ValsSecretReference, 0, len(keys))
	for _, name := range keys {
		result = append(result, ValsSecretReference{
			Name:     name,
			Ref:      data[name].Ref,
			Encoding: data[name].Encoding,
		})
	}
	return result
}

// refreshTemplates reconciles valssecret template blocks against the live map.
func refreshTemplates(prior []ValsSecretTemplate, tmpl map[string]string) []ValsSecretTemplate {
	keys := orderedKeys(priorNames(prior, func(t ValsSecretTemplate) string { return t.Name }), keyPresence(tmpl))
	if len(keys) == 0 {
		return nil
	}
	result := make([]ValsSecretTemplate, 0, len(keys))
	for _, name := range keys {
		result = append(result, ValsSecretTemplate{Name: name, Value: tmpl[name]})
	}
	return result
}

// refreshDbTemplates reconciles dbsecret template blocks against the live map.
func refreshDbTemplates(prior []DbSecretTemplateModel, tmpl map[string]string) []DbSecretTemplateModel {
	keys := orderedKeys(priorNames(prior, func(t DbSecretTemplateModel) string { return t.Name }), keyPresence(tmpl))
	if len(keys) == 0 {
		return nil
	}
	result := make([]DbSecretTemplateModel, 0, len(keys))
	for _, name := range keys {
		result = append(result, DbSecretTemplateModel{Name: name, Value: tmpl[name]})
	}
	return result
}

// splitNamespacedName parses a "namespace/name" import identifier.
func splitNamespacedName(id string) (namespace, name string, err error) {
	namespace, name, ok := strings.Cut(id, "/")
	if !ok || namespace == "" || name == "" {
		return "", "", fmt.Errorf("expected import identifier in the form %q, got: %q", "namespace/name", id)
	}
	return namespace, name, nil
}

func printDebug(msg ...any) {
	if logging.IsDebugOrHigher() {
		log.Println(msg...)
	}
}
