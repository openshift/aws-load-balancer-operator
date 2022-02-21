package awsloadbalancercontroller

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
)

func (r *AWSLoadBalancerControllerReconciler) updateControllerStatus(ctx context.Context, controller *albo.AWSLoadBalancerController, deployment *appsv1.Deployment) error {
	return nil
}
