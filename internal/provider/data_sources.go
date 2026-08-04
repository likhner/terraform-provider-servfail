package provider

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
)

var (
	_ datasource.DataSourceWithConfigure = (*zoneDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*zonesDataSource)(nil)
)

type zoneDataSource struct {
	data *providerData
}

func (d *zoneDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_zone"
}

func (d *zoneDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single DNS zone and all of its RRsets from SERVFAIL.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Canonical zone name including the trailing dot.",
			},
			"kind":        schema.StringAttribute{Computed: true, MarkdownDescription: "Zone kind."},
			"dnssec":      schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the zone is DNSSEC-signed."},
			"nsec3param":  schema.StringAttribute{Computed: true, MarkdownDescription: "NSEC3PARAM contents."},
			"nsec3narrow": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether NSEC3 narrow mode is enabled."},
			"serial":      schema.Int64Attribute{Computed: true, MarkdownDescription: "SOA serial."},
			"rrsets": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "All resource record sets in the zone.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{Computed: true, MarkdownDescription: "RRset name."},
						"type": schema.StringAttribute{Computed: true, MarkdownDescription: "RRset type."},
						"ttl":  schema.Int64Attribute{Computed: true, MarkdownDescription: "RRset TTL."},
						"records": schema.ListNestedAttribute{
							Computed:            true,
							MarkdownDescription: "Record values within the RRset.",
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"content":  schema.StringAttribute{Computed: true, MarkdownDescription: "Record value."},
									"disabled": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether disabled."},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (d *zoneDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.data, _ = req.ProviderData.(*providerData)
}

func (d *zoneDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var name string
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("name"), &name)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serverID := d.data.resolveServer(ctx, "", name, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	zone, err := d.data.client.GetZone(ctx, serverID, name)
	if err != nil {
		resp.Diagnostics.AddError("Error reading zone",
			fmt.Sprintf("Could not read zone %q: %s", name, err))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, zone)...)
}

type zonesDataSource struct {
	data *providerData
}

type zoneSummary struct {
	Name   string `tfsdk:"name"`
	Kind   string `tfsdk:"kind"`
	DNSSEC bool   `tfsdk:"dnssec"`
	Serial int64  `tfsdk:"serial"`
}

type zonesDataSourceModel struct {
	ServerID string        `tfsdk:"server_id"`
	Zones    []zoneSummary `tfsdk:"zones"`
}

func (d *zonesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_zones"
}

func (d *zonesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the zones you own or that are shared with you on a given `server_id`.",
		Attributes: map[string]schema.Attribute{
			"server_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Primary nameserver to list zones from, with trailing dot (e.g. `ns1.famfo.xyz.`). Find it in a zone's SOA MNAME or the web UI; an unknown id returns an empty list, not an error.",
			},
			"zones": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The zones on the server.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name":   schema.StringAttribute{Computed: true, MarkdownDescription: "Zone name."},
						"kind":   schema.StringAttribute{Computed: true, MarkdownDescription: "Zone kind."},
						"dnssec": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether DNSSEC-signed."},
						"serial": schema.Int64Attribute{Computed: true, MarkdownDescription: "SOA serial."},
					},
				},
			},
		},
	}
}

func (d *zonesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.data, _ = req.ProviderData.(*providerData)
}

func (d *zonesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var serverID string
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("server_id"), &serverID)...)
	if resp.Diagnostics.HasError() {
		return
	}

	zones, err := d.data.client.ListZones(ctx, serverID)
	if err != nil {
		resp.Diagnostics.AddError("Error listing zones",
			fmt.Sprintf("Could not list zones on server %q: %s", serverID, err))
		return
	}

	cfg := zonesDataSourceModel{ServerID: serverID, Zones: make([]zoneSummary, 0, len(zones))}
	for _, z := range zones {
		cfg.Zones = append(cfg.Zones, zoneSummary{Name: z.Name, Kind: z.Kind, DNSSEC: z.DNSSEC, Serial: z.Serial})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
