package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	clusterInfrastructureName = "cluster"
)

type EC2Client interface {
	DescribeSubnets(context.Context, *ec2.DescribeSubnetsInput, ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error)
	CreateTags(context.Context, *ec2.CreateTagsInput, ...func(*ec2.Options)) (*ec2.CreateTagsOutput, error)
	DeleteTags(context.Context, *ec2.DeleteTagsInput, ...func(*ec2.Options)) (*ec2.DeleteTagsOutput, error)
}

func GetEC2Client(client *configv1client.ConfigV1Client) (EC2Client, error) {
	infra, err := client.Infrastructures().Get(context.Background(), clusterInfrastructureName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get Infrastructure '%s': %v", clusterInfrastructureName, err)
	}
	if infra.Status.PlatformStatus == nil || infra.Status.PlatformStatus.AWS == nil || infra.Status.PlatformStatus.AWS.Region == "" {
		return nil, fmt.Errorf("could not get AWS region from Instructure '%s' status", clusterInfrastructureName)
	}
	region := infra.Status.PlatformStatus.AWS.Region

	awsConfig, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("unable to load AWS config: %w", err)
	}
	return ec2.NewFromConfig(awsConfig), nil
}
