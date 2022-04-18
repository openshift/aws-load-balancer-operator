package awsloadbalancercontroller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/aws/aws-sdk-go-v2/aws"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
)

func (r *AWSLoadBalancerControllerReconciler) ensureControllerServiceAccount(ctx context.Context, namespace string, controller *albo.AWSLoadBalancerController) (*corev1.ServiceAccount, error) {
	nsName := types.NamespacedName{Namespace: r.Namespace, Name: fmt.Sprintf("%s-%s", controllerResourcePrefix, controller.Name)}

	reqLogger := log.FromContext(ctx).WithValues("serviceaccout", nsName)

	desired := desiredAWSLoadBalancerServiceAccount(namespace, controller)

	if err := controllerutil.SetControllerReference(controller, desired, r.Scheme); err != nil {
		return nil, fmt.Errorf("failed to set the controller reference for serviceaccount %q: %w", nsName.Name, err)
	}

	exist, current, err := r.currentAWSLoadBalancerServiceAccount(ctx, nsName)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch current serviceaccount %q: %w", nsName.Name, err)
	}

	if !exist {
		if err := r.createAWSLoadBalancerServiceAccount(ctx, desired); err != nil {
			return nil, fmt.Errorf("failed to fetch created serviceaccount %q: %w", nsName.Name, err)
		}
		reqLogger.Info("successfully created serviceaccount")
		_, current, err = r.currentAWSLoadBalancerServiceAccount(ctx, nsName)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch current serviceaccount %q: %w", nsName.Name, err)
		}
	}

	return current, nil
}

// currentAWSLoadBalancerServiceAccount gets the current AWSLoadBalancer service account resource.
func (r *AWSLoadBalancerControllerReconciler) currentAWSLoadBalancerServiceAccount(ctx context.Context, nsName types.NamespacedName) (bool, *corev1.ServiceAccount, error) {
	sa := &corev1.ServiceAccount{}
	if err := r.Client.Get(ctx, nsName, sa); err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}
	return true, sa, nil
}

func desiredAWSLoadBalancerServiceAccount(namespace string, controller *albo.AWSLoadBalancerController) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      fmt.Sprintf("%s-%s", controllerResourcePrefix, controller.Name),
		},
		AutomountServiceAccountToken: aws.Bool(true),
	}
}

// createAWSLoadBalancerServiceAccount creates the given service account using the reconciler's client.
func (r *AWSLoadBalancerControllerReconciler) createAWSLoadBalancerServiceAccount(ctx context.Context, sa *corev1.ServiceAccount) error {
	if err := r.Client.Create(ctx, sa); err != nil {
		return fmt.Errorf("failed to create aws-load-balancer serviceaccount %s/%s: %w", sa.Namespace, sa.Name, err)
	}

	return nil
}
