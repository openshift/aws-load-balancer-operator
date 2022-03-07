package awsloadbalancercontroller

import (
	"context"
	"fmt"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
)

func (r *AWSLoadBalancerControllerReconciler) ensureService(ctx context.Context, namespace string, controller *v1alpha1.AWSLoadBalancerController, deployment *appsv1.Deployment) (*corev1.Service, error) {
	serviceName := types.NamespacedName{
		Name:      fmt.Sprintf("aws-load-balancer-controller-%s", controller.Name),
		Namespace: namespace,
	}

	desired := desiredService(serviceName.Name, serviceName.Namespace, deployment.Spec.Selector.MatchLabels)
	err := controllerutil.SetOwnerReference(controller, desired, r.Scheme)
	if err != nil {
		return nil, fmt.Errorf("failed to set owner reference on desired service: %w", err)
	}

	var service corev1.Service
	err = r.Get(ctx, serviceName, &service)
	if err != nil && !errors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get existing service %q: %w", serviceName, err)
	}
	if err != nil && errors.IsNotFound(err) {
		return desired, r.Create(ctx, desired)
	}
	return r.updateService(ctx, &service, desired)
}

func desiredService(name, namespace string, selector map[string]string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "webhook",
					Port:       controllerWebhookPort,
					TargetPort: intstr.FromInt(controllerWebhookPort),
				},
				{
					Name:       "metrics",
					Port:       controllerMetricsPort,
					TargetPort: intstr.FromInt(controllerMetricsPort),
				},
			},
			Selector: selector,
			Type:     corev1.ServiceTypeClusterIP,
		},
	}
}

func (r *AWSLoadBalancerControllerReconciler) updateService(ctx context.Context, current, desired *corev1.Service) (*corev1.Service, error) {
	updatedService := current.DeepCopy()
	var updated bool

	if !portsMatch(updatedService.Spec.Ports, desired.Spec.Ports) {
		updatedService.Spec.Ports = desired.Spec.Ports
		updated = true
	}
	if !equality.Semantic.DeepEqual(updatedService.Spec.Selector, desired.Spec.Selector) {
		updatedService.Spec.Selector = desired.Spec.Selector
		updated = true
	}

	if updatedService.Spec.Type != desired.Spec.Type {
		updatedService.Spec.Type = desired.Spec.Type
	}

	if updated {
		return updatedService, r.Update(ctx, updatedService)
	}
	return updatedService, nil
}

type SortableServicePort []corev1.ServicePort

func (s SortableServicePort) Len() int {
	return len(s)
}

func (s SortableServicePort) Less(i, j int) bool {
	return strings.Compare(s[i].Name, s[j].Name) < 0
}

func (s SortableServicePort) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func portsMatch(current, desired SortableServicePort) bool {
	if len(current) != len(desired) {
		return false
	}
	currentCopy := make(SortableServicePort, len(current))
	copy(currentCopy, current)
	sort.Sort(currentCopy)
	desiredCopy := make(SortableServicePort, len(desired))
	copy(desiredCopy, desired)
	sort.Sort(desiredCopy)

	for i := 0; i < len(currentCopy); i++ {
		c := currentCopy[i]
		d := desiredCopy[i]
		if c.Name != d.Name || c.Port != d.Port || c.TargetPort.IntVal != d.TargetPort.IntVal {
			return false
		}
	}
	return true
}
