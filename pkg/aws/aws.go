package aws

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"

	configv1 "github.com/openshift/api/config/v1"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	clusterInfrastructureName = "cluster"
	clusterTagKey             = "kubernetes.io/cluster/%s"
	tagKeyFilterName          = "tag-key"
)

// VPCClient can be used to query VPCs
type VPCClient interface {
	DescribeVpcs(ctx context.Context, params *ec2.DescribeVpcsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error)
}

// SubnetClient can be used to query subnets and perform tagging operations
type SubnetClient interface {
	DescribeSubnets(context.Context, *ec2.DescribeSubnetsInput, ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error)
	CreateTags(context.Context, *ec2.CreateTagsInput, ...func(*ec2.Options)) (*ec2.CreateTagsOutput, error)
	DeleteTags(context.Context, *ec2.DeleteTagsInput, ...func(*ec2.Options)) (*ec2.DeleteTagsOutput, error)
}

// EC2Client has a VPCClient and SubnetClient
type EC2Client interface {
	VPCClient
	SubnetClient
}

// GetEC2Client returns and EC2Client along with the VPC id and the cluster name
func GetEC2Client(ctx context.Context, client client.Client) (EC2Client, string, string, error) {
	var infra configv1.Infrastructure
	infraKey := types.NamespacedName{
		Name: clusterInfrastructureName,
	}
	err := client.Get(ctx, infraKey, &infra)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to get Infrastructure %q: %w", clusterInfrastructureName, err)
	}
	if infra.Status.PlatformStatus == nil || infra.Status.PlatformStatus.AWS == nil || infra.Status.PlatformStatus.AWS.Region == "" {
		return nil, "", "", fmt.Errorf("could not get AWS region from Infrastructure %q status", clusterInfrastructureName)
	}
	region := infra.Status.PlatformStatus.AWS.Region

	awsConfig, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, "", "", fmt.Errorf("unable to load AWS config: %w", err)
	}
	ec2Client := ec2.NewFromConfig(awsConfig)
	infraTagKey := fmt.Sprintf(clusterTagKey, infra.Status.InfrastructureName)
	vpcs, err := ec2Client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{
		Filters: []ec2types.Filter{
			{
				Name:   aws.String(tagKeyFilterName),
				Values: []string{infraTagKey},
			},
		},
	})
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to list VPC with tag %q: %w", infraTagKey, err)
	}
	if len(vpcs.Vpcs) == 0 {
		return nil, "", "", fmt.Errorf("no VPC with tag %q found", infraTagKey)
	}
	if len(vpcs.Vpcs) > 1 {
		return nil, "", "", fmt.Errorf("multiple VPC with tag %q found", infraTagKey)
	}
	return ec2Client, aws.ToString(vpcs.Vpcs[0].VpcId), infra.Status.InfrastructureName, nil
}
