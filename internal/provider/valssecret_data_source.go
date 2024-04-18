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

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &ValsSecretDataSource{}

func NewValsSecretDataSource() datasource.DataSource {
	return &ValsSecretDataSource{}
}

// ValsSecretDataSource defines the data source implementation.
type ValsSecretDataSource struct {
	client        *kubernetes.Clientset
	cfg           *restclient.Config
	dynamicClient dynamic.Interface
}

// TfDataSource is a copy of DataSource using the Tf data types
type TfDataSource struct {
	Key      types.String `tfsdk:"key"`
	Ref      types.String `tfsdk:"ref"`
	Encoding types.String `tfsdk:"encoding"`
}

// TfTemplate is a copy of DataSource using the Tf data types
type TfTemplateSource struct {
	Name  types.String `tfsdk:"name"`
	Value types.String `tfsdk:"value"`
}

// ValsSecretDataSourceModel describes the data source data model.
type ValsSecretDataSourceModel struct {
	Name      types.String       `tfsdk:"name"`
	Namespace types.String       `tfsdk:"namespace"`
	Data      []TfDataSource     `tfsdk:"data"`
	Template  []TfTemplateSource `tfsdk:"template"`
	Type      types.String       `tfsdk:"type"`
	Ttl       types.Int64        `tfsdk:"ttl"`
}

func (d *ValsSecretDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_valssecret"
}

func (d *ValsSecretDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Vals Opetator secret data source",

		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Vals secret name",
				Required:            true,
			},
			"namespace": schema.StringAttribute{
				MarkdownDescription: "Vals secret namespace",
				Required:            true,
			},
			"ttl": schema.Int64Attribute{
				MarkdownDescription: "Vals secret ttl (default is 3600 seconds)",
				Optional:            true,
			},
			"data": schema.ListNestedAttribute{
				MarkdownDescription: "Secret data objects",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"key": schema.StringAttribute{
							Required: true,
							Computed: false,
						},
						"ref": schema.StringAttribute{
							Required: true,
							Computed: false,
						},
						"encoding": schema.StringAttribute{
							Required: false,
							Optional: true,
						},
					},
				},
			},
			"template": schema.ListNestedAttribute{
				MarkdownDescription: "Secret template data",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"key": schema.StringAttribute{
							Required: true,
							Computed: false,
						},
						"value": schema.StringAttribute{
							Required: true,
							Computed: false,
						},
					},
				},
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Secret data type (default Opaque)",
				Computed:            true,
			},
		},
	}
}

func (d *ValsSecretDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	client, err := req.ProviderData.(*kubeClientsets).MainClientset()

	if err != nil {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *provider.KubeClientsets., got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	restClient, err := req.ProviderData.(*kubeClientsets).RestClientConfig()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *restclient.Config., got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	dClient, err := req.ProviderData.(*kubeClientsets).DynamicClient()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected dynamic.Interface., got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	d.client = client
	d.cfg = restClient
	d.dynamicClient = dClient
}

func (d *ValsSecretDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ValsSecretDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	s, err := GetValsSecret(ctx, d.dynamicClient, data.Name.ValueString(), data.Namespace.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Read Secret",
			fmt.Sprintf("Error getting secret from Kubernetes: %v", err),
		)

		return
	}

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "reading secret from kubernetes")

	// For the purposes of this Secret code, hardcoding a response value to
	// save into the Terraform state.
	data.Name = types.StringValue(s.GetName())
	data.Namespace = types.StringValue(s.GetNamespace())
	data.Ttl = types.Int64Value(s.Spec.TTL)

	for dataEntry := range s.Spec.Data {
		entry := TfDataSource{
			Key:      types.StringValue(dataEntry),
			Ref:      types.StringValue(s.Spec.Data[dataEntry].Ref),
			Encoding: types.StringValue(s.Spec.Data[dataEntry].Encoding),
		}
		data.Data = append(data.Data, entry)
	}

	for k, v := range s.Spec.Template {
		entry := TfTemplateSource{
			Name:  types.StringValue(k),
			Value: types.StringValue(v),
		}
		data.Template = append(data.Template, entry)
	}

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
