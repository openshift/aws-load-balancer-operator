package e2e

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type awsTestHelper struct {
	keyID     string
	secretKey string
}

func newAWSHelper(kubeClient client.Client, awsRegion string) (aws.Config, error) {
	provider := &awsTestHelper{}
	if err := provider.prepareConfigurations(kubeClient); err != nil {
		return aws.Config{}, err
	}

	return config.LoadDefaultConfig(context.Background(),
		config.WithRegion(awsRegion),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(provider.keyID, provider.secretKey, "")))
}

func rootCredentials(kubeClient client.Client, name string) (map[string][]byte, error) {
	secret := &corev1.Secret{}
	secretName := types.NamespacedName{
		Name:      name,
		Namespace: "kube-system",
	}
	if err := kubeClient.Get(context.TODO(), secretName, secret); err != nil {
		return nil, fmt.Errorf("failed to get credentials secret %s: %w", secretName.Name, err)
	}
	return secret.Data, nil
}

func (a *awsTestHelper) prepareConfigurations(kubeClient client.Client) error {
	data, err := rootCredentials(kubeClient, "aws-creds")
	if err != nil {
		return fmt.Errorf("failed to get AWS credentials: %w", err)
	}
	a.keyID = string(data["aws_access_key_id"])
	a.secretKey = string(data["aws_secret_access_key"])
	return nil
}
