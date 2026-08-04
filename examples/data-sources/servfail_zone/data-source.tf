# Read a single zone and all of its RRsets
data "servfail_zone" "example" {
  name = "example.com."
}

output "example_serial" {
  value = data.servfail_zone.example.serial
}

output "example_rrsets" {
  value = data.servfail_zone.example.rrsets
}
