package awsloadbalancercontroller

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1"
)

func (r *AWSLoadBalancerControllerReconciler) ensureRoleBinding(ctx context.Context, sa *corev1.ServiceAccount, controller *albo.AWSLoadBalancerController) error {
	reqLogger := log.FromContext(ctx)

	desired := desiredRoleBinding(ctx, fmt.Sprintf("%s-%s", controllerResourcePrefix, controller.Name), r.Namespace, sa)
	reqLogger.Info("ensuring rolebindings", "rolebindings", desired.Name)

	if err := controllerutil.SetControllerReference(controller, desired, r.Scheme); err != nil {
		return fmt.Errorf("failed to set the controller reference for rolebindings %s : %w", desired.Name, err)
	}

	exist, current, err := r.currentRoleBinding(ctx, desired)
	if err != nil {
		return fmt.Errorf("failed to fetch current rolebindings: %w", err)
	}

	if !exist {
		if err := r.createRoleBinding(ctx, desired); err != nil {
			return err
		}

		_, current, err = r.currentRoleBinding(ctx, desired)
		if err != nil {
			return fmt.Errorf("failed to fetch created rolebindings: %w", err)
		}
		reqLogger.Info("created rolebindings", "rolebindings", desired.Name)
	}

	if err := r.updateRoleBinding(ctx, current, desired); err != nil {
		return fmt.Errorf("failed to update rolebindings: %w", err)
	}

	return nil
}

// updateRoleBindings updates the current rolebindings and returns a flag to denote if the update was done.
func (r *AWSLoadBalancerControllerReconciler) updateRoleBinding(ctx context.Context, current *rbacv1.RoleBinding, desired *rbacv1.RoleBinding) error {
	changed := hasRoleBindingChanged(current, desired)

	if !changed {
		return nil
	}
	updated := current.DeepCopy()
	updated.Subjects = desired.Subjects
	updated.RoleRef = desired.RoleRef

	if err := r.Client.Update(ctx, updated); err != nil {
		return err
	}

	return nil
}

func hasRoleBindingChanged(current *rbacv1.RoleBinding, desired *rbacv1.RoleBinding) bool {
	if !(reflect.DeepEqual(current.Subjects, desired.Subjects)) || !(reflect.DeepEqual(current.RoleRef, desired.RoleRef)) {
		return true
	}

	return false
}

func (r *AWSLoadBalancerControllerReconciler) createRoleBinding(ctx context.Context, role *rbacv1.RoleBinding) error {
	if err := r.Client.Create(ctx, role); err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create rolebindings %s/%s: %w", role.Namespace, role.Name, err)
		}
	}
	return nil
}

func (r *AWSLoadBalancerControllerReconciler) currentRoleBinding(ctx context.Context, role *rbacv1.RoleBinding) (bool, *rbacv1.RoleBinding, error) {
	obj := &rbacv1.RoleBinding{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: role.Name, Namespace: role.Namespace}, obj); err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}

	return true, obj, nil
}

func desiredRoleBinding(ctx context.Context, name, namespace string, sa *corev1.ServiceAccount) *rbacv1.RoleBinding {
	return buildRoleBinding(name, namespace, name, sa)
}

func buildRoleBinding(name, namespace, role string, sa *corev1.ServiceAccount) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     role,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      sa.Name,
				Namespace: sa.Namespace,
			},
		},
	}
}
