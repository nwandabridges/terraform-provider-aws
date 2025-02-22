package cognitoidp

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/service/cognitoidentityprovider"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-aws/internal/conns"
)

func DataSourceUserPools() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceUserPoolsRead,
		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"ids": {
				Type:     schema.TypeSet,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"arns": {
				Type:     schema.TypeSet,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
		},
	}
}

func dataSourceUserPoolsRead(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*conns.AWSClient).CognitoIDPConn
	name := d.Get("name").(string)
	var ids []string
	var arns []string

	pools, err := getAllCognitoUserPools(conn)
	if err != nil {
		return fmt.Errorf("Error listing cognito user pools: %w", err)
	}
	for _, pool := range pools {
		if name == aws.StringValue(pool.Name) {
			id := aws.StringValue(pool.Id)
			arn := arn.ARN{
				Partition: meta.(*conns.AWSClient).Partition,
				Service:   "cognito-idp",
				Region:    meta.(*conns.AWSClient).Region,
				AccountID: meta.(*conns.AWSClient).AccountID,
				Resource:  fmt.Sprintf("userpool/%s", id),
			}.String()

			ids = append(ids, id)
			arns = append(arns, arn)
		}
	}

	if len(ids) == 0 {
		return fmt.Errorf("No cognito user pool found with name: %s", name)
	}

	d.SetId(name)
	d.Set("ids", ids)
	d.Set("arns", arns)

	return nil
}

func getAllCognitoUserPools(conn *cognitoidentityprovider.CognitoIdentityProvider) ([]*cognitoidentityprovider.UserPoolDescriptionType, error) {
	var pools []*cognitoidentityprovider.UserPoolDescriptionType
	var nextToken string

	for {
		input := &cognitoidentityprovider.ListUserPoolsInput{
			// MaxResults Valid Range: Minimum value of 1. Maximum value of 60
			MaxResults: aws.Int64(int64(60)),
		}
		if nextToken != "" {
			input.NextToken = aws.String(nextToken)
		}
		out, err := conn.ListUserPools(input)
		if err != nil {
			return pools, err
		}
		pools = append(pools, out.UserPools...)

		if out.NextToken == nil {
			break
		}
		nextToken = aws.StringValue(out.NextToken)
	}

	return pools, nil
}
