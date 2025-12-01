package tartarus

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceSandbox() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceSandboxCreate,
		ReadContext:   resourceSandboxRead,
		UpdateContext: resourceSandboxUpdate,
		DeleteContext: resourceSandboxDelete,
		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"image": {
				Type:     schema.TypeString,
				Required: true,
			},
			"status": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func resourceSandboxCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	// Implementation would go here
	d.SetId("mock-id")
	return nil
}

func resourceSandboxRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	return nil
}

func resourceSandboxUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	return nil
}

func resourceSandboxDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	return nil
}
