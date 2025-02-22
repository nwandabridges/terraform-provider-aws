package codepipeline

import (
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/codepipeline"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/hashicorp/terraform-provider-aws/internal/conns"
	tftags "github.com/hashicorp/terraform-provider-aws/internal/tags"
	"github.com/hashicorp/terraform-provider-aws/internal/tfresource"
	"github.com/hashicorp/terraform-provider-aws/internal/verify"
)

func ResourceWebhook() *schema.Resource {
	return &schema.Resource{
		Create: resourceWebhookCreate,
		Read:   resourceWebhookRead,
		Update: resourceWebhookUpdate,
		Delete: resourceWebhookDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"authentication": {
				Type:     schema.TypeString,
				ForceNew: true,
				Required: true,
				ValidateFunc: validation.StringInSlice([]string{
					codepipeline.WebhookAuthenticationTypeGithubHmac,
					codepipeline.WebhookAuthenticationTypeIp,
					codepipeline.WebhookAuthenticationTypeUnauthenticated,
				}, false),
			},
			"authentication_configuration": {
				Type:     schema.TypeList,
				MaxItems: 1,
				MinItems: 1,
				Optional: true,
				ForceNew: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"secret_token": {
							Type:      schema.TypeString,
							Optional:  true,
							ForceNew:  true,
							Sensitive: true,
						},
						"allowed_ip_range": {
							Type:         schema.TypeString,
							Optional:     true,
							ForceNew:     true,
							ValidateFunc: validation.IsCIDRNetwork(0, 32),
						},
					},
				},
			},
			"filter": {
				Type:     schema.TypeSet,
				ForceNew: true,
				Required: true,
				MinItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"json_path": {
							Type:     schema.TypeString,
							Required: true,
						},

						"match_equals": {
							Type:     schema.TypeString,
							Required: true,
						},
					},
				},
			},
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"url": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"target_action": {
				Type:     schema.TypeString,
				ForceNew: true,
				Required: true,
			},
			"target_pipeline": {
				Type:     schema.TypeString,
				ForceNew: true,
				Required: true,
			},
			"tags":     tftags.TagsSchema(),
			"tags_all": tftags.TagsSchemaComputed(),
		},

		CustomizeDiff: verify.SetTagsDiff,
	}
}

func extractCodePipelineWebhookRules(filters *schema.Set) []*codepipeline.WebhookFilterRule {
	var rules []*codepipeline.WebhookFilterRule

	for _, f := range filters.List() {
		r := f.(map[string]interface{})
		filter := codepipeline.WebhookFilterRule{
			JsonPath:    aws.String(r["json_path"].(string)),
			MatchEquals: aws.String(r["match_equals"].(string)),
		}

		rules = append(rules, &filter)
	}

	return rules
}

func extractCodePipelineWebhookAuthConfig(authType string, authConfig map[string]interface{}) *codepipeline.WebhookAuthConfiguration {
	var conf codepipeline.WebhookAuthConfiguration
	switch authType {
	case codepipeline.WebhookAuthenticationTypeIp:
		conf.AllowedIPRange = aws.String(authConfig["allowed_ip_range"].(string))
	case codepipeline.WebhookAuthenticationTypeGithubHmac:
		conf.SecretToken = aws.String(authConfig["secret_token"].(string))
	}

	return &conf
}

func resourceWebhookCreate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*conns.AWSClient).CodePipelineConn
	defaultTagsConfig := meta.(*conns.AWSClient).DefaultTagsConfig
	tags := defaultTagsConfig.MergeTags(tftags.New(d.Get("tags").(map[string]interface{})))
	authType := d.Get("authentication").(string)

	var authConfig map[string]interface{}
	if v, ok := d.GetOk("authentication_configuration"); ok {
		l := v.([]interface{})
		authConfig = l[0].(map[string]interface{})
	}

	request := &codepipeline.PutWebhookInput{
		Webhook: &codepipeline.WebhookDefinition{
			Authentication:              aws.String(authType),
			Filters:                     extractCodePipelineWebhookRules(d.Get("filter").(*schema.Set)),
			Name:                        aws.String(d.Get("name").(string)),
			TargetAction:                aws.String(d.Get("target_action").(string)),
			TargetPipeline:              aws.String(d.Get("target_pipeline").(string)),
			AuthenticationConfiguration: extractCodePipelineWebhookAuthConfig(authType, authConfig),
		},
		Tags: Tags(tags.IgnoreAWS()),
	}

	webhook, err := conn.PutWebhook(request)
	if err != nil {
		return fmt.Errorf("Error creating webhook: %s", err)
	}

	d.SetId(aws.StringValue(webhook.Webhook.Arn))

	return resourceWebhookRead(d, meta)
}

func GetWebhook(conn *codepipeline.CodePipeline, arn string) (*codepipeline.ListWebhookItem, error) {
	var nextToken string

	for {
		input := &codepipeline.ListWebhooksInput{
			MaxResults: aws.Int64(int64(60)),
		}
		if nextToken != "" {
			input.NextToken = aws.String(nextToken)
		}

		out, err := conn.ListWebhooks(input)
		if err != nil {
			return nil, err
		}

		for _, w := range out.Webhooks {
			if arn == aws.StringValue(w.Arn) {
				return w, nil
			}
		}

		if out.NextToken == nil {
			break
		}

		nextToken = aws.StringValue(out.NextToken)
	}

	return nil, &resource.NotFoundError{
		Message: fmt.Sprintf("No webhook with ARN %s found", arn),
	}
}

func flattenCodePipelineWebhookFilters(filters []*codepipeline.WebhookFilterRule) []interface{} {
	results := []interface{}{}
	for _, filter := range filters {
		f := map[string]interface{}{
			"json_path":    aws.StringValue(filter.JsonPath),
			"match_equals": aws.StringValue(filter.MatchEquals),
		}
		results = append(results, f)
	}

	return results
}

func flattenCodePipelineWebhookAuthenticationConfiguration(authConfig *codepipeline.WebhookAuthConfiguration) []interface{} {
	conf := map[string]interface{}{}
	if authConfig.AllowedIPRange != nil {
		conf["allowed_ip_range"] = aws.StringValue(authConfig.AllowedIPRange)
	}

	if authConfig.SecretToken != nil {
		conf["secret_token"] = aws.StringValue(authConfig.SecretToken)
	}

	var results []interface{}
	if len(conf) > 0 {
		results = append(results, conf)
	}

	return results
}

func resourceWebhookRead(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*conns.AWSClient).CodePipelineConn
	defaultTagsConfig := meta.(*conns.AWSClient).DefaultTagsConfig
	ignoreTagsConfig := meta.(*conns.AWSClient).IgnoreTagsConfig

	arn := d.Id()
	webhook, err := GetWebhook(conn, arn)

	if tfresource.NotFound(err) {
		log.Printf("[WARN] CodePipeline Webhook (%s) not found, removing from state", d.Id())
		d.SetId("")
		return nil
	}

	if err != nil {
		return fmt.Errorf("error getting CodePipeline Webhook (%s): %s", d.Id(), err)
	}

	name := aws.StringValue(webhook.Definition.Name)
	if name == "" {
		return fmt.Errorf("Webhook not found: %s", arn)
	}

	d.Set("name", name)
	d.Set("url", webhook.Url)

	if err := d.Set("target_action", webhook.Definition.TargetAction); err != nil {
		return err
	}

	if err := d.Set("target_pipeline", webhook.Definition.TargetPipeline); err != nil {
		return err
	}

	if err := d.Set("authentication", webhook.Definition.Authentication); err != nil {
		return err
	}

	if err := d.Set("authentication_configuration", flattenCodePipelineWebhookAuthenticationConfiguration(webhook.Definition.AuthenticationConfiguration)); err != nil {
		return fmt.Errorf("error setting authentication_configuration: %s", err)
	}

	if err := d.Set("filter", flattenCodePipelineWebhookFilters(webhook.Definition.Filters)); err != nil {
		return fmt.Errorf("error setting filter: %s", err)
	}

	tags := KeyValueTags(webhook.Tags).IgnoreAWS().IgnoreConfig(ignoreTagsConfig)

	//lintignore:AWSR002
	if err := d.Set("tags", tags.RemoveDefaultConfig(defaultTagsConfig).Map()); err != nil {
		return fmt.Errorf("error setting tags: %w", err)
	}

	if err := d.Set("tags_all", tags.Map()); err != nil {
		return fmt.Errorf("error setting tags_all: %w", err)
	}

	return nil
}

func resourceWebhookUpdate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*conns.AWSClient).CodePipelineConn

	if d.HasChange("tags_all") {
		o, n := d.GetChange("tags_all")

		if err := UpdateTags(conn, d.Id(), o, n); err != nil {
			return fmt.Errorf("error updating CodePipeline Webhook (%s) tags: %s", d.Id(), err)
		}
	}

	return resourceWebhookRead(d, meta)
}

func resourceWebhookDelete(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*conns.AWSClient).CodePipelineConn
	name := d.Get("name").(string)

	input := codepipeline.DeleteWebhookInput{
		Name: &name,
	}
	_, err := conn.DeleteWebhook(&input)

	if err != nil {
		return fmt.Errorf("Could not delete webhook: %s", err)
	}

	return nil
}
