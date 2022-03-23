package awsloadbalancercontroller

import (
	"context"
	"fmt"
	"sort"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
)

const (
	DeploymentAvailableCondition         = "DeploymentAvailable"
	DeploymentUpgradingCondition         = "DeploymentUpgrading"
	CredentialsRequestUpdatingCondition  = "CredentialsRequestUpgrading"
	CredentialsRequestAvailableCondition = "CredentialsRequestAvailable"
)

func (r *AWSLoadBalancerControllerReconciler) updateControllerStatus(ctx context.Context, controller *albo.AWSLoadBalancerController, deployment *appsv1.Deployment, cr *cco.CredentialsRequest) error {
	status := controller.Status.DeepCopy()
	status.Conditions = mergeConditions(status.Conditions, credentialRequestsConditions(cr, controller.Generation)...)
	status.Conditions = mergeConditions(status.Conditions, deploymentConditions(deployment, controller.Generation)...)

	if haveConditionsChanged(controller.Status.Conditions, status.Conditions) {
		controller.Status.Conditions = status.Conditions
		return r.Status().Update(ctx, controller)
	}
	return nil
}

func credentialRequestsConditions(cr *cco.CredentialsRequest, generation int64) []metav1.Condition {
	var conditions []metav1.Condition
	if cr.Status.Provisioned {
		conditions = append(conditions, metav1.Condition{
			Type:               CredentialsRequestAvailableCondition,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: generation,
			Reason:             "CredentialsRequestProvisioned",
			Message:            fmt.Sprintf("CredentialsRequest %q has been provisioned", cr.Name),
		})
	} else {
		conditions = append(conditions, metav1.Condition{
			Type:               CredentialsRequestAvailableCondition,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: generation,
			Reason:             "CredentialsRequestNotProvisioned",
			Message:            fmt.Sprintf("CredentialsRequest %q has not yet been provisioned", cr.Name),
		})
	}
	if cr.Status.LastSyncGeneration != cr.Generation {
		conditions = append(conditions, metav1.Condition{
			Type:               CredentialsRequestUpdatingCondition,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: generation,
			Reason:             "CredentialsRequestSyncGenerationMismatch",
			Message:            fmt.Sprintf("CredentialsRequest %q is updating", cr.Name),
		})
	} else {
		conditions = append(conditions, metav1.Condition{
			Type:               CredentialsRequestUpdatingCondition,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: generation,
			Reason:             "CredentialsRequestSyncGenerationMatch",
			Message:            fmt.Sprintf("CredentialsRequest %q is up-to-date", cr.Name),
		})
	}
	return conditions
}

func deploymentConditions(deployment *appsv1.Deployment, generation int64) []metav1.Condition {
	var conditions []metav1.Condition

	var replicas int32 = 1
	if deployment.Spec.Replicas != nil {
		replicas = *deployment.Spec.Replicas
	}

	if deployment.Status.AvailableReplicas == replicas {
		conditions = append(conditions, metav1.Condition{
			Type:               DeploymentAvailableCondition,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: generation,
			Reason:             "AllDeploymentReplicasAvailable",
			Message:            fmt.Sprintf("Number of desired and available replicas of deployment %q are equal", deployment.Name),
		})
	} else {
		conditions = append(conditions, metav1.Condition{
			Type:               DeploymentAvailableCondition,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: generation,
			Reason:             "AllDeploymentReplicasNotAvailable",
			Message:            fmt.Sprintf("Number of desired and available replicas of deployment %q are not equal", deployment.Name),
		})
	}

	if deployment.Status.UpdatedReplicas == replicas {
		conditions = append(conditions, metav1.Condition{
			Type:               DeploymentUpgradingCondition,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: generation,
			Reason:             "AllDeploymentReplicasUpdated",
			Message:            fmt.Sprintf("Number of desired and updated replicas of deployment %q are equal", deployment.Name),
		})
	} else {
		conditions = append(conditions, metav1.Condition{
			Type:               DeploymentUpgradingCondition,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: generation,
			Reason:             "AllDeploymentReplicasNotUpdated",
			Message:            fmt.Sprintf("Number of desired and updated replicas of deployment %q are not equal", deployment.Name),
		})
	}

	return conditions
}

// mergeConditions updates the conditions list with new conditions.
// Each condition is added if no condition of the same type already exists.
// Otherwise, the condition is merged with the existing condition of the same type.
func mergeConditions(conditions []metav1.Condition, updates ...metav1.Condition) []metav1.Condition {
	now := metav1.Now()
	for i, update := range updates {
		add := true
		for j, cond := range conditions {
			if cond.Type == update.Type {
				add = false
				if conditionChanged(cond, update) {
					conditions[j] = update
					conditions[j].LastTransitionTime = now
					break
				}
			}
		}
		if add {
			updates[i].LastTransitionTime = now
			conditions = append(conditions, updates[i])
		}
	}
	return conditions
}

func conditionChanged(a, b metav1.Condition) bool {
	return a.Status != b.Status || a.Reason != b.Reason || a.Message != b.Message
}

func haveConditionsChanged(current []metav1.Condition, desired []metav1.Condition) bool {
	var opts cmp.Options = cmp.Options{
		cmpopts.EquateEmpty(),
		cmpopts.SortSlices(func(a, b metav1.Condition) bool { return a.Type < b.Type }),
	}
	return !cmp.Equal(current, desired, opts)
}

func (r *AWSLoadBalancerControllerReconciler) updateStatusSubnets(ctx context.Context, controller *albo.AWSLoadBalancerController, internal []string, public []string, untagged []string, tagged []string, policy albo.SubnetTaggingPolicy) error {
	updatedALBC := controller.DeepCopy()
	var updated bool

	if updatedALBC.Status.Subnets == nil {
		updatedALBC.Status.Subnets = &albo.AWSLoadBalancerControllerStatusSubnets{}
		updated = true
	}

	if updatedALBC.Status.Subnets.SubnetTagging != policy {
		updatedALBC.Status.Subnets.SubnetTagging = policy
		updated = true
	}

	if !equalStrings(updatedALBC.Status.Subnets.Internal, internal) {
		updatedALBC.Status.Subnets.Internal = internal
		updated = true
	}
	if !equalStrings(updatedALBC.Status.Subnets.Public, public) {
		updatedALBC.Status.Subnets.Public = public
		updated = true
	}
	if !equalStrings(updatedALBC.Status.Subnets.Tagged, tagged) {
		updatedALBC.Status.Subnets.Tagged = tagged
		updated = true
	}
	if !equalStrings(updatedALBC.Status.Subnets.Untagged, untagged) {
		updatedALBC.Status.Subnets.Untagged = untagged
		updated = true
	}

	if updated {
		return r.Status().Update(ctx, updatedALBC)
	}
	return nil
}

func equalStrings(x1, x2 []string) bool {
	if len(x1) != len(x2) {
		return false
	}
	x1c := make([]string, len(x1))
	x2c := make([]string, len(x2))
	copy(x1c, x1)
	copy(x2c, x2)

	sort.Strings(x1c)
	sort.Strings(x2c)
	return cmp.Equal(x1c, x2c)
}

func (r *AWSLoadBalancerControllerReconciler) updateStatusIngressClass(ctx context.Context, controller *albo.AWSLoadBalancerController, ingressClass string) error {
	if controller.Status.IngressClass == ingressClass {
		return nil
	}

	updated := controller.DeepCopy()
	updated.Status.IngressClass = ingressClass
	return r.Status().Update(ctx, updated)
}
