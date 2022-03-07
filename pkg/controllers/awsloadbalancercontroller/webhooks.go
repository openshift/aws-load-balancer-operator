package awsloadbalancercontroller

import (
	"context"

	corev1 "k8s.io/api/core/v1"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
)

func (r *AWSLoadBalancerControllerReconciler) ensureWebhooks(ctx context.Context, controller *albo.AWSLoadBalancerController, service *corev1.Service) error {
	return nil
}
