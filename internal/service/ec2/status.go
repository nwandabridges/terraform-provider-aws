package ec2

import (
	"fmt"
	"log"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/hashicorp/aws-sdk-go-base/tfawserr"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	tfec2 "github.com/hashicorp/terraform-provider-aws/aws/internal/service/ec2"
	tfiam "github.com/hashicorp/terraform-provider-aws/aws/internal/service/iam"
	"github.com/hashicorp/terraform-provider-aws/aws/internal/tfresource"
	tfec2 "github.com/hashicorp/terraform-provider-aws/internal/service/ec2"
	tfiam "github.com/hashicorp/terraform-provider-aws/internal/service/iam"
)

const (
	carrierGatewayStateNotFound = "NotFound"
	carrierGatewayStateUnknown  = "Unknown"
)

// StatusCarrierGatewayState fetches the CarrierGateway and its State
func StatusCarrierGatewayState(conn *ec2.EC2, carrierGatewayID string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		carrierGateway, err := tfec2.FindCarrierGatewayByID(conn, carrierGatewayID)
		if tfawserr.ErrCodeEquals(err, tfec2.ErrCodeInvalidCarrierGatewayIDNotFound) {
			return nil, carrierGatewayStateNotFound, nil
		}
		if err != nil {
			return nil, carrierGatewayStateUnknown, err
		}

		if carrierGateway == nil {
			return nil, carrierGatewayStateNotFound, nil
		}

		state := aws.StringValue(carrierGateway.State)

		if state == ec2.CarrierGatewayStateDeleted {
			return nil, carrierGatewayStateNotFound, nil
		}

		return carrierGateway, state, nil
	}
}

// StatusLocalGatewayRouteTableVPCAssociationState fetches the LocalGatewayRouteTableVpcAssociation and its State
func StatusLocalGatewayRouteTableVPCAssociationState(conn *ec2.EC2, localGatewayRouteTableVpcAssociationID string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		input := &ec2.DescribeLocalGatewayRouteTableVpcAssociationsInput{
			LocalGatewayRouteTableVpcAssociationIds: aws.StringSlice([]string{localGatewayRouteTableVpcAssociationID}),
		}

		output, err := conn.DescribeLocalGatewayRouteTableVpcAssociations(input)

		if err != nil {
			return nil, "", err
		}

		var association *ec2.LocalGatewayRouteTableVpcAssociation

		for _, outputAssociation := range output.LocalGatewayRouteTableVpcAssociations {
			if outputAssociation == nil {
				continue
			}

			if aws.StringValue(outputAssociation.LocalGatewayRouteTableVpcAssociationId) == localGatewayRouteTableVpcAssociationID {
				association = outputAssociation
				break
			}
		}

		if association == nil {
			return association, ec2.RouteTableAssociationStateCodeDisassociated, nil
		}

		return association, aws.StringValue(association.State), nil
	}
}

const (
	ClientVPNEndpointStatusNotFound = "NotFound"

	ClientVPNEndpointStatusUnknown = "Unknown"
)

// StatusClientVPNEndpoint fetches the Client VPN endpoint and its Status
func StatusClientVPNEndpoint(conn *ec2.EC2, endpointID string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		result, err := conn.DescribeClientVpnEndpoints(&ec2.DescribeClientVpnEndpointsInput{
			ClientVpnEndpointIds: aws.StringSlice([]string{endpointID}),
		})
		if tfawserr.ErrCodeEquals(err, tfec2.ErrCodeClientVPNEndpointIdNotFound) {
			return nil, ClientVPNEndpointStatusNotFound, nil
		}
		if err != nil {
			return nil, ClientVPNEndpointStatusUnknown, err
		}

		if result == nil || len(result.ClientVpnEndpoints) == 0 || result.ClientVpnEndpoints[0] == nil {
			return nil, ClientVPNEndpointStatusNotFound, nil
		}

		endpoint := result.ClientVpnEndpoints[0]
		if endpoint.Status == nil || endpoint.Status.Code == nil {
			return endpoint, ClientVPNEndpointStatusUnknown, nil
		}

		return endpoint, aws.StringValue(endpoint.Status.Code), nil
	}
}

const (
	ClientVPNAuthorizationRuleStatusNotFound = "NotFound"

	ClientVPNAuthorizationRuleStatusUnknown = "Unknown"
)

// StatusClientVPNAuthorizationRule fetches the Client VPN authorization rule and its Status
func StatusClientVPNAuthorizationRule(conn *ec2.EC2, authorizationRuleID string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		result, err := tfec2.FindClientVPNAuthorizationRuleByID(conn, authorizationRuleID)
		if tfawserr.ErrCodeEquals(err, tfec2.ErrCodeClientVPNAuthorizationRuleNotFound) {
			return nil, ClientVPNAuthorizationRuleStatusNotFound, nil
		}
		if err != nil {
			return nil, ClientVPNAuthorizationRuleStatusUnknown, err
		}

		if result == nil || len(result.AuthorizationRules) == 0 || result.AuthorizationRules[0] == nil {
			return nil, ClientVPNAuthorizationRuleStatusNotFound, nil
		}

		if len(result.AuthorizationRules) > 1 {
			return nil, ClientVPNAuthorizationRuleStatusUnknown, fmt.Errorf("internal error: found %d results for Client VPN authorization rule (%s) status, need 1", len(result.AuthorizationRules), authorizationRuleID)
		}

		rule := result.AuthorizationRules[0]
		if rule.Status == nil || rule.Status.Code == nil {
			return rule, ClientVPNAuthorizationRuleStatusUnknown, nil
		}

		return rule, aws.StringValue(rule.Status.Code), nil
	}
}

const (
	ClientVPNNetworkAssociationStatusNotFound = "NotFound"

	ClientVPNNetworkAssociationStatusUnknown = "Unknown"
)

func StatusClientVPNNetworkAssociation(conn *ec2.EC2, cvnaID string, cvepID string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		result, err := conn.DescribeClientVpnTargetNetworks(&ec2.DescribeClientVpnTargetNetworksInput{
			ClientVpnEndpointId: aws.String(cvepID),
			AssociationIds:      []*string{aws.String(cvnaID)},
		})

		if tfawserr.ErrCodeEquals(err, tfec2.ErrCodeClientVPNAssociationIdNotFound) || tfawserr.ErrCodeEquals(err, tfec2.ErrCodeClientVPNEndpointIdNotFound) {
			return nil, ClientVPNNetworkAssociationStatusNotFound, nil
		}
		if err != nil {
			return nil, ClientVPNNetworkAssociationStatusUnknown, err
		}

		if result == nil || len(result.ClientVpnTargetNetworks) == 0 || result.ClientVpnTargetNetworks[0] == nil {
			return nil, ClientVPNNetworkAssociationStatusNotFound, nil
		}

		network := result.ClientVpnTargetNetworks[0]
		if network.Status == nil || network.Status.Code == nil {
			return network, ClientVPNNetworkAssociationStatusUnknown, nil
		}

		return network, aws.StringValue(network.Status.Code), nil
	}
}

const (
	ClientVPNRouteStatusNotFound = "NotFound"

	ClientVPNRouteStatusUnknown = "Unknown"
)

// StatusClientVPNRoute fetches the Client VPN route and its Status
func StatusClientVPNRoute(conn *ec2.EC2, routeID string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		result, err := tfec2.FindClientVPNRouteByID(conn, routeID)
		if tfawserr.ErrCodeEquals(err, tfec2.ErrCodeClientVPNRouteNotFound) {
			return nil, ClientVPNRouteStatusNotFound, nil
		}
		if err != nil {
			return nil, ClientVPNRouteStatusUnknown, err
		}

		if result == nil || len(result.Routes) == 0 || result.Routes[0] == nil {
			return nil, ClientVPNRouteStatusNotFound, nil
		}

		if len(result.Routes) > 1 {
			return nil, ClientVPNRouteStatusUnknown, fmt.Errorf("internal error: found %d results for Client VPN route (%s) status, need 1", len(result.Routes), routeID)
		}

		rule := result.Routes[0]
		if rule.Status == nil || rule.Status.Code == nil {
			return rule, ClientVPNRouteStatusUnknown, nil
		}

		return rule, aws.StringValue(rule.Status.Code), nil
	}
}

// StatusInstanceIAMInstanceProfile fetches the Instance and its IamInstanceProfile
//
// The EC2 API accepts a name and always returns an ARN, so it is converted
// back to the name to prevent unexpected differences.
func StatusInstanceIAMInstanceProfile(conn *ec2.EC2, id string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		instance, err := tfec2.FindInstanceByID(conn, id)

		if tfawserr.ErrCodeEquals(err, tfec2.ErrCodeInvalidInstanceIDNotFound) {
			return nil, "", nil
		}

		if err != nil {
			return nil, "", err
		}

		if instance == nil {
			return nil, "", nil
		}

		if instance.IamInstanceProfile == nil || instance.IamInstanceProfile.Arn == nil {
			return instance, "", nil
		}

		name, err := tfiam.InstanceProfileARNToName(aws.StringValue(instance.IamInstanceProfile.Arn))

		if err != nil {
			return instance, "", err
		}

		return instance, name, nil
	}
}

const (
	RouteStatusReady = "ready"
)

func StatusRoute(conn *ec2.EC2, routeFinder tfec2.RouteFinder, routeTableID, destination string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		output, err := routeFinder(conn, routeTableID, destination)

		if tfresource.NotFound(err) {
			return nil, "", nil
		}

		if err != nil {
			return nil, "", err
		}

		return output, RouteStatusReady, nil
	}
}

const (
	RouteTableStatusReady = "ready"
)

func StatusRouteTable(conn *ec2.EC2, id string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		output, err := tfec2.FindRouteTableByID(conn, id)

		if tfresource.NotFound(err) {
			return nil, "", nil
		}

		if err != nil {
			return nil, "", err
		}

		return output, RouteTableStatusReady, nil
	}
}

func StatusRouteTableAssociationState(conn *ec2.EC2, id string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		output, err := tfec2.FindRouteTableAssociationByID(conn, id)

		if tfresource.NotFound(err) {
			return nil, "", nil
		}

		if err != nil {
			return nil, "", err
		}

		return output.AssociationState, aws.StringValue(output.AssociationState.State), nil
	}
}

const (
	SecurityGroupStatusCreated = "Created"

	SecurityGroupStatusNotFound = "NotFound"

	SecurityGroupStatusUnknown = "Unknown"
)

// StatusSecurityGroup fetches the security group and its status
func StatusSecurityGroup(conn *ec2.EC2, id string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		group, err := tfec2.FindSecurityGroupByID(conn, id)
		if tfresource.NotFound(err) {
			return nil, SecurityGroupStatusNotFound, nil
		}
		if err != nil {
			return nil, SecurityGroupStatusUnknown, err
		}

		return group, SecurityGroupStatusCreated, nil
	}
}

// StatusSubnetMapCustomerOwnedIPOnLaunch fetches the Subnet and its MapCustomerOwnedIpOnLaunch
func StatusSubnetMapCustomerOwnedIPOnLaunch(conn *ec2.EC2, id string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		subnet, err := tfec2.FindSubnetByID(conn, id)

		if tfawserr.ErrCodeEquals(err, tfec2.ErrCodeInvalidSubnetIDNotFound) {
			return nil, "false", nil
		}

		if err != nil {
			return nil, "false", err
		}

		if subnet == nil {
			return nil, "false", nil
		}

		return subnet, strconv.FormatBool(aws.BoolValue(subnet.MapCustomerOwnedIpOnLaunch)), nil
	}
}

// StatusSubnetMapPublicIPOnLaunch fetches the Subnet and its MapPublicIpOnLaunch
func StatusSubnetMapPublicIPOnLaunch(conn *ec2.EC2, id string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		subnet, err := tfec2.FindSubnetByID(conn, id)

		if tfawserr.ErrCodeEquals(err, tfec2.ErrCodeInvalidSubnetIDNotFound) {
			return nil, "false", nil
		}

		if err != nil {
			return nil, "false", err
		}

		if subnet == nil {
			return nil, "false", nil
		}

		return subnet, strconv.FormatBool(aws.BoolValue(subnet.MapPublicIpOnLaunch)), nil
	}
}

func StatusTransitGatewayPrefixListReferenceState(conn *ec2.EC2, transitGatewayRouteTableID string, prefixListID string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		transitGatewayPrefixListReference, err := tfec2.FindTransitGatewayPrefixListReference(conn, transitGatewayRouteTableID, prefixListID)

		if err != nil {
			return nil, "", err
		}

		if transitGatewayPrefixListReference == nil {
			return nil, "", nil
		}

		return transitGatewayPrefixListReference, aws.StringValue(transitGatewayPrefixListReference.State), nil
	}
}

func StatusTransitGatewayRouteTablePropagationState(conn *ec2.EC2, transitGatewayRouteTableID string, transitGatewayAttachmentID string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		transitGatewayRouteTablePropagation, err := tfec2.FindTransitGatewayRouteTablePropagation(conn, transitGatewayRouteTableID, transitGatewayAttachmentID)

		if err != nil {
			return nil, "", err
		}

		if transitGatewayRouteTablePropagation == nil {
			return nil, "", nil
		}

		return transitGatewayRouteTablePropagation, aws.StringValue(transitGatewayRouteTablePropagation.State), nil
	}
}

// StatusVPCAttribute fetches the Vpc and its attribute value
func StatusVPCAttribute(conn *ec2.EC2, id string, attribute string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		attributeValue, err := tfec2.FindVPCAttribute(conn, id, attribute)

		if tfawserr.ErrCodeEquals(err, tfec2.ErrCodeInvalidVPCIDNotFound) {
			return nil, "", nil
		}

		if err != nil {
			return nil, "", err
		}

		if attributeValue == nil {
			return nil, "", nil
		}

		return attributeValue, strconv.FormatBool(aws.BoolValue(attributeValue)), nil
	}
}

const (
	vpcPeeringConnectionStatusNotFound = "NotFound"
	vpcPeeringConnectionStatusUnknown  = "Unknown"
)

// StatusVPCPeeringConnection fetches the VPC peering connection and its status
func StatusVPCPeeringConnection(conn *ec2.EC2, vpcPeeringConnectionID string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		vpcPeeringConnection, err := tfec2.FindVPCPeeringConnectionByID(conn, vpcPeeringConnectionID)
		if tfawserr.ErrCodeEquals(err, tfec2.ErrCodeInvalidVPCPeeringConnectionIDNotFound) {
			return nil, vpcPeeringConnectionStatusNotFound, nil
		}
		if err != nil {
			return nil, vpcPeeringConnectionStatusUnknown, err
		}

		// Sometimes AWS just has consistency issues and doesn't see
		// our peering connection yet. Return an empty state.
		if vpcPeeringConnection == nil || vpcPeeringConnection.Status == nil {
			return nil, vpcPeeringConnectionStatusNotFound, nil
		}

		statusCode := aws.StringValue(vpcPeeringConnection.Status.Code)

		// https://docs.aws.amazon.com/vpc/latest/peering/vpc-peering-basics.html#vpc-peering-lifecycle
		switch statusCode {
		case ec2.VpcPeeringConnectionStateReasonCodeFailed:
			log.Printf("[WARN] VPC Peering Connection (%s): %s: %s", vpcPeeringConnectionID, statusCode, aws.StringValue(vpcPeeringConnection.Status.Message))
			fallthrough
		case ec2.VpcPeeringConnectionStateReasonCodeDeleted, ec2.VpcPeeringConnectionStateReasonCodeExpired, ec2.VpcPeeringConnectionStateReasonCodeRejected:
			return nil, vpcPeeringConnectionStatusNotFound, nil
		}

		return vpcPeeringConnection, statusCode, nil
	}
}

const (
	attachmentStateNotFound = "NotFound"
	attachmentStateUnknown  = "Unknown"
)

// StatusVPNGatewayVPCAttachmentState fetches the attachment between the specified VPN gateway and VPC and its state
func StatusVPNGatewayVPCAttachmentState(conn *ec2.EC2, vpnGatewayID, vpcID string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		vpcAttachment, err := tfec2.FindVPNGatewayVPCAttachment(conn, vpnGatewayID, vpcID)
		if tfawserr.ErrCodeEquals(err, tfec2.InvalidVPNGatewayIDNotFound) {
			return nil, attachmentStateNotFound, nil
		}
		if err != nil {
			return nil, attachmentStateUnknown, err
		}

		if vpcAttachment == nil {
			return nil, attachmentStateNotFound, nil
		}

		return vpcAttachment, aws.StringValue(vpcAttachment.State), nil
	}
}

func StatusHostState(conn *ec2.EC2, id string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		output, err := tfec2.FindHostByID(conn, id)

		if tfresource.NotFound(err) {
			return nil, "", nil
		}

		if err != nil {
			return nil, "", err
		}

		return output, aws.StringValue(output.State), nil
	}
}

func StatusManagedPrefixListState(conn *ec2.EC2, id string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		output, err := tfec2.FindManagedPrefixListByID(conn, id)

		if tfresource.NotFound(err) {
			return nil, "", nil
		}

		if err != nil {
			return nil, "", err
		}

		return output, aws.StringValue(output.State), nil
	}
}

func StatusVPCEndpointState(conn *ec2.EC2, id string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		output, err := tfec2.FindVPCEndpointByID(conn, id)

		if tfresource.NotFound(err) {
			return nil, "", nil
		}

		if err != nil {
			return nil, "", err
		}

		return output, aws.StringValue(output.State), nil
	}
}

const (
	VPCEndpointRouteTableAssociationStatusReady = "ready"
)

func StatusVPCEndpointRouteTableAssociation(conn *ec2.EC2, vpcEndpointID, routeTableID string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		err := tfec2.FindVPCEndpointRouteTableAssociationExists(conn, vpcEndpointID, routeTableID)

		if tfresource.NotFound(err) {
			return nil, "", nil
		}

		if err != nil {
			return nil, "", err
		}

		return "", VPCEndpointRouteTableAssociationStatusReady, nil
	}
}

const (
	snapshotImportNotFound = "NotFound"
)

func StatusEBSSnapshotImport(conn *ec2.EC2, importTaskId string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		params := &ec2.DescribeImportSnapshotTasksInput{
			ImportTaskIds: []*string{aws.String(importTaskId)},
		}

		resp, err := conn.DescribeImportSnapshotTasks(params)
		if err != nil {
			return nil, "", err
		}

		if resp == nil || len(resp.ImportSnapshotTasks) < 1 {
			return nil, snapshotImportNotFound, nil
		}

		if task := resp.ImportSnapshotTasks[0]; task != nil {
			detail := task.SnapshotTaskDetail
			if detail.Status != nil && aws.StringValue(detail.Status) == tfec2.EBSSnapshotImportStateDeleting {
				err = fmt.Errorf("Snapshot import task is deleting")
			}
			return detail, aws.StringValue(detail.Status), err
		} else {
			return nil, snapshotImportNotFound, nil
		}
	}
}
