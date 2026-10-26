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
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	restclient "k8s.io/client-go/rest"
)

func newFakeDynamicClient() dynamic.Interface {
	gvrToListKind := map[k8sschema.GroupVersionResource]string{
		valsSecretGVR: "ValsSecretList",
		dbSecretGVR:   "DbSecretList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gvrToListKind)
}

func sampleValsPlan() ValsSecretResourceModel {
	return ValsSecretResourceModel{
		Name:      types.StringValue("db-creds"),
		Namespace: types.StringValue("apps"),
		Ttl:       types.Int64Value(3600),
		Type:      types.StringValue("Opaque"),
		SecretRef: []ValsSecretReference{{Name: "password", Ref: "ref+vault://secret/db#pw"}},
	}
}

func TestCreateValsSecret(t *testing.T) {
	ctx := context.Background()
	client := newFakeDynamicClient()
	plan := sampleValsPlan()

	if _, err := CreateValsSecret(ctx, client, plan); err != nil {
		t.Fatalf("first create failed: %v", err)
	}

	got, err := GetValsSecret(ctx, client, "db-creds", "apps")
	if err != nil {
		t.Fatalf("get after create failed: %v", err)
	}
	if got.GetName() != "db-creds" || got.Spec.TTL != 3600 {
		t.Fatalf("unexpected object read back: %+v", got)
	}

	// Creating the same object again must fail and point to import.
	_, err = CreateValsSecret(ctx, client, plan)
	if err == nil {
		t.Fatal("expected error creating an existing valssecret, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected an already-exists error, got: %v", err)
	}
}

func TestGetValsSecretNotFound(t *testing.T) {
	ctx := context.Background()
	client := newFakeDynamicClient()

	_, err := GetValsSecret(ctx, client, "missing", "apps")
	if !errors.IsNotFound(err) {
		t.Fatalf("expected NotFound, got: %v", err)
	}
}

func TestSplitNamespacedName(t *testing.T) {
	cases := []struct {
		id      string
		ns      string
		name    string
		wantErr bool
	}{
		{id: "apps/db-creds", ns: "apps", name: "db-creds"},
		{id: "db-creds", wantErr: true},
		{id: "/db-creds", wantErr: true},
		{id: "apps/", wantErr: true},
		{id: "", wantErr: true},
	}
	for _, tc := range cases {
		ns, name, err := splitNamespacedName(tc.id)
		if tc.wantErr {
			if err == nil {
				t.Errorf("splitNamespacedName(%q) expected error, got nil", tc.id)
			}
			continue
		}
		if err != nil {
			t.Errorf("splitNamespacedName(%q) unexpected error: %v", tc.id, err)
			continue
		}
		if ns != tc.ns || name != tc.name {
			t.Errorf("splitNamespacedName(%q) = %q,%q want %q,%q", tc.id, ns, name, tc.ns, tc.name)
		}
	}
}

// TestClientCacheConcurrent exercises the mutex-guarded lazy init under -race
// and asserts the cached client is reused rather than rebuilt per call.
func TestClientCacheConcurrent(t *testing.T) {
	k := &kubeClientsets{config: &restclient.Config{Host: "https://localhost:6443"}}

	const n = 20
	results := make([]dynamic.Interface, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			c, err := k.DynamicClient()
			if err != nil {
				t.Errorf("DynamicClient error: %v", err)
				return
			}
			results[idx] = c
		}(i)
	}
	wg.Wait()

	for i := 1; i < n; i++ {
		if results[i] != results[0] {
			t.Fatalf("client cache returned different instances: %p vs %p", results[i], results[0])
		}
	}
}

func TestClientCacheNoConfig(t *testing.T) {
	k := &kubeClientsets{}
	if _, err := k.DynamicClient(); err == nil {
		t.Fatal("expected error when no config is set, got nil")
	}
}
