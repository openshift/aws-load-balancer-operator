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

func (r *AWSLoadBalancerControllerReconciler) ensureClusterRoleBinding(ctx context.Context, sa *corev1.ServiceAccount, controller *albo.AWSLoadBalancerController) error {
	reqLogger := log.FromContext(ctx)

	desired := desiredClusterRoleBinding(ctx, sa, fmt.Sprintf("%s-%s", controllerResourcePrefix, controller.Name))
	reqLogger.Info("ensuring clusterrolebindings", "clusterrolebindings", desired.Name)

	if err := controllerutil.SetControllerReference(controller, desired, r.Scheme); err != nil {
		return fmt.Errorf("failed to set the controller reference for clusterrolebindings %s : %w", desired.Name, err)
	}

	exist, current, err := r.currentClusterRoleBinding(ctx, desired)
	if err != nil {
		return fmt.Errorf("failed to fetch current clusterrolebindings: %w", err)
	}

	if !exist {
		if err := r.createClusterRoleBinding(ctx, desired); err != nil {
			return err
		}

		_, current, err = r.currentClusterRoleBinding(ctx, desired)
		if err != nil {
			return fmt.Errorf("failed to fetch created clusterrolebindings: %w", err)
		}
		reqLogger.Info("created clusterrolebindings", "clusterrolebindings", desired.Name)
	}

	if err := r.updateClusterRoleBinding(ctx, current, desired); err != nil {
		return fmt.Errorf("failed to update clusterrolebindings: %w", err)
	}

	return nil
}

// updateClusterRoleBindings updates the current clusterrolebindings and returns a flag to denote if the update was done.
func (r *AWSLoadBalancerControllerReconciler) updateClusterRoleBinding(ctx context.Context, current *rbacv1.ClusterRoleBinding, desired *rbacv1.ClusterRoleBinding) error {
	changed := hasClusterRoleBindingChanged(current, desired)

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

func hasClusterRoleBindingChanged(current *rbacv1.ClusterRoleBinding, desired *rbacv1.ClusterRoleBinding) bool {
	if !(reflect.DeepEqual(current.Subjects, desired.Subjects)) || !(reflect.DeepEqual(current.RoleRef, desired.RoleRef)) {
		return true
	}

	return false
}

func (r *AWSLoadBalancerControllerReconciler) createClusterRoleBinding(ctx context.Context, role *rbacv1.ClusterRoleBinding) error {
	if err := r.Client.Create(ctx, role); err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create clusterrolebinding %s/%s: %w", role.Namespace, role.Name, err)
		}
	}

	return nil
}

func (r *AWSLoadBalancerControllerReconciler) currentClusterRoleBinding(ctx context.Context, role *rbacv1.ClusterRoleBinding) (bool, *rbacv1.ClusterRoleBinding, error) {
	obj := &rbacv1.ClusterRoleBinding{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: role.Name, Namespace: role.Namespace}, obj); err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}

	return true, obj, nil
}

func desiredClusterRoleBinding(ctx context.Context, sa *corev1.ServiceAccount, name string) *rbacv1.ClusterRoleBinding {
	return buildClusterRoleBinding(name, controllerClusterRoleName, sa)
}

func buildClusterRoleBinding(name, clusterrole string, sa *corev1.ServiceAccount) *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name: name,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     clusterrole,
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
