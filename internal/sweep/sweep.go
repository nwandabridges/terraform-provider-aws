//go:build sweep
// +build sweep

package sweep

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/hashicorp/aws-sdk-go-base/tfawserr"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-aws/internal/conns"
	"github.com/hashicorp/terraform-provider-aws/internal/tfresource"
)

const (
	SweepThrottlingRetryTimeout = 10 * time.Minute

	ResourcePrefix = "tf-acc-test"
)

const defaultSweeperAssumeRoleDurationSeconds = 3600

// SweeperClients is a shared cache of regional conns.AWSClient
// This prevents client re-initialization for every resource with no benefit.
var SweeperClients map[string]interface{}

// SharedRegionalSweepClient returns a common conns.AWSClient setup needed for the sweeper
// functions for a given region
func SharedRegionalSweepClient(region string) (interface{}, error) {
	if client, ok := SweeperClients[region]; ok {
		return client, nil
	}

	_, _, err := conns.RequireOneOfEnvVar([]string{conns.EnvVarProfile, conns.EnvVarAccessKeyId, conns.EnvVarContainerCredentialsFullUri}, "credentials for running sweepers")
	if err != nil {
		return nil, err
	}

	if os.Getenv(conns.EnvVarAccessKeyId) != "" {
		_, err := conns.RequireEnvVar(conns.EnvVarSecretAccessKey, "static credentials value when using "+conns.EnvVarAccessKeyId)
		if err != nil {
			return nil, err
		}
	}

	conf := &conns.Config{
		MaxRetries: 5,
		Region:     region,
	}

	if role := os.Getenv(conns.EnvVarAssumeRoleARN); role != "" {
		conf.AssumeRoleARN = role

		conf.AssumeRoleDurationSeconds = defaultSweeperAssumeRoleDurationSeconds
		if v := os.Getenv(conns.EnvVarAssumeRoleDuration); v != "" {
			d, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("environment variable %s: %w", conns.EnvVarAssumeRoleDuration, err)
			}
			conf.AssumeRoleDurationSeconds = d
		}

		if v := os.Getenv(conns.EnvVarAssumeRoleExternalID); v != "" {
			conf.AssumeRoleExternalID = v
		}

		if v := os.Getenv(conns.EnvVarAssumeRoleSessionName); v != "" {
			conf.AssumeRoleSessionName = v
		}
	}

	// configures a default client for the region, using the above env vars
	client, err := conf.Client()
	if err != nil {
		return nil, fmt.Errorf("error getting AWS client: %w", err)
	}

	SweeperClients[region] = client

	return client, nil
}

type SweepResource struct {
	d        *schema.ResourceData
	meta     interface{}
	resource *schema.Resource
}

func NewSweepResource(resource *schema.Resource, d *schema.ResourceData, meta interface{}) *SweepResource {
	return &SweepResource{
		d:        d,
		meta:     meta,
		resource: resource,
	}
}

func SweepOrchestrator(sweepResources []*SweepResource) error {
	return SweepOrchestratorContext(context.Background(), sweepResources, 0*time.Millisecond, 0*time.Millisecond, 0*time.Millisecond, 0*time.Millisecond, SweepThrottlingRetryTimeout)
}

func SweepOrchestratorContext(ctx context.Context, sweepResources []*SweepResource, delay time.Duration, delayRand time.Duration, minTimeout time.Duration, pollInterval time.Duration, timeout time.Duration) error {
	var g multierror.Group

	for _, sweepResource := range sweepResources {
		sweepResource := sweepResource

		g.Go(func() error {
			err := tfresource.RetryConfigContext(ctx, delay, delayRand, minTimeout, pollInterval, timeout, func() *resource.RetryError {
				err := DeleteResource(sweepResource.resource, sweepResource.d, sweepResource.meta)

				if err != nil {
					if strings.Contains(err.Error(), "Throttling") {
						log.Printf("[INFO] While sweeping resource (%s), encountered throttling error (%s). Retrying...", sweepResource.d.Id(), err)
						return resource.RetryableError(err)
					}

					return resource.NonRetryableError(err)
				}

				return nil
			})

			if tfresource.TimedOut(err) {
				err = DeleteResource(sweepResource.resource, sweepResource.d, sweepResource.meta)
			}

			return err
		})
	}

	return g.Wait().ErrorOrNil()
}

// Check sweeper API call error for reasons to skip sweeping
// These include missing API endpoints and unsupported API calls
func SkipSweepError(err error) bool {
	// Ignore missing API endpoints
	if tfawserr.ErrMessageContains(err, "RequestError", "send request failed") {
		return true
	}
	// Ignore unsupported API calls
	if tfawserr.ErrMessageContains(err, "UnsupportedOperation", "") {
		return true
	}
	// Ignore more unsupported API calls
	// InvalidParameterValue: Use of cache security groups is not permitted in this API version for your account.
	if tfawserr.ErrMessageContains(err, "InvalidParameterValue", "not permitted in this API version for your account") {
		return true
	}
	// InvalidParameterValue: Access Denied to API Version: APIGlobalDatabases
	if tfawserr.ErrMessageContains(err, "InvalidParameterValue", "Access Denied to API Version") {
		return true
	}
	// GovCloud has endpoints that respond with (no message provided):
	// AccessDeniedException:
	// Since acceptance test sweepers are best effort and this response is very common,
	// we allow bypassing this error globally instead of individual test sweeper fixes.
	if tfawserr.ErrMessageContains(err, "AccessDeniedException", "") {
		return true
	}
	// Example: BadRequestException: vpc link not supported for region us-gov-west-1
	if tfawserr.ErrMessageContains(err, "BadRequestException", "not supported") {
		return true
	}
	// Example: InvalidAction: The action DescribeTransitGatewayAttachments is not valid for this web service
	if tfawserr.ErrMessageContains(err, "InvalidAction", "is not valid") {
		return true
	}
	// For example from GovCloud SES.SetActiveReceiptRuleSet.
	if tfawserr.ErrMessageContains(err, "InvalidAction", "Unavailable Operation") {
		return true
	}
	// For example from us-west-2 Route53 key signing key
	if tfawserr.ErrMessageContains(err, "InvalidKeySigningKeyStatus", "cannot be deleted because") {
		return true
	}
	// For example from us-west-2 Route53 zone
	if tfawserr.ErrMessageContains(err, "KeySigningKeyInParentDSRecord", "Due to DNS lookup failure") {
		return true
	}
	// For example from us-west-2 ECR public repository
	if tfawserr.ErrMessageContains(err, "UnsupportedCommandException", "command is only supported in") {
		return true
	}
	return false
}

func DeleteResource(resource *schema.Resource, d *schema.ResourceData, meta interface{}) error {
	if resource.DeleteContext != nil || resource.DeleteWithoutTimeout != nil {
		var diags diag.Diagnostics

		if resource.DeleteContext != nil {
			diags = resource.DeleteContext(context.Background(), d, meta)
		} else {
			diags = resource.DeleteWithoutTimeout(context.Background(), d, meta)
		}

		for i := range diags {
			if diags[i].Severity == diag.Error {
				return fmt.Errorf("error deleting resource: %s", diags[i].Summary)
			}
		}

		return nil
	}

	return resource.Delete(d, meta)
}

func Partition(region string) string {
	if partition, ok := endpoints.PartitionForRegion(endpoints.DefaultPartitions(), region); ok {
		return partition.ID()
	}
	return "aws"
}

func PartitionDNSSuffix(region string) string {
	if partition, ok := endpoints.PartitionForRegion(endpoints.DefaultPartitions(), region); ok {
		return partition.DNSSuffix()
	}
	return "amazonaws.com"
}
