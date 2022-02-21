package awsloadbalancercontroller

import (
	"context"

	corev1 "k8s.io/api/core/v1"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
)

func (r *AWSLoadBalancerControllerReconciler) ensureControllerServiceAccount(ctx context.Context, namespace string, controller *albo.AWSLoadBalancerController) (bool, *corev1.ServiceAccount, error) {
	return true, nil, nil
}
