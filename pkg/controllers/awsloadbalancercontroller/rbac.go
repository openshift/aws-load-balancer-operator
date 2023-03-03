package awsloadbalancercontroller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1"
)

const (
	controllerClusterRoleName = "aws-load-balancer-operator-controller-role"
)

func (r *AWSLoadBalancerControllerReconciler) ensureClusterRoleAndBinding(ctx context.Context, sa *corev1.ServiceAccount, controller *albo.AWSLoadBalancerController) error {
	exists, err := r.verifyClusterRole(ctx, controllerClusterRoleName)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("cluster role %q doesn't exist", controllerClusterRoleName)
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
