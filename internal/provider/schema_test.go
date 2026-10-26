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
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	fwdatasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	fwresourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// TestResourceSchemas verifies each resource schema is internally valid. The
// framework reports contradictory attributes (e.g. Required combined with
// Computed) through the schema's validation, which this test exercises.
func TestResourceSchemas(t *testing.T) {
	ctx := context.Background()
	resources := map[string]resource.Resource{
		"valssecret": NewValsSecretResource(),
		"dbsecret":   NewDbSecretResource(),
	}

	for name, r := range resources {
		t.Run(name, func(t *testing.T) {
			var resp resource.SchemaResponse
			r.Schema(ctx, resource.SchemaRequest{}, &resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("schema has errors: %v", resp.Diagnostics)
			}
			if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
				t.Fatalf("schema implementation invalid: %v", diags)
			}
			var _ fwresourceschema.Schema = resp.Schema
		})
	}
}

// TestDataSourceSchemas verifies each data source schema is internally valid.
func TestDataSourceSchemas(t *testing.T) {
	ctx := context.Background()
	sources := map[string]datasource.DataSource{
		"secret":     NewSecretDataSource(),
		"valssecret": NewValsSecretDataSource(),
	}

	for name, d := range sources {
		t.Run(name, func(t *testing.T) {
			var resp datasource.SchemaResponse
			d.Schema(ctx, datasource.SchemaRequest{}, &resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("schema has errors: %v", resp.Diagnostics)
			}
			if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
				t.Fatalf("schema implementation invalid: %v", diags)
			}
			var _ fwdatasourceschema.Schema = resp.Schema
		})
	}
}
