package provider

import (
	"context"
	"encoding/json"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/likhner/terraform-provider-servfail/internal/client"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestWritePatchBody(t *testing.T) {
	shared := []client.Record{{Content: "192.0.2.1"}, {Content: "192.0.2.2"}}
	sole := []client.Record{{Content: "192.0.2.1"}}

	for _, tc := range []struct {
		name     string
		existing []client.Record
		content  string
		remove   bool
		want     client.RRset
	}{
		{"add keeps siblings and takes the planned ttl", shared, "192.0.2.3", false, client.RRset{
			Name: "www.example.com.", Type: "A", TTL: 60, ChangeType: client.ChangeTypeReplace,
			Records: []client.Record{{Content: "192.0.2.1"}, {Content: "192.0.2.2"}, {Content: "192.0.2.3", Disabled: true}},
		}},
		{"update rewrites our value in place", shared, "192.0.2.1", false, client.RRset{
			Name: "www.example.com.", Type: "A", TTL: 60, ChangeType: client.ChangeTypeReplace,
			Records: []client.Record{{Content: "192.0.2.2"}, {Content: "192.0.2.1", Disabled: true}},
		}},
		{"delete with siblings left keeps the live ttl", shared, "192.0.2.1", true, client.RRset{
			Name: "www.example.com.", Type: "A", TTL: 300, ChangeType: client.ChangeTypeReplace,
			Records: []client.Record{{Content: "192.0.2.2"}},
		}},
		{"deleting the last value drops the whole rrset", sole, "192.0.2.1", true, client.RRset{
			Name: "www.example.com.", Type: "A", ChangeType: client.ChangeTypeDelete,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got struct {
				RRsets []client.RRset `json:"rrsets"`
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "PATCH" {
					if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
						t.Error(err)
					}
					return
				}
				_ = json.NewEncoder(w).Encode(client.Zone{RRsets: []client.RRset{
					{Name: "www.example.com.", Type: "A", TTL: 300, Records: tc.existing},
				}})
			}))
			defer srv.Close()

			r := &recordResource{data: &providerData{client: client.New(srv.URL, "k", "test")}}
			var diags diag.Diagnostics
			r.write(context.Background(), &recordResourceModel{
				ServerID: types.StringValue("ns.example.com."),
				Zone:     types.StringValue("example.com."),
				Name:     types.StringValue("www.example.com."),
				Type:     types.StringValue("A"),
				TTL:      types.Int64Value(60),
				Content:  types.StringValue(tc.content),
				Disabled: types.BoolValue(true),
			}, tc.remove, &diags)

			if diags.HasError() {
				t.Fatal(diags)
			}
			if len(got.RRsets) != 1 || !reflect.DeepEqual(got.RRsets[0], tc.want) {
				t.Fatalf("PATCH body = %+v, want %+v", got.RRsets, tc.want)
			}
		})
	}
}

func TestQualifyName(t *testing.T) {
	cases := []struct{ name, zone, want string }{
		{"www", "example.com.", "www.example.com."},
		{"2.c.0.6", "3.e.0.1.ip6.arpa.", "2.c.0.6.3.e.0.1.ip6.arpa."},
		{"www.example.com.", "example.com.", "www.example.com."},
		{"www.example.com", "example.com.", "www.example.com."},
		{"example.com.", "example.com.", "example.com."},
		{"example.com", "example.com.", "example.com."},
		{"www", "example.com", "www.example.com."},
	}
	for _, c := range cases {
		if got := qualifyName(c.name, c.zone); got != c.want {
			t.Errorf("qualifyName(%q, %q) = %q, want %q", c.name, c.zone, got, c.want)
		}
	}
}
