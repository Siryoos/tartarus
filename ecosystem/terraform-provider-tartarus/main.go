package main

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/plugin"
	"github.com/tartarus/terraform-provider-tartarus/tartarus"
)

func main() {
	plugin.Serve(&plugin.ServeOpts{
		ProviderFunc: tartarus.Provider,
	})
}
