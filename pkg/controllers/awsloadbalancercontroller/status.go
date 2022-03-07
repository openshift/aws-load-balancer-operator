package awsloadbalancercontroller

import (
	"context"
	"sort"

	appsv1 "k8s.io/api/apps/v1"

	"github.com/google/go-cmp/cmp"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
)

func (r *AWSLoadBalancerControllerReconciler) updateControllerStatus(ctx context.Context, controller *albo.AWSLoadBalancerController, deployment *appsv1.Deployment) error {
	return nil
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
