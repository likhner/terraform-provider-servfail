package main

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/likhner/terraform-provider-servfail/internal/provider"
	"log"
)

var version = "dev"

func main() {
	opts := providerserver.ServeOpts{
		Address: "registry.opentofu.org/likhner/servfail",
	}
	if err := providerserver.Serve(context.Background(), provider.New(version), opts); err != nil {
		log.Fatal(err.Error())
	}
}
