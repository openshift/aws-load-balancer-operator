package update

import (
	"context"
	"fmt"
	"reflect"

	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/aws-load-balancer-operator/pkg/utils"
)

// UpdateCredentialsRequest updates the given CredentialsRequest to the desired state if needed.
// Returns true if the CredentialsRequest was updated, false otherwise.
func UpdateCredentialsRequest(ctx context.Context, client client.Client, current *cco.CredentialsRequest, desired *cco.CredentialsRequest) (bool, error) {
	if changed, err := IsCredentialsRequestChanged(current, desired); err != nil {
		return false, err
	} else if !changed {
		return false, nil
	}

	updated := current.DeepCopy()
	updated.Spec = desired.Spec
	if err := client.Update(ctx, updated); err != nil {
		return false, err
	}
	return true, nil
}

// IsCredentialsRequestChanged compares the current and desired states of a CredentialsRequest.
// Returns true of current start is different from the desired, false otherwise.
func IsCredentialsRequestChanged(current, desired *cco.CredentialsRequest) (bool, error) {
	if current.Name != desired.Name {
		return false, fmt.Errorf("current and desired name mismatch")
	}

	if current.Namespace != desired.Namespace {
		return false, fmt.Errorf("current and desired namespace mismatch")
	}

	if !reflect.DeepEqual(current.Spec.SecretRef, desired.Spec.SecretRef) {
		return true, nil
	}

	if !utils.EqualStrings(current.Spec.ServiceAccountNames, desired.Spec.ServiceAccountNames) {
		return true, nil
	}

	if current.Spec.CloudTokenPath != desired.Spec.CloudTokenPath {
		return true, nil
	}

	currentAwsSpec := cco.AWSProviderSpec{}
	err := cco.Codec.DecodeProviderSpec(current.Spec.ProviderSpec, &currentAwsSpec)
	if err != nil {
		return false, err
	}

	desiredAwsSpec := cco.AWSProviderSpec{}
	err = cco.Codec.DecodeProviderSpec(desired.Spec.ProviderSpec, &desiredAwsSpec)
	if err != nil {
		return false, err
	}

	if !(reflect.DeepEqual(currentAwsSpec, desiredAwsSpec)) {
		return true, nil
	}

	return false, nil
}
