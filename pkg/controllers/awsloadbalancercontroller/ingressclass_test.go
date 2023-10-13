package awsloadbalancercontroller

import (
	"context"
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1"
	"github.com/openshift/aws-load-balancer-operator/pkg/utils/test"
)

func TestDesiredIngressClass(t *testing.T) {
	ic := desiredIngressClass("test")
	if ic.Name != "test" {
		t.Errorf("unexpected name in desired ingress class, expected %q, got %q", "test", ic.Name)
	}
	if ic.Spec.Controller != albIngressClassController {
		t.Errorf("unexpected controller in desired ingress class, expected %q, got %q", albIngressClassController, ic.Spec.Controller)
	}
}

func TestEnsureIngressClass(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		existingIngressClass *networkingv1.IngressClass
		ingressClassName     string
		deletedIngressClass  bool
	}{
		{
			name:             "no existing ingress class",
			ingressClassName: "new",
		},
		{
			name:                 "existing ingress class",
			existingIngressClass: desiredIngressClass("old"),
			ingressClassName:     "new",
			deletedIngressClass:  true,
		},
		{
			name:                 "existing ingress class, name no change",
			existingIngressClass: desiredIngressClass("old"),
			ingressClassName:     "old",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var existingObjects []client.Object
			if tc.existingIngressClass != nil {
				existingObjects = append(existingObjects, tc.existingIngressClass)
			}
			controller := &albo.AWSLoadBalancerController{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: albo.AWSLoadBalancerControllerSpec{
					IngressClass: tc.ingressClassName,
				},
			}
			if tc.existingIngressClass != nil {
				controller.Status.IngressClass = tc.existingIngressClass.Name
			}
			existingObjects = append(existingObjects, controller)
			testClient := fake.NewClientBuilder().WithScheme(test.Scheme).WithObjects(existingObjects...).Build()
			r := &AWSLoadBalancerControllerReconciler{
				Scheme: test.Scheme,
				Client: testClient,
			}
			err := r.ensureIngressClass(context.Background(), controller)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			var ingressClass networkingv1.IngressClass
			err = testClient.Get(context.Background(), types.NamespacedName{Name: tc.ingressClassName}, &ingressClass)
			if err != nil {
				t.Fatalf("failed to get ingress class %q: %v", tc.ingressClassName, err)
			}
			if ingressClass.Spec.Controller != albIngressClassController {
				t.Errorf("IngressClass does not have correct controller name, expected %q, got %q", albIngressClassController, ingressClass.Spec.Controller)
			}
			if tc.deletedIngressClass {
				var ic networkingv1.IngressClass
				err = r.Get(context.Background(), types.NamespacedName{Name: tc.existingIngressClass.Name}, &ic)
				if err != nil && !errors.IsNotFound(err) {
					t.Errorf("failed to get ingress class %q: %v", tc.existingIngressClass.Name, err)
					return
				}
				if err == nil {
					t.Errorf("existing ingress class %q was not deleted", tc.existingIngressClass.Name)
				}
			}
		})
	}
}
