package eventgrid

import (
	"fmt"
	"log"
	"regexp"
	"time"

	"github.com/hashicorp/go-azure-helpers/response"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/helper/validation"
	"github.com/jcanizalez/azure-sdk-for-go/services/preview/eventgrid/mgmt/2020-10-15-preview/eventgrid"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/azure"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/tf"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/clients"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/services/eventgrid/parse"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/tags"
	azSchema "github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/tf/schema"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/tf/suppress"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/timeouts"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/utils"
)

func resourceEventGridSystemTopic() *schema.Resource {
	return &schema.Resource{
		Create: resourceEventGridSystemTopicCreateUpdate,
		Read:   resourceEventGridSystemTopicRead,
		Update: resourceEventGridSystemTopicCreateUpdate,
		Delete: resourceEventGridSystemTopicDelete,

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(30 * time.Minute),
			Read:   schema.DefaultTimeout(5 * time.Minute),
			Update: schema.DefaultTimeout(30 * time.Minute),
			Delete: schema.DefaultTimeout(30 * time.Minute),
		},

		Importer: azSchema.ValidateResourceIDPriorToImport(func(id string) error {
			_, err := parse.SystemTopicID(id)
			return err
		}),

		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				ValidateFunc: validation.All(
					validation.StringIsNotEmpty,
					validation.StringMatch(
						regexp.MustCompile("^[-a-zA-Z0-9]{3,128}$"),
						"EventGrid Topics name must be 3 - 128 characters long, contain only letters, numbers and hyphens.",
					),
				),
			},

			"location": azure.SchemaLocation(),

			"resource_group_name": azure.SchemaResourceGroupName(),

			"source_arm_resource_id": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: azure.ValidateResourceID,
			},

			"topic_type": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				ValidateFunc: validation.StringInSlice([]string{
					"Microsoft.AppConfiguration.ConfigurationStores",
					"Microsoft.Communication.CommunicationServices",
					"Microsoft.ContainerRegistry.Registries",
					"Microsoft.Devices.IoTHubs",
					"Microsoft.EventGrid.Domains",
					"Microsoft.EventGrid.Topics",
					"Microsoft.Eventhub.Namespaces",
					"Microsoft.KeyVault.vaults",
					"Microsoft.MachineLearningServices.Workspaces",
					"Microsoft.Maps.Accounts",
					"Microsoft.Media.MediaServices",
					"Microsoft.Resources.ResourceGroups",
					"Microsoft.Resources.Subscriptions",
					"Microsoft.ServiceBus.Namespaces",
					"Microsoft.SignalRService.SignalR",
					"Microsoft.Storage.StorageAccounts",
					"Microsoft.Web.ServerFarms",
					"Microsoft.Web.Sites",
				}, false),
			},

			"metric_arm_resource_id": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"identity": {
				Type:     schema.TypeList,
				Optional: true,
				Computed: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"principal_id": {
							Type:     schema.TypeString,
							Computed: true,
						},

						"tenant_id": {
							Type:     schema.TypeString,
							Computed: true,
						},

						"type": {
							Type:             schema.TypeString,
							Optional:         true,
							DiffSuppressFunc: suppress.CaseDifference,
							ValidateFunc: validation.StringInSlice([]string{
								"SystemAssigned",
							}, true),
						},
					},
				},
			},

			"tags": tags.Schema(),
		},
	}
}

func resourceEventGridSystemTopicCreateUpdate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).EventGrid.SystemTopicsClient
	ctx, cancel := timeouts.ForCreateUpdate(meta.(*clients.Client).StopContext, d)
	defer cancel()

	name := d.Get("name").(string)
	resourceGroup := d.Get("resource_group_name").(string)
	source := d.Get("source_arm_resource_id").(string)
	topicType := d.Get("topic_type").(string)

	if d.IsNewResource() {
		existing, err := client.Get(ctx, resourceGroup, name)
		if err != nil {
			if !utils.ResponseWasNotFound(existing.Response) {
				return fmt.Errorf("Error checking for presence of existing Event Grid System Topic %q (Resource Group %q): %s", name, resourceGroup, err)
			}
		}

		if existing.ID != nil && *existing.ID != "" {
			return tf.ImportAsExistsError("azurerm_eventgrid_system_topic", *existing.ID)
		}
	}

	location := azure.NormalizeLocation(d.Get("location").(string))
	t := d.Get("tags").(map[string]interface{})

	systemTopic := eventgrid.SystemTopic{
		Location: &location,
		SystemTopicProperties: &eventgrid.SystemTopicProperties{
			Source:    &source,
			TopicType: &topicType,
		},
		Tags: tags.Expand(t),
	}

	if v, ok := d.GetOk("identity"); ok {
		systemTopic.Identity = expandSystemTopicIdentity(v.([]interface{}))
	}

	log.Printf("[INFO] preparing arguments for AzureRM Event Grid System Topic creation with Properties: %+v.", systemTopic)

	future, err := client.CreateOrUpdate(ctx, resourceGroup, name, systemTopic)
	if err != nil {
		return err
	}

	if err = future.WaitForCompletionRef(ctx, client.Client); err != nil {
		return err
	}

	read, err := client.Get(ctx, resourceGroup, name)
	if err != nil {
		return err
	}
	if read.ID == nil {
		return fmt.Errorf("Cannot read Event Grid System Topic %s (resource group %s) ID", name, resourceGroup)
	}

	d.SetId(*read.ID)

	return resourceEventGridSystemTopicRead(d, meta)
}

func resourceEventGridSystemTopicRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).EventGrid.SystemTopicsClient
	ctx, cancel := timeouts.ForRead(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := parse.SystemTopicID(d.Id())
	if err != nil {
		return err
	}

	resp, err := client.Get(ctx, id.ResourceGroup, id.Name)
	if err != nil {
		if utils.ResponseWasNotFound(resp.Response) {
			log.Printf("[WARN] Event Grid System Topic '%s' was not found (resource group '%s')", id.Name, id.ResourceGroup)
			d.SetId("")
			return nil
		}

		return fmt.Errorf("Error making Read request on Event Grid System Topic '%s': %+v", id.Name, err)
	}

	d.Set("name", resp.Name)
	d.Set("resource_group_name", id.ResourceGroup)
	if location := resp.Location; location != nil {
		d.Set("location", azure.NormalizeLocation(*location))
	}

	if props := resp.SystemTopicProperties; props != nil {
		d.Set("source_arm_resource_id", props.Source)
		d.Set("topic_type", props.TopicType)
		d.Set("metric_arm_resource_id", props.MetricResourceID)
	}

	d.Set("identity", flattenSystemTopicIdentity(resp.Identity))

	return tags.FlattenAndSet(d, resp.Tags)
}

func resourceEventGridSystemTopicDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).EventGrid.SystemTopicsClient
	ctx, cancel := timeouts.ForDelete(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := parse.SystemTopicID(d.Id())
	if err != nil {
		return err
	}

	future, err := client.Delete(ctx, id.ResourceGroup, id.Name)
	if err != nil {
		if response.WasNotFound(future.Response()) {
			return nil
		}
		return fmt.Errorf("Error deleting Event Grid System Topic %q: %+v", id.Name, err)
	}

	if err = future.WaitForCompletionRef(ctx, client.Client); err != nil {
		if response.WasNotFound(future.Response()) {
			return nil
		}
		return fmt.Errorf("Error deleting Event Grid System Topic %q: %+v", id.Name, err)
	}

	return nil
}

func expandSystemTopicIdentity(input []interface{}) *eventgrid.SystemTopicIdentity {
	if len(input) == 0 {
		return &eventgrid.SystemTopicIdentity{
			Type: eventgrid.ManagedIdentityTypeNone,
		}
	}

	raw := input[0].(map[string]interface{})

	identity := eventgrid.SystemTopicIdentity{
		Type: eventgrid.ManagedIdentityType(raw["type"].(string)),
	}

	return &identity
}

func flattenSystemTopicIdentity(input *eventgrid.SystemTopicIdentity) []interface{} {
	if input == nil || input.Type == eventgrid.ManagedIdentityTypeNone {
		return []interface{}{}
	}

	principalID := ""
	if input.PrincipalID != nil {
		principalID = *input.PrincipalID
	}

	tenantID := ""
	if input.TenantID != nil {
		tenantID = *input.TenantID
	}

	return []interface{}{
		map[string]interface{}{
			"type":         string(input.Type),
			"principal_id": principalID,
			"tenant_id":    tenantID,
		},
	}
}
