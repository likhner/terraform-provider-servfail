package provider

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/likhner/terraform-provider-servfail/internal/client"
	"testing"
)

func TestDataSourceModelsMatchSchema(t *testing.T) {
	ctx := context.Background()
	check := func(name string, ds datasource.DataSource, model any) {
		var resp datasource.SchemaResponse
		ds.Schema(ctx, datasource.SchemaRequest{}, &resp)
		state := tfsdk.State{Schema: resp.Schema}
		if diags := state.Set(ctx, model); diags.HasError() {
			t.Errorf("%s: %v", name, diags)
		}
	}

	check("zone", &zoneDataSource{}, &client.Zone{
		Name:   "example.com.",
		Serial: 1,
		RRsets: []client.RRset{
			{Name: "www.example.com.", Type: "A", TTL: 300, Records: []client.Record{{Content: "192.0.2.1"}}},
		},
	})
	check("zones", &zonesDataSource{}, &zonesDataSourceModel{
		Zones: []zoneSummary{{Name: "example.com.", Kind: "Native", Serial: 1}},
	})
}
