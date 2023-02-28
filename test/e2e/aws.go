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
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// awsConfigWithCredentials returns the default AWS config with the given region and static credentials.
func awsConfigWithCredentials(kubeClient client.Client, awsRegion string, secretName types.NamespacedName) (aws.Config, error) {
	secret := &corev1.Secret{}
	err := wait.PollImmediateInfinite(5*time.Second, func() (done bool, err error) {
		err = kubeClient.Get(context.TODO(), secretName, secret)
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

	return config.LoadDefaultConfig(context.Background(),
		config.WithRegion(awsRegion),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(keyID, secretKey, "")))
}
