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

	albo "github.com/openshift/aws-load-balancer-operator/api/v1"
)

func (r *AWSLoadBalancerControllerReconciler) ensureRole(ctx context.Context, controller *albo.AWSLoadBalancerController) error {
	reqLogger := log.FromContext(ctx)

	desired := desiredRole(ctx, r.Namespace, fmt.Sprintf("%s-%s", controllerResourcePrefix, controller.Name))
	reqLogger.Info("ensuring roles", "roles", desired.Name)

	if err := controllerutil.SetControllerReference(controller, desired, r.Scheme); err != nil {
		return fmt.Errorf("failed to set the controller reference for roles %s : %w", desired.Name, err)
	}

	exist, current, err := r.currentRole(ctx, desired)
	if err != nil {
		return fmt.Errorf("failed to fetch current roles: %w", err)
	}

	if !exist {
		if err := r.createRole(ctx, desired); err != nil {
			return err
		}

		_, current, err = r.currentRole(ctx, desired)
		if err != nil {
			return fmt.Errorf("failed to fetch created roles: %w", err)
		}
		reqLogger.Info("created roles", "roles", desired.Name)
	}

	if err := r.updateRole(ctx, current, desired); err != nil {
		return fmt.Errorf("failed to update roles: %w", err)
	}

	return nil
}

// updateRoles updates the current roles and returns a flag to denote if the update was done.
func (r *AWSLoadBalancerControllerReconciler) updateRole(ctx context.Context, current *rbacv1.Role, desired *rbacv1.Role) error {
	changed := hasRoleChanged(current, desired)

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

func hasRoleChanged(current *rbacv1.Role, desired *rbacv1.Role) bool {
	return !(reflect.DeepEqual(current.Rules, desired.Rules))
}

func (r *AWSLoadBalancerControllerReconciler) createRole(ctx context.Context, role *rbacv1.Role) error {
	if err := r.Client.Create(ctx, role); err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create roles %s/%s: %w", role.Namespace, role.Name, err)
		}
	}
	return nil
}

func (r *AWSLoadBalancerControllerReconciler) currentRole(ctx context.Context, role *rbacv1.Role) (bool, *rbacv1.Role, error) {
	obj := &rbacv1.Role{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: role.Name, Namespace: role.Namespace}, obj); err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}

	return true, obj, nil
}

func desiredRole(ctx context.Context, namespace string, name string) *rbacv1.Role {
	return buildRole(name, namespace, getLeaderElectionRules())
}

func buildRole(name, namespace string, rules []rbacv1.PolicyRule) *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Rules: rules,
	}
}
