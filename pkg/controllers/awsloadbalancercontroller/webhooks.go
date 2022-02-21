package awsloadbalancercontroller

import (
	"context"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
)

func (r *AWSLoadBalancerControllerReconciler) ensureWebhooks(ctx context.Context, controller *albo.AWSLoadBalancerController) error {
	return nil
}
