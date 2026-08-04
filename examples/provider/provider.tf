terraform {
  required_providers {
    servfail = {
      source = "likhner/servfail"
    }
  }
}

# api_token is read from SERVFAIL_API_TOKEN
provider "servfail" {}
