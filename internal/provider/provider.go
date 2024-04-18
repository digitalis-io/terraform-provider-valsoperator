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
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	gversion "github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/logging"
	"github.com/mitchellh/go-homedir"
	apimachineryschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	aggregator "k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset"
)

// Ensure ValsOperatorProvider satisfies various provider interfaces.
var _ provider.Provider = &ValsOperatorProvider{}

// ValsOperatorProvider defines the provider implementation.
type ValsOperatorProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// KubernetesProviderModel describes the provider data model.
type ValsOperatorProviderModel struct {
	Host     types.String `tfsdk:"host"`
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
	Insecure types.Bool   `tfsdk:"insecure"`

	TLSServerName        types.String `tfsdk:"tls_server_name"`
	ClientCertificate    types.String `tfsdk:"client_certificate"`
	ClientKey            types.String `tfsdk:"client_key"`
	ClusterCACertificate types.String `tfsdk:"cluster_ca_certificate"`

	ConfigPaths []types.String `tfsdk:"config_paths"`
	ConfigPath  types.String   `tfsdk:"config_path"`

	ConfigContext         types.String `tfsdk:"config_context"`
	ConfigContextAuthInfo types.String `tfsdk:"config_context_auth_info"`
	ConfigContextCluster  types.String `tfsdk:"config_context_cluster"`

	Token types.String `tfsdk:"token"`

	ProxyURL types.String `tfsdk:"proxy_url"`

	IgnoreAnnotations types.List `tfsdk:"ignore_annotations"`
	IgnoreLabels      types.List `tfsdk:"ignore_labels"`

	Exec []struct {
		APIVersion types.String            `tfsdk:"api_version"`
		Command    types.String            `tfsdk:"command"`
		Env        map[string]types.String `tfsdk:"env"`
		Args       []types.String          `tfsdk:"args"`
	} `tfsdk:"exec"`

	Experiments []struct {
		ManifestResource types.Bool `tfsdk:"manifest_resource"`
	} `tfsdk:"experiments"`
}

func (p *ValsOperatorProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "valsoperator"
	resp.Version = p.version
}

func (p *ValsOperatorProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				Description: "The hostname (in form of URI) of Kubernetes master.",
				Optional:    true,
			},
			"username": schema.StringAttribute{
				Description: "The username to use for HTTP basic authentication when accessing the Kubernetes master endpoint.",
				Optional:    true,
			},
			"password": schema.StringAttribute{
				Description: "The password to use for HTTP basic authentication when accessing the Kubernetes master endpoint.",
				Optional:    true,
			},
			"insecure": schema.BoolAttribute{
				Description: "Whether server should be accessed without verifying the TLS certificate.",
				Optional:    true,
			},
			"tls_server_name": schema.StringAttribute{
				Description: "Server name passed to the server for SNI and is used in the client to check server certificates against.",
				Optional:    true,
			},
			"client_certificate": schema.StringAttribute{
				Description: "PEM-encoded client certificate for TLS authentication.",
				Optional:    true,
			},
			"client_key": schema.StringAttribute{
				Description: "PEM-encoded client certificate key for TLS authentication.",
				Optional:    true,
			},
			"cluster_ca_certificate": schema.StringAttribute{
				Description: "PEM-encoded root certificates bundle for TLS authentication.",
				Optional:    true,
			},
			"config_paths": schema.ListAttribute{
				ElementType: types.StringType,
				Description: "A list of paths to kube config files. Can be set with KUBE_CONFIG_PATHS environment variable.",
				Optional:    true,
			},
			"config_path": schema.StringAttribute{
				Description: "Path to the kube config file. Can be set with KUBE_CONFIG_PATH.",
				Optional:    true,
			},
			"config_context": schema.StringAttribute{
				Description: "",
				Optional:    true,
			},
			"config_context_auth_info": schema.StringAttribute{
				Description: "",
				Optional:    true,
			},
			"config_context_cluster": schema.StringAttribute{
				Description: "",
				Optional:    true,
			},
			"token": schema.StringAttribute{
				Description: "Token to authenticate an service account",
				Optional:    true,
			},
			"proxy_url": schema.StringAttribute{
				Description: "URL to the proxy to be used for all API requests",
				Optional:    true,
			},
			"ignore_annotations": schema.ListAttribute{
				ElementType: types.StringType,
				Description: "List of Kubernetes metadata annotations to ignore across all resources handled by this provider for situations where external systems are managing certain resource annotations. Each item is a regular expression.",
				Optional:    true,
			},
			"ignore_labels": schema.ListAttribute{
				ElementType: types.StringType,
				Description: "List of Kubernetes metadata labels to ignore across all resources handled by this provider for situations where external systems are managing certain resource labels. Each item is a regular expression.",
				Optional:    true,
			},
		},
		Blocks: map[string]schema.Block{
			"exec": schema.ListNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"api_version": schema.StringAttribute{
							Required: true,
						},
						"command": schema.StringAttribute{
							Required: true,
						},
						"env": schema.MapAttribute{
							ElementType: types.StringType,
							Optional:    true,
						},
						"args": schema.ListAttribute{
							ElementType: types.StringType,
							Optional:    true,
						},
					},
				},
			},
			"experiments": schema.ListNestedBlock{
				Description: "Enable and disable experimental features.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"manifest_resource": schema.BoolAttribute{
							Description: "Enable the `kubernetes_manifest` resource.",
							Optional:    true,
						},
					},
				},
			},
		},
	}
}

func (p *ValsOperatorProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data ValsOperatorProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	cfg, err := initializeConfiguration(ctx, data)
	if err != nil {
		resp.Diagnostics.AddError("Kubernetes config", "The Kubernetes access config is not correct")
		return
	}
	if cfg == nil {
		// This is a TEMPORARY measure to work around https://github.com/hashicorp/terraform/issues/24055
		// IMPORTANT: this will NOT enable a workaround of issue: https://github.com/hashicorp/terraform/issues/4149
		// IMPORTANT: if the supplied configuration is incomplete or invalid
		///IMPORTANT: provider operations will fail or attempt to connect to localhost endpoints
		cfg = &restclient.Config{}
	}

	cfg.UserAgent = fmt.Sprintf("HashiCorp/1.0 Terraform/%s", req.TerraformVersion)

	if logging.IsDebugOrHigher() {
		log.Printf("[DEBUG] Enabling HTTP requests/responses tracing")
		cfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
			return logging.NewSubsystemLoggingHTTPTransport("Kubernetes", rt)
		}
	}

	ignoreAnnotations := []string{}
	ignoreLabels := []string{}

	for _, x := range data.IgnoreAnnotations.Elements() {
		ignoreAnnotations = append(ignoreAnnotations, x.String())
	}
	for _, x := range data.IgnoreLabels.Elements() {
		ignoreAnnotations = append(ignoreAnnotations, x.String())
	}

	m := &kubeClientsets{
		config:              cfg,
		mainClientset:       nil,
		aggregatorClientset: nil,
		IgnoreAnnotations:   ignoreAnnotations,
		IgnoreLabels:        ignoreLabels,
	}

	log.Printf("[DEBUG] the config file is %s", cfg.Host)

	// Secret client configuration for data sources and resources
	resp.DataSourceData = m
	resp.ResourceData = m
}

func (p *ValsOperatorProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewValsSecretResource,
	}
}

func (p *ValsOperatorProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewSecretDataSource,
		NewValsSecretDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &ValsOperatorProvider{
			version: version,
		}
	}
}

// Kube config

type KubeClientsets interface {
	MainClientset() (*kubernetes.Clientset, error)
	AggregatorClientset() (*aggregator.Clientset, error)
	DynamicClient() (dynamic.Interface, error)
	DiscoveryClient() (discovery.DiscoveryInterface, error)
	RestClientConfig() (*restclient.Config, error)
}

type kubeClientsets struct {
	// TODO: this struct has become overloaded we should
	// rename this or break it into smaller structs
	config              *restclient.Config
	mainClientset       *kubernetes.Clientset
	aggregatorClientset *aggregator.Clientset
	dynamicClient       dynamic.Interface
	discoveryClient     discovery.DiscoveryInterface

	IgnoreAnnotations []string
	IgnoreLabels      []string
}

func (k kubeClientsets) MainClientset() (*kubernetes.Clientset, error) {
	if k.mainClientset != nil {
		return k.mainClientset, nil
	}

	if k.config != nil {
		kc, err := kubernetes.NewForConfig(k.config)
		if err != nil {
			return nil, fmt.Errorf("Failed to configure client: %s", err)
		}
		k.mainClientset = kc
	}
	return k.mainClientset, nil
}

func (k kubeClientsets) RestClientConfig() (*restclient.Config, error) {
	return k.config, nil
}

func (k kubeClientsets) AggregatorClientset() (*aggregator.Clientset, error) {
	if k.aggregatorClientset != nil {
		return k.aggregatorClientset, nil
	}
	if k.config != nil {
		ac, err := aggregator.NewForConfig(k.config)
		if err != nil {
			return nil, fmt.Errorf("Failed to configure client: %s", err)
		}
		k.aggregatorClientset = ac
	}
	return k.aggregatorClientset, nil
}

func (k kubeClientsets) DynamicClient() (dynamic.Interface, error) {
	if k.dynamicClient != nil {
		return k.dynamicClient, nil
	}

	if k.config != nil {
		kc, err := dynamic.NewForConfig(k.config)
		if err != nil {
			return nil, fmt.Errorf("Failed to configure dynamic client: %s", err)
		}
		k.dynamicClient = kc
	}
	return k.dynamicClient, nil
}

func (k kubeClientsets) DiscoveryClient() (discovery.DiscoveryInterface, error) {
	if k.discoveryClient != nil {
		return k.discoveryClient, nil
	}

	if k.config != nil {
		kc, err := discovery.NewDiscoveryClientForConfig(k.config)
		if err != nil {
			return nil, fmt.Errorf("Failed to configure discovery client: %s", err)
		}
		k.discoveryClient = kc
	}
	return k.discoveryClient, nil
}

func initializeConfiguration(ctx context.Context, d ValsOperatorProviderModel) (*restclient.Config, error) {
	overrides := &clientcmd.ConfigOverrides{}
	loader := &clientcmd.ClientConfigLoadingRules{}

	configPaths := []string{}

	if v := d.ConfigPath.ValueString(); v != "" {
		configPaths = []string{v}
	} else if p := d.ConfigPaths; len(p) > 0 {
		for _, i := range p {
			configPaths = append(configPaths, i.ValueString())
		}
	} else if v := os.Getenv("KUBE_CONFIG_PATHS"); v != "" {
		// NOTE we have to do this here because the schema
		// does not yet allow you to set a default for a TypeList
		configPaths = filepath.SplitList(v)
	}

	if len(configPaths) > 0 {
		expandedPaths := []string{}
		for _, p := range configPaths {
			path, err := homedir.Expand(p)
			if err != nil {
				return nil, err
			}

			log.Printf("[DEBUG] Using kubeconfig: %s", path)
			expandedPaths = append(expandedPaths, path)
		}

		if len(expandedPaths) == 1 {
			loader.ExplicitPath = expandedPaths[0]
		} else {
			loader.Precedence = expandedPaths
		}

		ctxSuffix := "; default context"

		kubectx := d.ConfigContext.ValueString()
		authInfo := d.ConfigContextAuthInfo.ValueString()
		cluster := d.ConfigContextCluster.ValueString()
		if kubectx != "" || authInfo != "" || cluster != "" {
			ctxSuffix = "; overridden context"
			if kubectx != "" {
				overrides.CurrentContext = kubectx
				ctxSuffix += fmt.Sprintf("; config ctx: %s", overrides.CurrentContext)
				log.Printf("[DEBUG] Using custom current context: %q", overrides.CurrentContext)
			}

			overrides.Context = clientcmdapi.Context{}
			if authInfo != "" {
				overrides.Context.AuthInfo = authInfo
				ctxSuffix += fmt.Sprintf("; auth_info: %s", overrides.Context.AuthInfo)
			}
			if cluster != "" {
				overrides.Context.Cluster = cluster
				ctxSuffix += fmt.Sprintf("; cluster: %s", overrides.Context.Cluster)
			}
			log.Printf("[DEBUG] Using overridden context: %#v", overrides.Context)
		}
	}
	// Overriding with static configuration

	overrides.ClusterInfo.InsecureSkipTLSVerify = d.Insecure.ValueBool()
	if v := d.TLSServerName.ValueString(); v != "" {
		overrides.ClusterInfo.TLSServerName = v
	}
	if v := d.ClusterCACertificate.ValueString(); v != "" {
		overrides.ClusterInfo.CertificateAuthorityData = bytes.NewBufferString(v).Bytes()
	}
	if v := d.ClientCertificate.ValueString(); v != "" {
		overrides.AuthInfo.ClientCertificateData = bytes.NewBufferString(v).Bytes()
	}
	if v := d.Host.ValueString(); v != "" {
		// Server has to be the complete address of the kubernetes cluster (scheme://hostname:port), not just the hostname,
		// because `overrides` are processed too late to be taken into account by `defaultServerUrlFor()`.
		// This basically replicates what defaultServerUrlFor() does with config but for overrides,
		// see https://github.com/kubernetes/client-go/blob/v12.0.0/rest/url_utils.go#L85-L87
		hasCA := len(overrides.ClusterInfo.CertificateAuthorityData) != 0
		hasCert := len(overrides.AuthInfo.ClientCertificateData) != 0
		defaultTLS := hasCA || hasCert || overrides.ClusterInfo.InsecureSkipTLSVerify
		host, _, err := restclient.DefaultServerURL(v, "", apimachineryschema.GroupVersion{}, defaultTLS)
		if err != nil {
			return nil, fmt.Errorf("failed to parse host: %s", err)
		}

		overrides.ClusterInfo.Server = host.String()
	}
	if v := d.Username.ValueString(); v != "" {
		overrides.AuthInfo.Username = v
	}
	if v := d.Password.ValueString(); v != "" {
		overrides.AuthInfo.Password = v
	}
	if v := d.ClientKey.ValueString(); v != "" {
		overrides.AuthInfo.ClientKeyData = bytes.NewBufferString(v).Bytes()
	}
	if v := d.Token.ValueString(); v != "" {
		overrides.AuthInfo.Token = v
	}

	// if v := d.Exec[0].Command.ValueString(); v != "" {
	// 	// exec := &clientcmdapi.ExecConfig{
	// 	// 	Command: d.Exec[0].Command.ValueString(),
	// 	// 	Args:    d.Exec[0].Args,
	// 	// }

	// 	// overrides.AuthInfo.Exec = exec
	// 	fmt.Println("TODO")
	// }
	if v := d.ProxyURL.ValueString(); v != "" {
		overrides.ClusterDefaults.ProxyURL = v
	}

	cc := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loader, overrides)
	cfg, err := cc.ClientConfig()
	if err != nil {
		log.Printf("[WARN] Invalid provider configuration was supplied. Provider operations likely to fail: %v", err)
		return nil, nil
	}

	return cfg, nil
}

func getServerVersion(connection *kubernetes.Clientset) (*gversion.Version, error) {
	sv, err := connection.ServerVersion()
	if err != nil {
		return nil, err
	}

	return gversion.NewVersion(sv.String())
}

func serverVersionGreaterThanOrEqual(connection *kubernetes.Clientset, version string) (bool, error) {
	sv, err := getServerVersion(connection)
	if err != nil {
		return false, err
	}
	// server version that we need to compare with
	cv, err := gversion.NewVersion(version)
	if err != nil {
		return false, err
	}

	return sv.GreaterThanOrEqual(cv), nil
}

func expandStringSlice(s []interface{}) []string {
	result := make([]string, len(s))
	for k, v := range s {
		// Handle the Terraform parser bug which turns empty strings in lists to nil.
		if v == nil {
			result[k] = ""
		} else {
			result[k] = v.(string)
		}
	}
	return result
}
