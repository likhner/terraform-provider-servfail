package client

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sync/singleflight"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

const DefaultEndpoint = "https://beta.servfail.network/api/v1"

type Zone struct {
	Name        string  `json:"name,omitempty" tfsdk:"name"`
	Kind        string  `json:"kind,omitempty" tfsdk:"kind"`
	Serial      int64   `json:"serial,omitempty" tfsdk:"serial"`
	DNSSEC      bool    `json:"dnssec" tfsdk:"dnssec"`
	NSEC3Param  string  `json:"nsec3param,omitempty" tfsdk:"nsec3param"`
	NSEC3Narrow bool    `json:"nsec3narrow,omitempty" tfsdk:"nsec3narrow"`
	RRsets      []RRset `json:"rrsets,omitempty" tfsdk:"rrsets"`
}

type RRset struct {
	Name       string   `json:"name" tfsdk:"name"`
	Type       string   `json:"type" tfsdk:"type"`
	TTL        int64    `json:"ttl,omitempty" tfsdk:"ttl"`
	ChangeType string   `json:"changetype,omitempty" tfsdk:"-"`
	Records    []Record `json:"records,omitempty" tfsdk:"records"`
}

type Record struct {
	Content  string `json:"content" tfsdk:"content"`
	Disabled bool   `json:"disabled" tfsdk:"disabled"`
}

const (
	ChangeTypeReplace = "REPLACE"
	ChangeTypeDelete  = "DELETE"
)

type Client struct {
	httpClient *http.Client
	endpoint   string
	apiKey     string
	userAgent  string

	sf singleflight.Group

	primaryCache sync.Map
}

func New(endpoint, apiKey, version string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		endpoint:   strings.TrimRight(cmp.Or(endpoint, DefaultEndpoint), "/"),
		apiKey:     apiKey,
		userAgent:  "terraform-provider-servfail/" + version,
	}
}

type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string { return e.Message }

func NotFound(err error) bool {
	var e *APIError
	return errors.As(err, &e) && e.StatusCode == http.StatusNotFound
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding request body: %w", err)
		}
		reqBody = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, reqBody)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("X-Api-Key", c.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("performing request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{StatusCode: resp.StatusCode, Message: fmt.Sprintf("servfail api: %s %s -> %d: %s",
			method, path, resp.StatusCode, cmp.Or(parseErrorMessage(raw), http.StatusText(resp.StatusCode)))}
	}

	if out != nil && len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decoding response body: %w", err)
		}
	}
	return nil
}

func parseErrorMessage(raw []byte) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return ""
	}
	var perr struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(raw, &perr) == nil && perr.Error != "" {
		return perr.Error
	}
	if len(trimmed) > 300 {
		return trimmed[:300] + "..."
	}
	return trimmed
}

func zonePath(server, zoneID string) string {
	return "/servers/" + url.PathEscape(server) + "/zones/" + url.PathEscape(zoneID)
}

func (c *Client) ListZones(ctx context.Context, server string) ([]Zone, error) {
	var zones []Zone
	if err := c.do(ctx, "GET", "/servers/"+url.PathEscape(server)+"/zones", nil, &zones); err != nil {
		return nil, err
	}
	return zones, nil
}

func (c *Client) GetZone(ctx context.Context, server, zoneID string) (*Zone, error) {
	v, err, _ := c.sf.Do("zone:"+server+"/"+zoneID, func() (any, error) {
		var zone Zone
		if err := c.do(ctx, "GET", zonePath(server, zoneID), nil, &zone); err != nil {
			return nil, err
		}
		return &zone, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*Zone), nil
}

func (c *Client) GetRRset(ctx context.Context, server, zoneID, name, rrtype string) (*RRset, error) {
	zone, err := c.GetZone(ctx, server, zoneID)
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(zone.RRsets, func(rr RRset) bool { return rr.Name == name && rr.Type == rrtype })
	if i < 0 {
		return nil, nil
	}
	return &zone.RRsets[i], nil
}

func (c *Client) GetPrimary(ctx context.Context, zone string) (string, error) {
	if v, ok := c.primaryCache.Load(zone); ok {
		return v.(string), nil
	}
	v, err, _ := c.sf.Do("primary:"+zone, func() (any, error) {
		if v, ok := c.primaryCache.Load(zone); ok {
			return v.(string), nil
		}
		var out struct {
			Primary string `json:"primary"`
		}
		if err := c.do(ctx, "GET", "/servfail/primary/"+url.PathEscape(zone), nil, &out); err != nil {
			return "", err
		}
		if out.Primary != "" {
			c.primaryCache.Store(zone, out.Primary)
		}
		return out.Primary, nil
	})
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

func (c *Client) PatchRRsets(ctx context.Context, server, zoneID string, rrsets []RRset) error {
	body := struct {
		RRsets []RRset `json:"rrsets"`
	}{RRsets: rrsets}
	return c.do(ctx, "PATCH", zonePath(server, zoneID), body, nil)
}
