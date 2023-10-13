package operator

// NOTE: this file is influenced by the new approach of how to do STS described in
// https://docs.google.com/document/d/1iFNpyycby_rOY1wUew-yl3uPWlE00krTgr9XHDZOTNo.

import (
	"context"
	"fmt"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/aws-load-balancer-operator/pkg/utils/resource/update"
)

const (
	roleARNEnvVar                 = "ROLEARN"
	operatorCredentialsSecretName = "aws-load-balancer-operator"
	operatorServiceAccountName    = "aws-load-balancer-operator-controller-manager"
	credentialsFilePattern        = "albo-aws-shared-credentials-"
	webIdentityTokenPath          = "/var/run/secrets/openshift/serviceaccount/token"
	credentialsKey                = "credentials"
	waitForSecretTimeout          = 5 * time.Minute
	waitForSecretPollInterval     = 5 * time.Second
)

var (
	operatorCredentialsRequestNsName = types.NamespacedName{
		Namespace: cco.CloudCredOperatorNamespace,
		Name:      "aws-load-balancer-operator",
	}
)

// ProvisionCredentials provisions cloud credentials secret in the given namespace
// with IAM policies required by the operator. The credentials data are put
// into a file which can be used to set up AWS SDK client.
func ProvisionCredentials(ctx context.Context, client client.Client, secretNamespace string) (string, error) {
	roleARN := os.Getenv(roleARNEnvVar)
	if roleARN != "" && !arn.IsARN(roleARN) {
		return "", fmt.Errorf("provided role arn is invalid: %q", roleARN)
	}

	secretNsName := types.NamespacedName{
		Namespace: secretNamespace,
		Name:      operatorCredentialsSecretName,
	}

	// create/update CredentialsRequest resource
	desiredCredReq := buildCredentialsRequest(secretNsName, roleARN)
	currCredReq := &cco.CredentialsRequest{}
	if err := client.Get(ctx, operatorCredentialsRequestNsName, currCredReq); err != nil {
		if errors.IsNotFound(err) {
			if err := client.Create(ctx, desiredCredReq); err != nil {
				return "", err
			}
		} else {
			return "", err
		}
	} else if _, err := update.UpdateCredentialsRequest(ctx, client, currCredReq, desiredCredReq); err != nil {
		return "", err
	}

	// wait till the credentials secret is provisioned by CCO
	secret, err := waitForSecret(ctx, client, secretNsName, waitForSecretTimeout, waitForSecretPollInterval)
	if err != nil {
		return "", err
	}

	// create credentials file with data taken from the provisioned secret
	credFileName, err := credentialsFileFromSecret(secret, credentialsFilePattern)
	if err != nil {
		return "", err
	}

	return credFileName, nil
}

// buildCredentialsRequest returns CredentialsRequest object with IAM policies
// required by this operator. STS IAM role is set if the given role ARN is not empty.
func buildCredentialsRequest(secretNsName types.NamespacedName, roleARN string) *cco.CredentialsRequest {
	providerSpecIn := cco.AWSProviderSpec{
		StatementEntries: GetIAMPolicy().Statement,
	}
	if roleARN != "" {
		providerSpecIn.STSIAMRoleARN = roleARN
	}

	providerSpec, _ := cco.Codec.EncodeProviderSpec(providerSpecIn.DeepCopyObject())

	credReq := &cco.CredentialsRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorCredentialsRequestNsName.Name,
			Namespace: operatorCredentialsRequestNsName.Namespace,
		},
		Spec: cco.CredentialsRequestSpec{
			ProviderSpec: providerSpec,
			SecretRef: corev1.ObjectReference{
				Name:      secretNsName.Name,
				Namespace: secretNsName.Namespace,
			},
			ServiceAccountNames: []string{operatorServiceAccountName},
		},
	}

	if roleARN != "" {
		credReq.Spec.CloudTokenPath = webIdentityTokenPath
	}

	return credReq
}

// waitForSecret waits until the secret with the given name appears in the given namespace.
// It returns the secret object and an error if the timeout is exceeded.
func waitForSecret(ctx context.Context, client client.Client, nsName types.NamespacedName, timeout, pollInterval time.Duration) (*corev1.Secret, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timer.C:
			return nil, fmt.Errorf("timed out waiting for operator credentials secret %v", nsName)
		case <-ticker.C:
			secret := &corev1.Secret{}
			err := client.Get(ctx, nsName, secret)
			if err != nil {
				if errors.IsNotFound(err) {
					continue
				} else {
					return nil, err
				}
			} else {
				return secret, nil
			}
		}
	}
}

// credentialsFileFromSecret creates a file on a temporary file system with data from the given secret.
// It returns the full path of the created file and an error
func credentialsFileFromSecret(secret *corev1.Secret, pattern string) (string, error) {
	if len(secret.Data[credentialsKey]) == 0 {
		return "", fmt.Errorf("failed to find credentials in secret")
	}

	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", fmt.Errorf("failed to create credentials file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(secret.Data[credentialsKey]); err != nil {
		return "", fmt.Errorf("failed to write credentials to %q: %w", f.Name(), err)
	}

	return f.Name(), nil
}
