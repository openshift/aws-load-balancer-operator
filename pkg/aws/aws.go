package aws

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"

	configv1 "github.com/openshift/api/config/v1"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	clusterInfrastructureName = "cluster"
)

type EC2Client interface {
	DescribeSubnets(context.Context, *ec2.DescribeSubnetsInput, ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error)
	CreateTags(context.Context, *ec2.CreateTagsInput, ...func(*ec2.Options)) (*ec2.CreateTagsOutput, error)
	DeleteTags(context.Context, *ec2.DeleteTagsInput, ...func(*ec2.Options)) (*ec2.DeleteTagsOutput, error)
}

func GetEC2Client(ctx context.Context, client client.Client) (EC2Client, error) {
	var infra configv1.Infrastructure
	infraKey := types.NamespacedName{
		Name: clusterInfrastructureName,
	}
	err := client.Get(ctx, infraKey, &infra)
	if err != nil {
		return nil, fmt.Errorf("failed to get Infrastructure %q: %w", clusterInfrastructureName, err)
	}
	if infra.Status.PlatformStatus == nil || infra.Status.PlatformStatus.AWS == nil || infra.Status.PlatformStatus.AWS.Region == "" {
		return nil, fmt.Errorf("could not get AWS region from Infrastructure %q status", clusterInfrastructureName)
	}
	region := infra.Status.PlatformStatus.AWS.Region

	awsConfig, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("unable to load AWS config: %w", err)
	}
	return ec2.NewFromConfig(awsConfig), nil
}
