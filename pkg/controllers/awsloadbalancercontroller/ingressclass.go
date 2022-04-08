package awsloadbalancercontroller

import (
	"context"
	"fmt"

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
)

const (
	albIngressClassController = "ingress.k8s.aws/alb"
)

// ensureIngressClass create the default IngressClass which is specified in the controller. This is required because the OpenShift router
// reconciles any Ingress resource whose class is not defined or if the IngressClass does not have the spec.controllerName set.
// Steps to ensure the IngressClass
// 1. Check the status to see if the IngressClass already exists
// 2. If the name matches then do nothing.
// 3. If the name does not match then delete the existing IngressClass. Ignore if it doesn't exist.
// 4. Create the new IngresClass with the correct controller name. If there is an AlreadyExists error, ignore it.
func (r *AWSLoadBalancerControllerReconciler) ensureIngressClass(ctx context.Context, controller *albo.AWSLoadBalancerController) error {

	// if the current ingress class is the same then do nothing.
	if controller.Status.IngressClass == controller.Spec.IngressClass {
		return nil
	}

	// if the current ingress class name does not match then delete it.
	if controller.Status.IngressClass != "" {
		err := r.Delete(ctx, &networkingv1.IngressClass{ObjectMeta: metav1.ObjectMeta{Name: controller.Status.IngressClass}})
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete existing IngressClass %q: %w", controller.Status.IngressClass, err)
		}
	}

	ingressClass := desiredIngressClass(controller.Spec.IngressClass)
	err := controllerutil.SetControllerReference(controller, ingressClass, r.Scheme)
	if err != nil {
		return fmt.Errorf("failed to set owner reference on new IngressClass %q: %w", ingressClass.Name, err)
	}

	err = r.Create(ctx, ingressClass)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create default IngressClass %s: %w", controller.Spec.IngressClass, err)
	}
	return nil
}

func desiredIngressClass(name string) *networkingv1.IngressClass {
	return &networkingv1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: networkingv1.IngressClassSpec{
			Controller: albIngressClassController,
		},
	}
}
