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

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &ValsSecretResource{}
var _ resource.ResourceWithImportState = &ValsSecretResource{}

func NewValsSecretResource() resource.Resource {
	return &ValsSecretResource{}
}

// ValsSecretResource defines the resource implementation.
type ValsSecretResource struct {
	client        *kubernetes.Clientset
	cfg           *restclient.Config
	dynamicClient dynamic.Interface
}

type ValsSecretReference struct {
	Name     string `tfsdk:"name"`
	Ref      string `tfsdk:"ref"`
	Encoding string `tfsdk:"encoding"`
}

type ValsSecretTemplate struct {
	Name  string `tfsdk:"name"`
	Value string `tfsdk:"value"`
}

type ValsSecretRollout struct {
	Kind string `tfsdk:"kind"`
	Name string `tfsdk:"name"`
}

// ValsSecretResourceModel describes the resource data model.
type ValsSecretResourceModel struct {
	Name      types.String          `tfsdk:"name"`
	Namespace types.String          `tfsdk:"namespace"`
	SecretRef []ValsSecretReference `tfsdk:"secret_ref"`
	Template  []ValsSecretTemplate  `tfsdk:"template"`
	Rollout   []ValsSecretRollout   `tfsdk:"rollout"`
	Type      types.String          `tfsdk:"type"`
	Ttl       types.Int64           `tfsdk:"ttl"`
}

func (r *ValsSecretResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_valssecret"
}

func (r *ValsSecretResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Vals Operator secret resource",

		Blocks: map[string]schema.Block{
			"secret_ref": schema.ListNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Required: true,
						},
						"ref": schema.StringAttribute{
							Required: true,
						},
						"encoding": schema.StringAttribute{
							Optional: true,
						},
					},
				},
			},
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
				MarkdownDescription: "List of Deployments or StatefulSets to rollout restart when the secret changes",
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
				MarkdownDescription: "Vals secret name",
				Required:            true,
			},
			"namespace": schema.StringAttribute{
				MarkdownDescription: "Vals secret namespace",
				Required:            true,
			},
			"ttl": schema.Int64Attribute{
				MarkdownDescription: "Vals secret ttl",
				Optional:            true,
				Default:             int64default.StaticInt64(3600),
				Computed:            true,
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Secret data type (default Opaque)",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("Opaque"),
			},
		},
	}
}

func (r *ValsSecretResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	clients, ok := req.ProviderData.(*kubeClientsets)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *kubeClientsets, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	client, err := clients.MainClientset()
	if err != nil {
		resp.Diagnostics.AddError("Kubernetes client error", err.Error())
		return
	}

	restClient, err := clients.RestClientConfig()
	if err != nil {
		resp.Diagnostics.AddError("Kubernetes client error", err.Error())
		return
	}

	dClient, err := clients.DynamicClient()
	if err != nil {
		resp.Diagnostics.AddError("Kubernetes client error", err.Error())
		return
	}

	r.client = client
	r.cfg = restClient
	r.dynamicClient = dClient
}

func (r *ValsSecretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ValsSecretResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Creating a ValsSecret for %s/%s", plan.Namespace.ValueString(), plan.Name.ValueString()))
	_, err := CreateValsSecret(ctx, r.dynamicClient, plan)
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

func (r *ValsSecretResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Retrieve values from plan
	var state ValsSecretResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	s, err := GetValsSecret(ctx, r.dynamicClient, state.Name.ValueString(), state.Namespace.ValueString())
	if err != nil {
		if errors.IsNotFound(err) {
			// The resource was deleted outside Terraform; drop it from state
			// so it is planned for recreation.
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Unexpected Resource Read Secret",
			fmt.Sprintf("Error getting secret from Kubernetes: %v", err),
		)

		return
	}
	tflog.Debug(ctx, fmt.Sprintf("found a kubernetes valssecret in namespace %s with the name %s", s.GetNamespace(), s.GetName()))

	state.Name = types.StringValue(s.GetName())
	state.Namespace = types.StringValue(s.GetNamespace())
	state.Ttl = types.Int64Value(s.Spec.TTL)
	if s.Spec.Type != "" {
		state.Type = types.StringValue(s.Spec.Type)
	}

	// Refresh the nested blocks from the live object so drift is detected.
	// Maps have no stable order, so reconcile against the order already in
	// state to avoid a perpetual diff: keep existing entries that still exist
	// (with refreshed values), then append any new keys in sorted order.
	state.SecretRef = refreshSecretRefs(state.SecretRef, s.Spec.Data)
	state.Template = refreshTemplates(state.Template, s.Spec.Template)

	state.Rollout = nil
	for _, ro := range s.Spec.Rollout {
		state.Rollout = append(state.Rollout, ValsSecretRollout{
			Kind: ro.Kind,
			Name: ro.Name,
		})
	}

	// Set refreshed state
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

func (r *ValsSecretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ValsSecretResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Updating a ValsSecret for %s/%s", plan.Namespace.ValueString(), plan.Name.ValueString()))

	_, err := UpdateValsSecret(ctx, r.dynamicClient, plan)
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
}

func (r *ValsSecretResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ValsSecretResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	err := DeleteValsSecret(ctx, r.dynamicClient, data.Name.ValueString(), data.Namespace.ValueString())
	if err != nil && !errors.IsNotFound(err) {
		resp.Diagnostics.AddError(
			"Delete error",
			fmt.Sprintf("Error deleting valssecret: %v", err),
		)
	}
}

// ImportState imports an existing ValsSecret using a "namespace/name" identifier.
func (r *ValsSecretResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	namespace, name, err := splitNamespacedName(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("namespace"), namespace)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
}
