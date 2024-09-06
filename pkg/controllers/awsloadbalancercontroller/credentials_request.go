package awsloadbalancercontroller

import (
	"context"
	"fmt"
	"path"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1"
	"github.com/openshift/aws-load-balancer-operator/pkg/utils/resource/update"
)

const (
	credentialRequestNamespace = "openshift-cloud-credential-operator"
)

// currentCredentialsRequest returns true if credentials request exists.
func (r *AWSLoadBalancerControllerReconciler) currentCredentialsRequest(ctx context.Context, name types.NamespacedName) (bool, *cco.CredentialsRequest, error) {
	cr := &cco.CredentialsRequest{}
	if err := r.Client.Get(ctx, name, cr); err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}
	return true, cr, nil
}

// ensureCredentialsRequest ensures the CredentialsRequest resource and return the secret where the credentials will be written
func (r *AWSLoadBalancerControllerReconciler) ensureCredentialsRequest(ctx context.Context, namespace string, controller *albo.AWSLoadBalancerController) (*cco.CredentialsRequest, error) {
	name := fmt.Sprintf("%s-%s", controllerResourcePrefix, controller.Name)
	credReq := createCredentialsRequestName(name)

	reqLogger := log.FromContext(ctx).WithValues("credentialsrequest", credReq)
	reqLogger.Info("ensuring credentials secret for aws-load-balancer-controller instance")

	exists, current, err := r.currentCredentialsRequest(ctx, credReq)
	if err != nil {
		return nil, fmt.Errorf("failed to get existing credentials request %q: %w", credReq.Name, err)
	}

	credentialRequestSecretName := fmt.Sprintf("%s-credentialsrequest-%s", controllerResourcePrefix, controller.Name)

	// The secret created will be in the operator namespace.
	secretRef := createCredentialsSecretRef(credentialRequestSecretName, namespace)

	desired, err := desiredCredentialsRequest(credReq, secretRef, name, controller.Spec.CredentialsRequestConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build desired credentials request: %w", err)
	}

	err = controllerutil.SetControllerReference(controller, desired, r.Scheme)
	if err != nil {
		return nil, fmt.Errorf("failed to set owner reference on desired credentials request: %w", err)
	}

	if !exists {
		if err := r.createCredentialsRequest(ctx, desired); err != nil {
			return nil, fmt.Errorf("failed to create credentials request %s: %w", desired.Name, err)
		}
		found, created, err := r.currentCredentialsRequest(ctx, credReq)
		if err != nil {
			return nil, fmt.Errorf("failed to get new credentials request %q: %w", credReq.Name, err)
		}
		if !found {
			return nil, fmt.Errorf("failed to get new credentials request %q: not found", credReq.Name)
		}
		return created, nil
	}

	gotUpdated, err := update.UpdateCredentialsRequest(ctx, r.Client, current, desired)
	if err != nil {
		return nil, fmt.Errorf("failed to update credentials request %q: %w", credReq.Name, err)
	}
	if gotUpdated {
		found, updated, err := r.currentCredentialsRequest(ctx, credReq)
		if err != nil {
			return nil, fmt.Errorf("failed to get updated credentials request %q: %w", credReq.Name, err)
		}
		if !found {
			return nil, fmt.Errorf("failed to get updated credentials request %q: not found", credReq.Name)
		}
		return updated, nil
	}
	return current, nil
}

func (r *AWSLoadBalancerControllerReconciler) credentialsSecretProvisioned(ctx context.Context, name types.NamespacedName) (bool, error) {
	var secret corev1.Secret

	err := r.Client.Get(context.TODO(), name, &secret)
	if err != nil && errors.IsNotFound(err) {
		log.FromContext(ctx).Info("failed to get secret associated with credentials request", "secret", name)
		return false, nil
	} else if err != nil {
		return false, err
	}

	return true, nil
}

func (r *AWSLoadBalancerControllerReconciler) createCredentialsRequest(ctx context.Context, desired *cco.CredentialsRequest) error {
	if err := r.Client.Create(ctx, desired); err != nil {
		return err
	}
	return nil
}

func desiredCredentialsRequest(name types.NamespacedName, secretRef corev1.ObjectReference, saName string, config *albo.AWSLoadBalancerCredentialsRequestConfig) (*cco.CredentialsRequest, error) {
	credentialsRequest := &cco.CredentialsRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
		},
		Spec: cco.CredentialsRequestSpec{
			ServiceAccountNames: []string{saName},
			SecretRef:           secretRef,
		},
	}

	if config != nil && config.STSIAMRoleARN != "" {
		credentialsRequest.Spec.CloudTokenPath = path.Join(boundSATokenDir, "token")
	}

	providerSpec, err := createProviderConfig(cco.Codec, config)
	if err != nil {
		return nil, err
	}
	credentialsRequest.Spec.ProviderSpec = providerSpec
	return credentialsRequest, nil
}

func createProviderConfig(codec *cco.ProviderCodec, config *albo.AWSLoadBalancerCredentialsRequestConfig) (*runtime.RawExtension, error) {
	providerSpec := &cco.AWSProviderSpec{
		// NOTE:
		// The minified version of the policy has to be added to the CredentialsRequest.
		// The full policy exceeds the user inline policy size limit.
		//
		// On STS clusters: a drift between the statements from the STS IAM role (set below)
		// and the CredentialsRequest can occur in case a roleARN is added to AWSLoadBalancerController CR.
		// This doesn't impact the permissions granted to the service account though
		// because they are taken from the role.
		StatementEntries: GetIAMPolicyMinify().Statement,
	}
	if config != nil && config.STSIAMRoleARN != "" {
		providerSpec.STSIAMRoleARN = config.STSIAMRoleARN
	}
	return codec.EncodeProviderSpec(providerSpec)
}

// createCredentialsRequestName will always return a fixed namespaced resource, so as to
// make it future-proof. The credentials operator will have limitations in the future, wrt watched namespaces.
func createCredentialsRequestName(name string) types.NamespacedName {
	return types.NamespacedName{
		Name:      name,
		Namespace: credentialRequestNamespace,
	}
}

func createCredentialsSecretRef(name string, namespace string) corev1.ObjectReference {
	return corev1.ObjectReference{
		Name:      name,
		Namespace: namespace,
	}
}
