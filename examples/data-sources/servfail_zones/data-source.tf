# List every zone on a server (server_id is required here)
data "servfail_zones" "all" {
  server_id = "sakamoto.pl."
}

output "all_zone_names" {
  value = [for z in data.servfail_zones.all.zones : z.name]
}
