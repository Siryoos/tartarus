package tartarus

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func Provider() *schema.Provider {
	return &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"tartarus_sandbox": resourceSandbox(),
		},
		DataSourcesMap: map[string]*schema.Resource{
			// "tartarus_template": dataSourceTemplate(),
		},
	}
}
