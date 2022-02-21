package awsloadbalancercontroller

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
)

func (r *AWSLoadBalancerControllerReconciler) ensureControllerDeployment(ctx context.Context, namespace string, image string, sa *corev1.ServiceAccount, controller *albo.AWSLoadBalancerController) (*appsv1.Deployment, error) {
	return nil, nil
}
