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

	desired, err := desiredCredentialsRequest(credReq, secretRef, name)
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
	} else if err := r.updateCredentialsRequest(ctx, current, desired); err != nil {
		return nil, fmt.Errorf("failed to update credentials request %q: %w", credReq.Name, err)
	}

	_, current, err = r.currentCredentialsRequest(ctx, credReq)
	if err != nil {
		return nil, fmt.Errorf("failed to get existing credentials request %q: %w", credReq.Name, err)
	}
	return current, nil
}

func (r *AWSLoadBalancerControllerReconciler) createCredentialsRequest(ctx context.Context, desired *cco.CredentialsRequest) error {
	if err := r.Client.Create(ctx, desired); err != nil {
		return err
	}
	return nil
}

// updateCredentialsRequest updates the CredentialsRequest if needed
func (r *AWSLoadBalancerControllerReconciler) updateCredentialsRequest(ctx context.Context, current *cco.CredentialsRequest, desired *cco.CredentialsRequest) error {
	var updated *cco.CredentialsRequest
	changed, err := isCredentialsRequestChanged(current, desired)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	updated = current.DeepCopy()
	updated.Name = desired.Name
	updated.Namespace = desired.Namespace
	updated.Spec = desired.Spec
	if err := r.Client.Update(ctx, updated); err != nil {
		return err
	}
	return nil
}

func desiredCredentialsRequest(name types.NamespacedName, secretRef corev1.ObjectReference, saName string) (*cco.CredentialsRequest, error) {
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
