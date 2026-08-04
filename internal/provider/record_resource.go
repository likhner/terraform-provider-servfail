package provider

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/likhner/terraform-provider-servfail/internal/client"
	"slices"
	"strings"
)

func qualifyName(name, zone string) string {
	n, z := strings.TrimSuffix(name, "."), strings.TrimSuffix(zone, ".")
	if n != z && !strings.HasSuffix(n, "."+z) {
		n += "." + z
	}
	return n + "."
}

type rejectSOA struct{}

func (rejectSOA) Description(context.Context) string         { return `must not be "SOA"` }
func (rejectSOA) MarkdownDescription(context.Context) string { return "must not be `SOA`" }
func (rejectSOA) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if strings.EqualFold(req.ConfigValue.ValueString(), "SOA") {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid record type",
			"The SOA record is managed by SERVFAIL and cannot be set here.")
	}
}

var (
	_ resource.ResourceWithConfigure   = (*recordResource)(nil)
	_ resource.ResourceWithImportState = (*recordResource)(nil)
)

type recordResource struct {
	data *providerData
}

type recordResourceModel struct {
	ServerID types.String `tfsdk:"server_id"`
	Zone     types.String `tfsdk:"zone"`
	Name     types.String `tfsdk:"name"`
	Type     types.String `tfsdk:"type"`
	TTL      types.Int64  `tfsdk:"ttl"`
	Content  types.String `tfsdk:"content"`
	Disabled types.Bool   `tfsdk:"disabled"`
}

func (r *recordResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_record"
}

func (r *recordResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a single DNS record value (one entry of a PowerDNS RRset).\n\nMultiple `servfail_record` resources may share a name+type, but the RRset has a single `ttl`; set the same `ttl` on all of them, or the last apply wins.",
		Attributes: map[string]schema.Attribute{
			"server_id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "PowerDNS server id — the zone's primary nameserver (its SOA MNAME). Each zone can live on a different primary, so set this per record; if unset it is looked up via the SERVFAIL primary endpoint.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"zone": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Canonical name of the zone this record belongs to (with trailing dot).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Record name. Either a fully-qualified name with the trailing dot (`www.example.com.`) or one relative to `zone` (`www`), which is qualified automatically. Use the zone name itself for the apex.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "DNS record type in upper case, e.g. `A`, `AAAA`, `MX`, `TXT`, `PTR`, `CNAME`, `CAA`. The `SOA` type is owned by the zone and is rejected here.",
				Validators:          []validator.String{rejectSOA{}},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"content": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Record value in canonical PowerDNS/BIND presentation form, e.g. `192.0.2.1`, `10 mail.example.com.`, or `\"some text\"` for TXT.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"ttl": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Time-to-live in seconds. Shared across all records of the same name+type.",
			},
			"disabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Whether the record is disabled (present in the zone but not served).",
			},
		},
	}
}

func (r *recordResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.data, _ = req.ProviderData.(*providerData)
}

func (r *recordResource) write(ctx context.Context, m *recordResourceModel, remove bool, diags *diag.Diagnostics) {
	zone := m.Zone.ValueString()
	serverID := r.data.resolveServer(ctx, m.ServerID.ValueString(), zone, diags)
	if diags.HasError() {
		return
	}
	m.ServerID = types.StringValue(serverID)

	name := qualifyName(m.Name.ValueString(), zone)
	rrtype := m.Type.ValueString()
	content := m.Content.ValueString()

	r.data.rrsetMu.Lock()
	defer r.data.rrsetMu.Unlock()

	existing, err := r.data.client.GetRRset(ctx, serverID, zone, name, rrtype)
	if err != nil {
		if remove && client.NotFound(err) {
			return
		}
		diags.AddError("Error reading RRset", err.Error())
		return
	}
	if remove && existing == nil {
		return
	}

	rrset := client.RRset{Name: name, Type: rrtype, ChangeType: client.ChangeTypeReplace}
	if remove {
		rrset.TTL, rrset.Records = existing.TTL, removeRecord(existing, content)
		if len(rrset.Records) == 0 {
			rrset = client.RRset{Name: name, Type: rrtype, ChangeType: client.ChangeTypeDelete}
		}
	} else {
		rrset.TTL = m.TTL.ValueInt64()
		rrset.Records = append(removeRecord(existing, content), client.Record{Content: content, Disabled: m.Disabled.ValueBool()})
	}

	if err := r.data.client.PatchRRsets(ctx, serverID, zone, []client.RRset{rrset}); err != nil {
		diags.AddError("Error writing record",
			fmt.Sprintf("Could not update %s %s in zone %s: %s", name, rrtype, zone, err))
	}
}

func (r *recordResource) apply(ctx context.Context, plan tfsdk.Plan, state *tfsdk.State, diags *diag.Diagnostics) {
	var model recordResourceModel
	diags.Append(plan.Get(ctx, &model)...)
	if diags.HasError() {
		return
	}

	r.write(ctx, &model, false, diags)
	if diags.HasError() {
		return
	}
	diags.Append(state.Set(ctx, &model)...)
}

func (r *recordResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.apply(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *recordResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state recordResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	zone := state.Zone.ValueString()
	serverID := r.data.resolveServer(ctx, state.ServerID.ValueString(), zone, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	state.ServerID = types.StringValue(serverID)

	content := state.Content.ValueString()

	rrset, err := r.data.client.GetRRset(ctx, serverID, zone, qualifyName(state.Name.ValueString(), zone), state.Type.ValueString())
	if err != nil {
		if client.NotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading record", err.Error())
		return
	}
	if rrset == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	i := slices.IndexFunc(rrset.Records, func(r client.Record) bool { return r.Content == content })
	if i < 0 {
		resp.State.RemoveResource(ctx)
		return
	}

	state.TTL = types.Int64Value(rrset.TTL)
	state.Disabled = types.BoolValue(rrset.Records[i].Disabled)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *recordResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.apply(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *recordResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state recordResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.write(ctx, &state, true, &resp.Diagnostics)
}

func (r *recordResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 4)
	if len(parts) != 4 {
		resp.Diagnostics.AddError("Invalid import ID",
			"Expected `zone/name/type/content`, got: "+req.ID)
		return
	}
	for i, attr := range []string{"zone", "name", "type", "content"} {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(attr), parts[i])...)
	}
}

func removeRecord(existing *client.RRset, content string) []client.Record {
	if existing == nil {
		return nil
	}
	return slices.DeleteFunc(slices.Clone(existing.Records), func(r client.Record) bool { return r.Content == content })
}
