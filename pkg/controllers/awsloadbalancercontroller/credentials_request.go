package awsloadbalancercontroller

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
)

const (
	credentialRequestName      = "aws-load-balancer-credentials-request"
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

func (r *AWSLoadBalancerControllerReconciler) ensureCredentialsRequest(ctx context.Context, namespace string, controller *albo.AWSLoadBalancerController) error {
	credReq := createCredentialsRequestName(credentialRequestName)

	reqLogger := log.FromContext(ctx).WithValues("credentialsrequest", credReq)
	reqLogger.Info("ensuring credentials secret for aws-load-balancer instance")

	exists, current, err := r.currentCredentialsRequest(ctx, credReq)
	if err != nil {
		return fmt.Errorf("failed to find existing credential request: %w", err)
	}

	// The secret created will be in the operator namespace.
	secretRef := createCredentialsSecretRef(namespace)

	desired, err := desiredCredentialsRequest(ctx, credReq, secretRef)
	if err != nil {
		return fmt.Errorf("failed to build desired credential request: %w", err)
	}

	err = controllerutil.SetOwnerReference(controller, desired, r.Scheme)
	if err != nil {
		return fmt.Errorf("failed to set owner reference on desired credentialrequest: %w", err)
	}

	if !exists {
		if err := r.createCredentialsRequest(ctx, desired); err != nil {
			return fmt.Errorf("failed to create aws-load-balancer credentials request %s: %w", desired.Name, err)
		}
		_, _, err = r.currentCredentialsRequest(ctx, credReq)
		return err
	}

	if updated, err := r.updateCredentialsRequest(ctx, current, desired); err != nil {
		return fmt.Errorf("failed to update credential request: %w", err)
	} else if updated {
		_, _, err = r.currentCredentialsRequest(ctx, credReq)
		return err
	}

	return nil
}

func (r *AWSLoadBalancerControllerReconciler) createCredentialsRequest(ctx context.Context, desired *cco.CredentialsRequest) error {
	if err := r.Client.Create(ctx, desired); err != nil {
		return err
	}
	return nil
}

// updateCredentialsRequest updates the credentialrequest if needed and returns a flag to denote if the update was done.
func (r *AWSLoadBalancerControllerReconciler) updateCredentialsRequest(ctx context.Context, current *cco.CredentialsRequest, desired *cco.CredentialsRequest) (bool, error) {
	var updated *cco.CredentialsRequest
	changed, err := isCredentialsRequestChanged(current, desired)
	if err != nil {
		return false, err
	}
	if !changed {
		return false, nil
	}
	updated = current.DeepCopy()
	updated.Name = desired.Name
	updated.Namespace = desired.Namespace
	updated.Spec = desired.Spec
	if err := r.Client.Update(ctx, updated); err != nil {
		return false, err
	}
	return true, nil
}

func desiredCredentialsRequest(ctx context.Context, name types.NamespacedName, secretRef corev1.ObjectReference) (*cco.CredentialsRequest, error) {
	credentialsRequest := &cco.CredentialsRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
		},
		Spec: cco.CredentialsRequestSpec{
			ServiceAccountNames: []string{controllerServiceAccountName},
			SecretRef:           secretRef,
		},
	}

	codec, err := cco.NewCodec()
	if err != nil {
		return nil, err
	}

	providerSpec, err := createProviderConfig(codec)
	if err != nil {
		return nil, err
	}
	credentialsRequest.Spec.ProviderSpec = providerSpec
	return credentialsRequest, nil
}

func createProviderConfig(codec *cco.ProviderCodec) (*runtime.RawExtension, error) {
	return codec.EncodeProviderSpec(&cco.AWSProviderSpec{
		StatementEntries: GetIAMPolicy().Statement,
	})
}

// createCredentialsRequestName will always return a fixed namespaced resource, so as to
// make it future proof. The credentials operator will have limitations in the future, wrt watched namespaces.
func createCredentialsRequestName(name string) types.NamespacedName {
	return types.NamespacedName{
		Name:      name,
		Namespace: credentialRequestNamespace,
	}
}

func createCredentialsSecretRef(namespace string) corev1.ObjectReference {
	return corev1.ObjectReference{
		Name:      controllerSecretName,
		Namespace: namespace,
	}
}

func isCredentialsRequestChanged(current, desired *cco.CredentialsRequest) (bool, error) {
	if current.Name != desired.Name {
		return true, nil
	}

	if current.Namespace != desired.Namespace {
		return true, nil
	}

	codec, err := cco.NewCodec()
	if err != nil {
		return false, err
	}

	currentAwsSpec := cco.AWSProviderSpec{}
	err = codec.DecodeProviderSpec(current.Spec.ProviderSpec, &currentAwsSpec)
	if err != nil {
		return false, err
	}

	desiredAwsSpec := cco.AWSProviderSpec{}
	err = codec.DecodeProviderSpec(desired.Spec.ProviderSpec, &desiredAwsSpec)
	if err != nil {
		return false, err
	}

	if !(reflect.DeepEqual(currentAwsSpec, desiredAwsSpec)) {
		return true, nil
	}

	return false, nil
}
