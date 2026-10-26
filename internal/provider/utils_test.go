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
	"reflect"
	"testing"
)

func TestOrderedKeys(t *testing.T) {
	cases := []struct {
		name    string
		prior   []string
		present map[string]bool
		want    []string
	}{
		{
			name:    "preserves prior order and appends new sorted",
			prior:   []string{"b", "a"},
			present: map[string]bool{"a": true, "b": true, "z": true, "m": true},
			want:    []string{"b", "a", "m", "z"},
		},
		{
			name:    "drops prior keys no longer present",
			prior:   []string{"a", "gone", "b"},
			present: map[string]bool{"a": true, "b": true},
			want:    []string{"a", "b"},
		},
		{
			name:    "all new keys sorted when no prior",
			prior:   nil,
			present: map[string]bool{"c": true, "a": true, "b": true},
			want:    []string{"a", "b", "c"},
		},
		{
			name:    "empty present yields empty",
			prior:   []string{"a"},
			present: map[string]bool{},
			want:    []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := orderedKeys(tc.prior, tc.present)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("orderedKeys() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRefreshSecretRefs(t *testing.T) {
	prior := []ValsSecretReference{
		{Name: "second", Ref: "old", Encoding: "base64"},
		{Name: "first", Ref: "old"},
	}
	data := map[string]DataSource{
		"first":  {Ref: "ref+vault://a"},
		"second": {Ref: "ref+vault://b", Encoding: "base64"},
		"third":  {Ref: "ref+vault://c"},
	}

	got := refreshSecretRefs(prior, data)
	want := []ValsSecretReference{
		{Name: "second", Ref: "ref+vault://b", Encoding: "base64"},
		{Name: "first", Ref: "ref+vault://a"},
		{Name: "third", Ref: "ref+vault://c"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("refreshSecretRefs() = %+v, want %+v", got, want)
	}

	if refreshSecretRefs(nil, nil) != nil {
		t.Fatal("refreshSecretRefs(nil, nil) should be nil")
	}
}

func TestRefreshTemplates(t *testing.T) {
	prior := []ValsSecretTemplate{{Name: "b", Value: "old"}}
	tmpl := map[string]string{"b": "newB", "a": "newA"}

	got := refreshTemplates(prior, tmpl)
	want := []ValsSecretTemplate{
		{Name: "b", Value: "newB"},
		{Name: "a", Value: "newA"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("refreshTemplates() = %+v, want %+v", got, want)
	}
}

func TestRefreshDbTemplates(t *testing.T) {
	got := refreshDbTemplates(nil, map[string]string{"x": "1", "a": "2"})
	want := []DbSecretTemplateModel{
		{Name: "a", Value: "2"},
		{Name: "x", Value: "1"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("refreshDbTemplates() = %+v, want %+v", got, want)
	}
}
