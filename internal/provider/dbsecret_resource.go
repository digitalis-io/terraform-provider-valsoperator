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

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &DbSecretResource{}
var _ resource.ResourceWithImportState = &DbSecretResource{}

func NewDbSecretResource() resource.Resource {
	return &DbSecretResource{}
}

// DbSecretResource defines the resource implementation.
type DbSecretResource struct {
	client        *kubernetes.Clientset
	cfg           *restclient.Config
	dynamicClient dynamic.Interface
}

type DbSecretVaultModel struct {
	Role  string `tfsdk:"role"`
	Mount string `tfsdk:"mount"`
}

type DbSecretRolloutModel struct {
	Kind string `tfsdk:"kind"`
	Name string `tfsdk:"name"`
}

type DbSecretTemplateModel struct {
	Name  string `tfsdk:"name"`
	Value string `tfsdk:"value"`
}

// DbSecretResourceModel describes the resource data model.
type DbSecretResourceModel struct {
	Name       types.String            `tfsdk:"name"`
	Namespace  types.String            `tfsdk:"namespace"`
	SecretName types.String            `tfsdk:"secret_name"`
	Vault      []DbSecretVaultModel    `tfsdk:"vault"`
	Template   []DbSecretTemplateModel `tfsdk:"template"`
	Rollout    []DbSecretRolloutModel  `tfsdk:"rollout"`
	Renew      types.Bool              `tfsdk:"renew"`
}

func (r *DbSecretResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dbsecret"
}

func (r *DbSecretResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Vals Operator DbSecret resource for managing Vault dynamic database credentials",

		Blocks: map[string]schema.Block{
			"vault": schema.ListNestedBlock{
				MarkdownDescription: "Vault connection configuration",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"role": schema.StringAttribute{
							MarkdownDescription: "Vault role used to connect to the database",
							Required:            true,
						},
						"mount": schema.StringAttribute{
							MarkdownDescription: "Vault database mount path",
							Required:            true,
						},
					},
				},
			},
			"template": schema.ListNestedBlock{
				MarkdownDescription: "Template for the secret data keys",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Required: true,
						},
						"value": schema.StringAttribute{
							Required: true,
						},
					},
				},
			},
			"rollout": schema.ListNestedBlock{
				MarkdownDescription: "List of Deployments or StatefulSets to rollout restart when credentials are renewed",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"kind": schema.StringAttribute{
							MarkdownDescription: "Kind of the resource: Deployment or StatefulSet",
							Required:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Name of the Deployment or StatefulSet",
							Required:            true,
						},
					},
				},
			},
		},
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "DbSecret name",
				Required:            true,
			},
			"namespace": schema.StringAttribute{
				MarkdownDescription: "DbSecret namespace",
				Required:            true,
			},
			"secret_name": schema.StringAttribute{
				MarkdownDescription: "Override the Kubernetes secret name (defaults to metadata.name)",
				Optional:            true,
			},
			"renew": schema.BoolAttribute{
				MarkdownDescription: "Whether to automatically renew the credentials before they expire",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
		},
	}
}

func (r *DbSecretResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, err := req.ProviderData.(*kubeClientsets).MainClientset()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *provider.KubeClientsets, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	restClient, err := req.ProviderData.(*kubeClientsets).RestClientConfig()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *restclient.Config, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	dClient, err := req.ProviderData.(*kubeClientsets).DynamicClient()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected dynamic.Interface, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client
	r.cfg = restClient
	r.dynamicClient = dClient
}

func (r *DbSecretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan DbSecretResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("[DEBUG] Creating a DbSecret for %v/%v", plan.Name.ValueString(), plan.Namespace.ValueString())
	_, err := CreateDbSecret(ctx, r.dynamicClient, plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"Apply failed",
			fmt.Sprintf("Error applying: %v", err),
		)
		return
	}

	diags := resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *DbSecretResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state DbSecretResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	s, err := GetDbSecret(ctx, r.dynamicClient, state.Name.ValueString(), state.Namespace.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unexpected Resource Read DbSecret",
			fmt.Sprintf("Error getting DbSecret from Kubernetes: %v", err),
		)
		return
	}
	tflog.Debug(ctx, fmt.Sprintf("[DEBUG] found a kubernetes dbsecret in namespace %s with the name %s", s.GetNamespace(), s.GetName()))
	tflog.Trace(ctx, "reading dbsecret from kubernetes")

	state.Name = types.StringValue(s.GetName())
	state.Namespace = types.StringValue(s.GetNamespace())
	state.Renew = types.BoolValue(s.Spec.Renew)
	if s.Spec.SecretName != "" {
		state.SecretName = types.StringValue(s.Spec.SecretName)
	}

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

func (r *DbSecretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan DbSecretResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("[DEBUG] Updating a DbSecret for %v/%v", plan.Name.ValueString(), plan.Namespace.ValueString())

	_, err := CreateDbSecret(ctx, r.dynamicClient, plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"Apply failed",
			fmt.Sprintf("Error applying: %v", err),
		)
		return
	}

	diags := resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *DbSecretResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data DbSecretResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := DeleteDbSecret(ctx, r.dynamicClient, data.Name.ValueString(), data.Namespace.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Delete error",
			fmt.Sprintf("Error deleting dbsecret: %v", err),
		)
	}
}

func (r *DbSecretResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
