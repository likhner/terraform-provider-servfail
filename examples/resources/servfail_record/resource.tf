# Single A record
resource "servfail_record" "www_v4" {
  zone    = "example.com."
  name    = "www.example.com."
  type    = "A"
  ttl     = 3600
  content = "192.0.2.10"
}

# Two A records sharing the same name+type (one shared RRset / TTL)
# Provider read-modify-writes the RRset and serializes these internally
resource "servfail_record" "www_v4_secondary" {
  zone    = "example.com."
  name    = "www.example.com."
  type    = "A"
  ttl     = 3600
  content = "192.0.2.11"
}

# AAAA record
resource "servfail_record" "www_v6" {
  zone    = "example.com."
  name    = "www.example.com."
  type    = "AAAA"
  ttl     = 3600
  content = "2001:db8::10"
}

# MX record (priority is part of the content)
resource "servfail_record" "mx" {
  zone    = "example.com."
  name    = "example.com."
  type    = "MX"
  ttl     = 3600
  content = "10 mail.example.com."
}

# Reverse-DNS PTR record
resource "servfail_record" "ptr" {
  zone    = "3.e.0.1.c.5.8.f.0.a.2.ip6.arpa."
  name    = "5.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.c.3.e.0.1.c.5.8.f.0.a.2.ip6.arpa."
  type    = "PTR"
  ttl     = 3600
  content = "host.example.com."
}
