package awsloadbalancercontroller

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/google/go-cmp/cmp"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
	"github.com/openshift/aws-load-balancer-operator/pkg/controllers/utils/test"
)

func testControllerService(name, namespace string, selector, annotations map[string]string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
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
		},
	}
}

func TestEnsureService(t *testing.T) {
	for _, tc := range []struct {
		name            string
		existingObjects []client.Object
		controller      *v1alpha1.AWSLoadBalancerController
		deployment      *appsv1.Deployment
		expectedService *corev1.Service
	}{
		{
			name: "new service",
			controller: &v1alpha1.AWSLoadBalancerController{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			},
			deployment: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "controller"},
					},
				},
			},
			expectedService: testControllerService(
				"aws-load-balancer-controller-test",
				"test-namespace",
				map[string]string{"app": "controller"},
				map[string]string{servingSecretAnnotationName: "serving-secret"},
			),
		},
		{
			name: "existing service, selector modified",
			controller: &v1alpha1.AWSLoadBalancerController{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			},
			existingObjects: []client.Object{
				testControllerService(
					"aws-load-balancer-controller-test",
					"test-namespace",
					map[string]string{"app": "controller-old"},
					map[string]string{servingSecretAnnotationName: "serving-secret"},
				),
			},
			deployment: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "controller"},
					},
				},
			},
			expectedService: testControllerService(
				"aws-load-balancer-controller-test",
				"test-namespace",
				map[string]string{"app": "controller"},
				map[string]string{servingSecretAnnotationName: "serving-secret"},
			),
		},
		{
			name: "existing service, ports modified",
			controller: &v1alpha1.AWSLoadBalancerController{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			},
			existingObjects: []client.Object{
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{Name: "aws-load-balancer-controller-test", Namespace: "test-namespace"},
					Spec: corev1.ServiceSpec{
						Type: corev1.ServiceTypeClusterIP,
						Ports: []corev1.ServicePort{
							{
								Name:       "webhook",
								Port:       9440,
								TargetPort: intstr.FromInt(9440),
							},
							{
								Name:       "metrics",
								Port:       controllerMetricsPort,
								TargetPort: intstr.FromInt(controllerMetricsPort),
							},
						},
						Selector: map[string]string{"app": "controller"},
					},
				},
			},
			deployment: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "controller"},
					},
				},
			},
			expectedService: testControllerService(
				"aws-load-balancer-controller-test",
				"test-namespace",
				map[string]string{"app": "controller"},
				map[string]string{servingSecretAnnotationName: "serving-secret"},
			),
		},
		{
			name: "existing service, service type modified",
			controller: &v1alpha1.AWSLoadBalancerController{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			},
			existingObjects: []client.Object{
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name: "aws-load-balancer-controller-test", Namespace: "test-namespace",
						Annotations: map[string]string{servingSecretAnnotationName: ""},
					},
					Spec: corev1.ServiceSpec{
						Type: corev1.ServiceTypeNodePort,
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
						Selector: map[string]string{"app": "controller"},
					},
				},
			},
			deployment: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "controller"},
					},
				},
			},
			expectedService: testControllerService(
				"aws-load-balancer-controller-test",
				"test-namespace",
				map[string]string{"app": "controller"},
				map[string]string{servingSecretAnnotationName: "serving-secret"},
			),
		},
		{
			name: "existing service, extra annotations present",
			controller: &v1alpha1.AWSLoadBalancerController{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			},
			existingObjects: []client.Object{
				testControllerService(
					"aws-load-balancer-controller-test",
					"test-namespace",
					map[string]string{"app": "controller"},
					map[string]string{"extra-annotation-key": "extra-annotation-value"},
				),
			},
			deployment: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "controller"},
					},
				},
			},
			expectedService: testControllerService(
				"aws-load-balancer-controller-test",
				"test-namespace",
				map[string]string{"app": "controller"},
				map[string]string{
					servingSecretAnnotationName: "serving-secret",
					"extra-annotation-key":      "extra-annotation-value",
				},
			),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testClient := fake.NewClientBuilder().WithObjects(tc.existingObjects...).WithScheme(test.Scheme).Build()
			r := &AWSLoadBalancerControllerReconciler{
				Client: testClient,
				Scheme: test.Scheme,
			}
			_, err := r.ensureService(context.Background(), "test-namespace", tc.controller, "serving-secret", tc.deployment)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			var s corev1.Service
			err = testClient.Get(context.Background(), types.NamespacedName{Name: tc.expectedService.Name, Namespace: tc.expectedService.Namespace}, &s)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if diff := cmp.Diff(tc.expectedService.Annotations, s.Annotations); diff != "" {
				t.Errorf("unexpected annotations\n%s", diff)
			}

			if !equality.Semantic.DeepEqual(s.Spec, tc.expectedService.Spec) {
				t.Errorf("service has unexpected configuration:\n%s", cmp.Diff(s.Spec, tc.expectedService.Spec))
			}
		})
	}
}
