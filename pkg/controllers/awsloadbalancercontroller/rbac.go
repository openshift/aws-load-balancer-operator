package awsloadbalancercontroller

import (
	"context"

	corev1 "k8s.io/api/core/v1"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
)

func (r *AWSLoadBalancerControllerReconciler) ensureClusterRoleAndBinding(ctx context.Context, sa *corev1.ServiceAccount, controller *albo.AWSLoadBalancerController) error {
	err := r.ensureClusterRole(ctx, controller)
	if err != nil {
		return err
	}

	err = r.ensureClusterRoleBinding(ctx, sa, controller)
	if err != nil {
		return err
	}

	err = r.ensureRole(ctx, controller)
	if err != nil {
		return err
	}

	err = r.ensureRoleBinding(ctx, sa, controller)
	if err != nil {
		return err
	}

	return nil
}
