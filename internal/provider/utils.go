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
	"encoding/json"
	"fmt"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/logging"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

func GetValsSecret(ctx context.Context, client dynamic.Interface, secretName string, namespace string) (*ValsSecret, error) {
	var secret *ValsSecret

	// Define the GVR (Group-Version-Resource) for the custom resource
	gvr := k8sschema.GroupVersionResource{
		Group:    "digitalis.io",
		Version:  "v1",
		Resource: "valssecrets",
	}

	obj, err := client.Resource(gvr).Namespace(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return secret, err
	}

	err = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), &secret)
	if err != nil {
		return secret, err
	}

	return secret, nil
}

func CreateValsSecret(ctx context.Context, client dynamic.Interface, plan ValsSecretResourceModel) (*ValsSecret, error) {
	// Define the GVR (Group-Version-Resource) for the custom resource
	gvr := k8sschema.GroupVersionResource{
		Group:    "digitalis.io",
		Version:  "v1",
		Resource: "valssecrets",
	}
	gkr := k8sschema.GroupVersionKind{
		Group:   "digitalis.io",
		Version: "v1",
		Kind:    "ValsSecret",
	}
	refs := make(map[string]interface{})
	for _, r := range plan.SecretRef {
		refs[r.Name] = map[string]interface{}{
			"ref":      r.Ref,
			"encoding": r.Encoding,
		}
	}

	templates := make(map[string]string)
	for _, r := range plan.Template {
		templates[r.Name] = r.Value
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "digitalis.io/v1",
			"kind":       "ValsSecret",
			"metadata": map[string]interface{}{
				"name":      plan.Name.ValueString(),
				"namespace": plan.Namespace.ValueString(),
			},
			"spec": map[string]interface{}{
				"name":     plan.Name.ValueString(),
				"ttl":      plan.Ttl.ValueInt64(),
				"type":     plan.Type.ValueString(),
				"data":     refs,
				"template": templates,
			},
		},
	}

	log.Println(prettyPrint(obj.UnstructuredContent()))

	obj.SetGroupVersionKind(gkr)

	var secret *ValsSecret
	var err error

	secret, err = GetValsSecret(ctx, client, plan.Name.ValueString(), plan.Namespace.ValueString())
	printDebug("[DEBUG] GetValsSecret error", err)
	if err != nil && !errors.IsNotFound(err) {
		return secret, err
	}

	if secret == nil || secret.GetName() == "" {
		printDebug("[DEBUG] CreateValsSecret, creating new secret", plan.Name.ValueString(), plan.Namespace.ValueString())
		out, err := client.Resource(gvr).Namespace(plan.Namespace.ValueString()).Create(ctx, obj, metav1.CreateOptions{})
		if err != nil {
			return secret, err
		}
		log.Println(prettyPrint(out.UnstructuredContent()))
	} else {
		printDebug("[DEBUG] Update secret", plan.Name.ValueString(), plan.Namespace.ValueString())
		obj.SetResourceVersion(secret.GetResourceVersion())
		_, err = client.Resource(gvr).Namespace(plan.Namespace.ValueString()).Update(ctx, obj, metav1.UpdateOptions{})
		if err != nil {
			return secret, err
		}
	}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), &secret)
	if err != nil {
		return secret, err
	}

	return secret, nil
}

func DeleteValsSecret(ctx context.Context, client dynamic.Interface, secretName string, namespace string) error {
	gvr := k8sschema.GroupVersionResource{
		Group:    "digitalis.io",
		Version:  "v1",
		Resource: "valssecrets",
	}
	return client.Resource(gvr).Namespace(namespace).Delete(ctx, secretName, metav1.DeleteOptions{})
}

func prettyPrint(obj map[string]interface{}) string {
	b, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		fmt.Println("error:", err)
		return fmt.Sprintf("%v", err)
	}
	return string(b)
}

func printDebug(msg ...any) {
	if logging.IsDebugOrHigher() {
		log.Println(msg...)
	}
}
