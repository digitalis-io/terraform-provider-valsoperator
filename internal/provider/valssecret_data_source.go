// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
)

// DataSource defines a secret
type DataSource struct {
	// Ref value to the secret in the format ref+backend://path
	// https://github.com/helmfile/vals
	Ref string `json:"ref"`
	// Encoding type for the secret. Only base64 supported. Optional
	Encoding string `json:"encoding,omitempty"`
}

// DatabaseLoginCredentials holds the access details for the DB
type DatabaseLoginCredentials struct {
	// Name of the secret containing the credentials to be able to log in to the database
	SecretName string `json:"secretName"`
	// Optional namespace of the secret, default current namespace
	Namespace string `json:"namespace,omitempty"`
	// Key in the secret containing the database username
	UsernameKey string `json:"usernameKey,omitempty"`
	// Key in the secret containing the database username
	PasswordKey string `json:"passwordKey"`
}

// Database defines a DB connection
type Database struct {
	// Defines the database type
	Driver string `json:"driver"`
	// Credentials to access the database
	LoginCredentials DatabaseLoginCredentials `json:"loginCredentials,omitempty"`
	// Database port number
	Port int `json:"port,omitempty"`
	// Key in the secret containing the database username
	UsernameKey string `json:"usernameKey,omitempty"`
	// Key in the secret containing the database username
	PasswordKey string `json:"passwordKey"`
	// Used for MySQL only, the host part for the username
	UserHost string `json:"userHost,omitempty"`
	// List of hosts to connect to, they'll be tried in sequence until one succeeds
	Hosts []string `json:"hosts"`
}

// ValsSecretSpec defines the desired state of ValsSecret
type ValsSecretSpec struct {
	Name      string                `json:"name,omitempty"`
	Data      map[string]DataSource `json:"data"`
	TTL       int64                 `json:"ttl,omitempty"`
	Type      string                `json:"type,omitempty"`
	Databases []Database            `json:"databases,omitempty"`
	Template  map[string]string     `json:"template,omitempty"`
}

// ValsSecretStatus defines the observed state of ValsSecret
type ValsSecretStatus struct {
}

// ValsSecret is the Schema for the valssecrets API
type ValsSecret struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ValsSecretSpec   `json:"spec,omitempty"`
	Status ValsSecretStatus `json:"status,omitempty"`
}

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
	Key   types.String `tfsdk:"key"`
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
				MarkdownDescription: "Vals secret ttl",
				Optional:            true,
			},
			"data": schema.ListNestedAttribute{
				MarkdownDescription: "Secret data",
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

	s, err := d.getValsSecret(ctx, data.Name.ValueString(), data.Namespace.ValueString())
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
			Key:   types.StringValue(k),
			Value: types.StringValue(v),
		}
		data.Template = append(data.Template, entry)
	}

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (p *ValsSecretDataSource) getValsSecret(ctx context.Context, secretName string, namespace string) (*ValsSecret, error) {
	var secret *ValsSecret

	// Define the GVR (Group-Version-Resource) for the custom resource
	gvr := k8sschema.GroupVersionResource{
		Group:    "digitalis.io",
		Version:  "v1",
		Resource: "valssecrets",
	}

	obj, err := p.dynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return secret, err
	}

	err = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), &secret)
	if err != nil {
		return secret, err
	}

	return secret, nil
}
