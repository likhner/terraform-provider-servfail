package provider

import (
	"cmp"
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/likhner/terraform-provider-servfail/internal/client"
	"os"
	"strings"
	"sync"
)

const (
	envEndpoint = "SERVFAIL_ENDPOINT"
	envAPIToken = "SERVFAIL_API_TOKEN"
)

type providerData struct {
	client  *client.Client
	rrsetMu sync.Mutex
}

func (pd *providerData) resolveServer(ctx context.Context, resourceServerID, zone string, diags *diag.Diagnostics) string {
	if resourceServerID != "" {
		return resourceServerID
	}
	primary, err := pd.client.GetPrimary(ctx, zone)
	if err != nil {
		diags.AddError("Could not resolve server_id",
			fmt.Sprintf("No server_id set and looking up the primary for zone %q failed: %s", zone, err))
		return ""
	}
	if primary == "" {
		diags.AddError("Missing server_id",
			fmt.Sprintf("The primary lookup for zone %q returned empty; set server_id explicitly.", zone))
	}
	return primary
}

type servfailProvider struct {
	version string
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &servfailProvider{version: version}
	}
}

type providerModel struct {
	Endpoint types.String `tfsdk:"endpoint"`
	APIToken types.String `tfsdk:"api_token"`
}

func (p *servfailProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "servfail"
	resp.Version = p.version
}

func (p *servfailProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "SERVFAIL provider manages DNS records on the [SERVFAIL](https://servfail.network/) authoritative nameserver network via its PowerDNS-compatible HTTP API.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Base URL of the PowerDNS-compatible API, including the `/api/v1` suffix. Defaults to `" + client.DefaultEndpoint + "`. May also be set via the `" + envEndpoint + "` environment variable.",
			},
			"api_token": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "SERVFAIL API token (generated on the SERVFAIL settings page). May also be set via the `" + envAPIToken + "` environment variable (recommended).",
			},
		},
	}
}

func (p *servfailProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint := cmp.Or(strings.TrimSpace(cfg.Endpoint.ValueString()), os.Getenv(envEndpoint))
	apiToken := cmp.Or(cfg.APIToken.ValueString(), os.Getenv(envAPIToken))

	if apiToken == "" {
		resp.Diagnostics.AddAttributeError(path.Root("api_token"),
			"Missing API token",
			"No API token was provided. Set the `api_token` provider argument or the "+envAPIToken+" environment variable.")
		return
	}

	data := &providerData{client: client.New(endpoint, apiToken, p.version)}
	resp.ResourceData = data
	resp.DataSourceData = data
}

func (p *servfailProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		func() resource.Resource { return &recordResource{} },
	}
}

func (p *servfailProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		func() datasource.DataSource { return &zoneDataSource{} },
		func() datasource.DataSource { return &zonesDataSource{} },
	}
}
