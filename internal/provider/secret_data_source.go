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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &SecretDataSource{}

func NewSecretDataSource() datasource.DataSource {
	return &SecretDataSource{}
}

// SecretDataSource defines the data source implementation.
type SecretDataSource struct {
	client *kubernetes.Clientset
	cfg    *restclient.Config
}

// SecretDataSourceModel describes the data source data model.
type SecretDataSourceModel struct {
	Name       types.String `tfsdk:"name"`
	Namespace  types.String `tfsdk:"namespace"`
	Data       types.String `tfsdk:"data"`
	BinaryData types.String `tfsdk:"binary_data"`
	Type       types.String `tfsdk:"type"`
}

func (d *SecretDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_secret"
}

func (d *SecretDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Secret data source",

		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Secret name",
				Required:            true,
			},
			"namespace": schema.StringAttribute{
				MarkdownDescription: "Secret namespace",
				Required:            true,
			},
			"data": schema.StringAttribute{
				MarkdownDescription: "Secret data",
				Computed:            true,
			},
			"binary_data": schema.StringAttribute{
				MarkdownDescription: "Secret data in base64",
				Computed:            true,
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Secret data type (default Opaque)",
				Computed:            true,
			},
		},
	}
}

func (d *SecretDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

	d.client = client
	d.cfg = restClient
}

func (d *SecretDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data SecretDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	s, err := d.getSecret(ctx, data.Name.ValueString(), data.Namespace.ValueString())
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
	data.Type = types.StringValue(string(s.Type))

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (p *SecretDataSource) getSecret(ctx context.Context, secretName string, namespace string) (*corev1.Secret, error) {
	var secret *corev1.Secret

	secret, err := p.client.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return secret, nil
}
