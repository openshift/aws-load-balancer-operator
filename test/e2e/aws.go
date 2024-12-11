//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	rgtTpye "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// arnToTagsMap maps ARNs to formatted tags.
type arnToTagsMap map[string]map[string]string

// awsConfigWithCredentials returns the default AWS config with the given region and static credentials.
func awsConfigWithCredentials(ctx context.Context, kubeClient client.Client, awsRegion string, secretName types.NamespacedName) (aws.Config, error) {
	secret := &corev1.Secret{}
	err := wait.PollUntilContextCancel(ctx, 5*time.Second, true, func(ctx context.Context) (done bool, err error) {
		err = kubeClient.Get(ctx, secretName, secret)
		if err == nil {
			return true, nil
		} else if errors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get credentials secret %s: %w", secretName.Name, err)
	})
	if err != nil {
		return aws.Config{}, fmt.Errorf("failed to get credentials secret %s: %w", secretName.Name, err)
	}

	keyID := string(secret.Data["aws_access_key_id"])
	secretKey := string(secret.Data["aws_secret_access_key"])

	return config.LoadDefaultConfig(ctx,
		config.WithRegion(awsRegion),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(keyID, secretKey, "")))
}

// getLoadBalancerARNsByTags retrieves the ARNs and Tags of Elastic Load Balancers that match the specified tags.
// It uses the Resource Groups Tagging API to filter load balancers based on tag criteria.
func getLoadBalancerARNsByTags(ctx context.Context, rgtClient *resourcegroupstaggingapi.Client, tags map[string]string) (arnToTagsMap, error) {
	var tagFilters []rgtTpye.TagFilter
	for key, value := range tags {
		tagFilters = append(tagFilters, rgtTpye.TagFilter{
			Key:    aws.String(key),
			Values: []string{value},
		})
	}

	input := &resourcegroupstaggingapi.GetResourcesInput{
		ResourceTypeFilters: []string{"elasticloadbalancing:loadbalancer"},
		TagFilters:          tagFilters,
	}

	arnsAndTags := make(arnToTagsMap)
	paginator := resourcegroupstaggingapi.NewGetResourcesPaginator(rgtClient, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed getting resources page: %w", err)
		}

		for _, resource := range page.ResourceTagMappingList {
			formattedTags := make(map[string]string)
			for _, tag := range resource.Tags {
				formattedTags[*tag.Key] = *tag.Value
			}

			arnsAndTags[*resource.ResourceARN] = formattedTags
		}

	}

	return arnsAndTags, nil
}
