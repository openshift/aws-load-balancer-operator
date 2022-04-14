package awsloadbalancercontroller

import (
	"context"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

// verifyClusterRole verifies whether the given cluster role exists.
func (r *AWSLoadBalancerControllerReconciler) verifyClusterRole(ctx context.Context, name string) (bool, error) {
	log.FromContext(ctx).Info("verifying clusterrole", "name", name)

	obj := &rbacv1.ClusterRole{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: name}, obj); err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
