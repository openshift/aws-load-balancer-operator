package awsloadbalancercontroller

import (
	"context"
	"fmt"
	"reflect"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
)

// ensureClusterRole creates and maintains a clusterrole required by the aws-load-balancer controller.
func (r *AWSLoadBalancerControllerReconciler) ensureClusterRole(ctx context.Context, controller *albo.AWSLoadBalancerController) error {
	reqLogger := log.FromContext(ctx)

	desired := desiredClusterRole(ctx)

	reqLogger.Info("ensuring clusterroles", "clusterroles", desired.Name)

	if err := controllerutil.SetControllerReference(controller, desired, r.Scheme); err != nil {
		return fmt.Errorf("failed to set the controller reference for clusterroles %s : %w", desired.Name, err)
	}

	exist, current, err := r.currentClusterRole(ctx, desired)
	if err != nil {
		return fmt.Errorf("failed to fetch current clusterroles: %w", err)
	}

	if !exist {
		if err := r.createClusterRole(ctx, desired); err != nil {
			return err
		}
		_, current, err = r.currentClusterRole(ctx, desired)
		if err != nil {
			return fmt.Errorf("failed to fetch created clusterroles: %w", err)
		}
		reqLogger.Info("created clusterroles", "clusterroles", desired.Name)
	}

	if err := r.updateClusterRole(ctx, current, desired); err != nil {
		return fmt.Errorf("failed to update clusterroles: %w", err)
	}

	return nil
}

// updateClusterRoles updates the current clusterroles and returns a flag to denote if the update was done.
func (r *AWSLoadBalancerControllerReconciler) updateClusterRole(ctx context.Context, current *rbacv1.ClusterRole, desired *rbacv1.ClusterRole) error {
	changed := hasClusterRoleChanged(current, desired)

	if !changed {
		return nil
	}

	updated := current.DeepCopy()
	updated.Rules = desired.Rules

	if err := r.Client.Update(ctx, updated); err != nil {
		return err
	}

	return nil
}

func hasClusterRoleChanged(current *rbacv1.ClusterRole, desired *rbacv1.ClusterRole) bool {
	return !(reflect.DeepEqual(current.Rules, desired.Rules))
}

func (r *AWSLoadBalancerControllerReconciler) createClusterRole(ctx context.Context, role *rbacv1.ClusterRole) error {
	if err := r.Client.Create(ctx, role); err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create clusterrole %s/%s: %w", role.Namespace, role.Name, err)
		}
	}

	return nil
}

func (r *AWSLoadBalancerControllerReconciler) currentClusterRole(ctx context.Context, role *rbacv1.ClusterRole) (bool, *rbacv1.ClusterRole, error) {
	obj := &rbacv1.ClusterRole{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: role.Name}, obj); err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}
	return true, obj, nil
}

func desiredClusterRole(ctx context.Context) *rbacv1.ClusterRole {
	return buildClusterRole(commonResourceName, getControllerRules())
}

func buildClusterRole(name string, rules []rbacv1.PolicyRule) *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		ObjectMeta: v1.ObjectMeta{
			Name: name,
		},
		Rules: rules,
	}
}
