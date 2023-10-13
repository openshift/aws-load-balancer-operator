package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

const (
	clusterTagKey    = "kubernetes.io/cluster/%s"
	tagKeyFilterName = "tag-key"
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

func NewClient(ctx context.Context, awsRegion, sharedCredFileName string) (EC2Client, error) {
	awsConfig, err := config.LoadDefaultConfig(ctx, config.WithRegion(awsRegion), config.WithSharedCredentialsFiles([]string{sharedCredFileName}))
	if err != nil {
		return nil, fmt.Errorf("unable to load AWS config: %w", err)
	}
	return ec2.NewFromConfig(awsConfig), nil
}

// GetVPCId return the VPC ID of the cluster
func GetVPCId(ctx context.Context, ec2Client EC2Client, clusterName string) (string, error) {
	infraTagKey := fmt.Sprintf(clusterTagKey, clusterName)
	vpcs, err := ec2Client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{
		Filters: []ec2types.Filter{
			{
				Name:   aws.String(tagKeyFilterName),
				Values: []string{infraTagKey},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to list VPC with tag %q: %w", infraTagKey, err)
	}
	if len(vpcs.Vpcs) == 0 {
		return "", fmt.Errorf("no VPC with tag %q found", infraTagKey)
	}
	if len(vpcs.Vpcs) > 1 {
		return "", fmt.Errorf("multiple VPCs with tag %q found", infraTagKey)
	}
	return aws.ToString(vpcs.Vpcs[0].VpcId), nil
}
