# 🕯 SERVFAIL OpenTofu

OpenTofu provider for [SERVFAIL](https://servfail.network/) authoritative nameserver network, built on its [PowerDNS-compatible HTTP API](https://docs.servfail.network/api/). It manages DNS records, and exposes data sources for reading zones.

## Configuration

```hcl
terraform {
  required_providers {
    servfail = {
      source = "likhner/servfail"
    }
  }
}

provider "servfail" {
  endpoint  = "https://beta.servfail.network/api/v1" # optional (this is default)
  api_token = var.servfail_api_token
}
```

| Argument    | Environment variable | Required | Description                                                   |
|-------------|----------------------|----------|---------------------------------------------------------------|
| `api_token` | `SERVFAIL_API_TOKEN` | yes      | [SERVFAIL API token](https://beta.servfail.network/settings/) |
| `endpoint`  | `SERVFAIL_ENDPOINT`  | no       | Full API base URL                                             |

## Resources

Zones aren't managed here, API is read-only for zones (`POST`, `PUT`, `DELETE` return `405`). Create and configure zones in SERVFAIL web UI, and read them with [`servfail_zone` data source](#data-sources).

### `servfail_record`

Manages a single record value (one entry of a PowerDNS RRset).

```hcl
resource "servfail_record" "www_v4" {
  zone    = "example.com."
  name    = "www" # relative to zone; "www.example.com." also works
  type    = "A"
  ttl     = 3600
  content = "192.0.2.10"
}
```

See [`examples/resources/servfail_record`](./examples/resources/servfail_record)
for AAAA, MX, PTR and shared-RRset examples.

Import (`zone/name/type/content`):

```sh
tofu import servfail_record.www_v4 'example.com./www.example.com./A/192.0.2.10'
```

## Data sources

- [`servfail_zone`](./examples/data-sources/servfail_zone): reads one zone and all of its RRsets (`kind`, `serial`, `dnssec`, `nsec3param`, `nsec3narrow`, `rrsets`)
- [`servfail_zones`](./examples/data-sources/servfail_zones): lists zones you own or that are shared with you on a given `server_id` (required; find id in a zone SOA MNAME or web UI; a wrong id returns an empty list, not an error)

## SERVFAIL API limitations

- RRset comments are not supported
- `$ORIGIN` and `$TTL` are informational only
- PowerDNS `rrset_name`/`rrset_type` filters do nothing
- [DNSSEC keys](https://docs.servfail.network/dnssec/), [DDNS](https://docs.servfail.network/ddns/) and zone sharing are web-UI-only, but DNSSEC status is readable via `dnssec`, `nsec3param`, `nsec3narrow` attributes of `servfail_zone` data source

## Building & local testing

Requires Go 1.25+ and OpenTofu.

```sh
go build -o bin/ .
go test ./...
```

Use a dev override so OpenTofu loads your local build:

`dev.tfrc`:
```hcl
provider_installation {
  dev_overrides {
    "likhner/servfail" = "/absolute/path/to/repo/bin"
  }
  direct {}
}
```

```sh
export TF_CLI_CONFIG_FILE=$PWD/dev.tfrc
export SERVFAIL_API_TOKEN=... # your token
cd examples/provider          # terraform{} block; add a resource to exercise it
tofu plan
```

---

This provider is not affiliated with or endorsed by Project SERVFAIL.
