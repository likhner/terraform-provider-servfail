package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseErrorMessage(t *testing.T) {
	cases := map[string]string{
		`{"error":"Not found"}`:  "Not found",
		"Not Found":              "Not Found",
		"":                       "",
		strings.Repeat("x", 400): strings.Repeat("x", 300) + "...",
	}
	for in, want := range cases {
		if got := parseErrorMessage([]byte(in)); got != want {
			t.Errorf("parseErrorMessage(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGetRRset_FiltersByNameAndType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "secret" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_ = json.NewEncoder(w).Encode(Zone{
			Name: "example.com.",
			RRsets: []RRset{
				{Name: "www.example.com.", Type: "AAAA", TTL: 60, Records: []Record{{Content: "2001:db8::1"}}},
				{Name: "other.example.com.", Type: "A", TTL: 60, Records: []Record{{Content: "192.0.2.9"}}},
				{Name: "www.example.com.", Type: "A", TTL: 300, Records: []Record{{Content: "192.0.2.1"}}},
			},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "secret", "test")
	rr, err := c.GetRRset(context.Background(), "ns.example.com.", "example.com.", "www.example.com.", "A")
	if err != nil {
		t.Fatal(err)
	}
	if rr == nil || rr.TTL != 300 || len(rr.Records) != 1 {
		t.Fatalf("unexpected rrset: %+v", rr)
	}
}

func TestGetRRset_NotPresentReturnsNil(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Zone{Name: "example.com.", RRsets: nil})
	}))
	defer srv.Close()

	c := New(srv.URL, "secret", "test")
	rr, err := c.GetRRset(context.Background(), "ns.", "example.com.", "missing.example.com.", "A")
	if err != nil {
		t.Fatal(err)
	}
	if rr != nil {
		t.Fatalf("expected nil, got %+v", rr)
	}
}

func TestDo_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":"Conflicts with pre-existing RRset"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "secret", "test")
	_, err := c.GetZone(context.Background(), "ns.", "example.com.")
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(apiErr.Message, "Conflicts with pre-existing RRset") {
		t.Fatalf("unexpected: %+v", apiErr)
	}
}
