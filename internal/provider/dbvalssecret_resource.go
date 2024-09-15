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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
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

type TfDbRolloutTarget struct {
	// Kind is either Deployment or StatefulSet
	Kind string `tfsdk:"kind"`
	// Name is the object name
	Name string `tfsdk:"name"`
}

type DbSecretTemplate struct {
	Name  string `tfsdk:"name"`
	Value string `tfsdk:"value"`
}

// DbSecretResourceModel describes the resource data model.
type DbSecretResourceModel struct {
	Name       types.String        `tfsdk:"name"`
	Namespace  types.String        `tfsdk:"namespace"`
	VaultRole  types.String        `tfsdk:"vault_role"`
	VaultMount types.String        `tfsdk:"vault_mount"`
	Template   []DbSecretTemplate  `tfsdk:"template"`
	Renew      types.Bool          `tfsdk:"renew"`
	Rollout    []TfDbRolloutTarget `tfsdk:"rollout"`
}

func (r *DbSecretResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dbsecret"
}

func (r *DbSecretResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Vals Operator secret data source",

		Blocks: map[string]schema.Block{
			"template": schema.ListNestedBlock{
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
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Required: true,
						},
						"kind": schema.StringAttribute{
							Required: true,
						},
					},
				},
			},
		},
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Vals db secret name",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"renew": schema.BoolAttribute{
				MarkdownDescription: "Whether to renew or reissue the credentials",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"namespace": schema.StringAttribute{
				MarkdownDescription: "Vals db secret namespace",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"vault_role": schema.StringAttribute{
				MarkdownDescription: "Vaule role name with permission to issue credentials",
				Required:            true,
			},
			"vault_mount": schema.StringAttribute{
				MarkdownDescription: "Path to the secrets engine providing the credentials",
				Required:            true,
			},
		},
	}
}

func (r *DbSecretResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

	r.client = client
	r.cfg = restClient
	r.dynamicClient = dClient
}

func (r *DbSecretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan DbSecretResourceModel

	// Read Terraform plan data into the model
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

	// Set state to fully populated data
	diags := resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *DbSecretResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Retrieve values from plan
	var state DbSecretResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	s, err := GetDbSecret(ctx, r.dynamicClient, state.Name.ValueString(), state.Namespace.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unexpected Resource Read Secret",
			fmt.Sprintf("Error getting secret from Kubernetes: %v", err),
		)

		return
	}
	tflog.Debug(ctx, fmt.Sprintf("[DEBUG] found a kubernetes DbSecret in namespace %s with the name %s ", s.GetNamespace(), s.Name))

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "reading secret from kubernetes")

	state.Name = types.StringValue(s.GetName())
	state.Namespace = types.StringValue(s.GetNamespace())

	// Set refreshed state
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		resp.Diagnostics.AddError(
			"Planning error",
			fmt.Sprintf("Error updating terraform plan: %v", err),
		)
		return
	}
}

func (r *DbSecretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan DbSecretResourceModel

	// Read Terraform plan data into the model
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

	// Set state to fully populated data
	diags := resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		resp.Diagnostics.AddError(
			"Update error",
			fmt.Sprintf("Error updating DbSecret: %v", err),
		)
		return
	}
}

func (r *DbSecretResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data DbSecretResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		resp.Diagnostics.AddError(
			"Delete error",
			fmt.Sprintf("Error loading the the state file"),
		)
		return
	}

	err := DeleteDbSecret(ctx, r.dynamicClient, data.Name.ValueString(), data.Namespace.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Delete error",
			fmt.Sprintf("Error deleting DbSecret: %v", err),
		)
	}
}

func (r *DbSecretResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
