package awsloadbalancercontroller

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	arv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1"
	"github.com/openshift/aws-load-balancer-operator/pkg/controllers/utils/test"
)

func TestAreValidatingWebhooksSame(t *testing.T) {
	for _, tc := range []struct {
		name           string
		currentVWs     []arv1.ValidatingWebhook
		desiredVWs     []arv1.ValidatingWebhook
		expectedResult bool
	}{
		{
			name:           "second empty",
			currentVWs:     []arv1.ValidatingWebhook{{}},
			desiredVWs:     []arv1.ValidatingWebhook{},
			expectedResult: true,
		},
		{
			name:           "first empty",
			currentVWs:     []arv1.ValidatingWebhook{},
			desiredVWs:     []arv1.ValidatingWebhook{{}},
			expectedResult: true,
		},
		{
			name:       "same webhooks, different order",
			currentVWs: []arv1.ValidatingWebhook{{Name: "a"}, {Name: "b"}},
			desiredVWs: []arv1.ValidatingWebhook{{Name: "b"}, {Name: "a"}},
		},
		{
			name: "same service",
			currentVWs: []arv1.ValidatingWebhook{
				{
					Name: "a", ClientConfig: arv1.WebhookClientConfig{
						Service: &arv1.ServiceReference{
							Namespace: "test-namespace",
							Name:      "test-service",
							Path:      pointer.StringPtr("/test"),
							Port:      pointer.Int32Ptr(8080),
						},
					},
				},
			},
			desiredVWs: []arv1.ValidatingWebhook{
				{
					Name: "a", ClientConfig: arv1.WebhookClientConfig{
						Service: &arv1.ServiceReference{
							Namespace: "test-namespace",
							Name:      "test-service",
							Path:      pointer.StringPtr("/test"),
							Port:      pointer.Int32Ptr(8080),
						},
					},
				},
			},
		},
		{
			name: "service path changed",
			currentVWs: []arv1.ValidatingWebhook{
				{
					Name: "a", ClientConfig: arv1.WebhookClientConfig{
						Service: &arv1.ServiceReference{
							Namespace: "test-namespace",
							Name:      "test-service",
							Path:      pointer.StringPtr("/test-old"),
							Port:      pointer.Int32Ptr(8080),
						},
					},
				},
			},
			desiredVWs: []arv1.ValidatingWebhook{
				{
					Name: "a", ClientConfig: arv1.WebhookClientConfig{
						Service: &arv1.ServiceReference{
							Namespace: "test-namespace",
							Name:      "test-service",
							Path:      pointer.StringPtr("/test-new"),
							Port:      pointer.Int32Ptr(8080),
						},
					},
				},
			},
			expectedResult: true,
		},
		{
			name:       "desired side effect is nil",
			currentVWs: []arv1.ValidatingWebhook{{Name: "a", SideEffects: sideEffectPtr(arv1.SideEffectClassNone)}},
			desiredVWs: []arv1.ValidatingWebhook{{Name: "a"}},
		},
		{
			name:           "current side effect is nil",
			currentVWs:     []arv1.ValidatingWebhook{{Name: "a"}},
			desiredVWs:     []arv1.ValidatingWebhook{{Name: "a", SideEffects: sideEffectPtr(arv1.SideEffectClassNone)}},
			expectedResult: true,
		},
		{
			name:           "current and desired side effects differ",
			currentVWs:     []arv1.ValidatingWebhook{{Name: "a", SideEffects: sideEffectPtr(arv1.SideEffectClassSome)}},
			desiredVWs:     []arv1.ValidatingWebhook{{Name: "a", SideEffects: sideEffectPtr(arv1.SideEffectClassNone)}},
			expectedResult: true,
		},
		{
			name:       "desired match policy is nil",
			currentVWs: []arv1.ValidatingWebhook{{Name: "a", MatchPolicy: matchPolicyPtr(arv1.Equivalent)}},
			desiredVWs: []arv1.ValidatingWebhook{{Name: "a"}},
		},
		{
			name:           "current match policy is nil",
			currentVWs:     []arv1.ValidatingWebhook{{Name: "a"}},
			desiredVWs:     []arv1.ValidatingWebhook{{Name: "a", MatchPolicy: matchPolicyPtr(arv1.Equivalent)}},
			expectedResult: true,
		},
		{
			name:           "current and desired match policy differ",
			currentVWs:     []arv1.ValidatingWebhook{{Name: "a", MatchPolicy: matchPolicyPtr(arv1.Exact)}},
			desiredVWs:     []arv1.ValidatingWebhook{{Name: "a", MatchPolicy: matchPolicyPtr(arv1.Equivalent)}},
			expectedResult: true,
		},
		{
			name:       "desired failure policy is nil",
			currentVWs: []arv1.ValidatingWebhook{{Name: "a", FailurePolicy: failurePolicyPtr(arv1.Fail)}},
			desiredVWs: []arv1.ValidatingWebhook{{Name: "a"}},
		},
		{
			name:           "current failure policy is nil",
			currentVWs:     []arv1.ValidatingWebhook{{Name: "a"}},
			desiredVWs:     []arv1.ValidatingWebhook{{Name: "a", FailurePolicy: failurePolicyPtr(arv1.Fail)}},
			expectedResult: true,
		},
		{
			name:           "current and desired failure policy differ",
			currentVWs:     []arv1.ValidatingWebhook{{Name: "a", FailurePolicy: failurePolicyPtr(arv1.Ignore)}},
			desiredVWs:     []arv1.ValidatingWebhook{{Name: "a", FailurePolicy: failurePolicyPtr(arv1.Fail)}},
			expectedResult: true,
		},
		{
			name:       "rules have changed",
			currentVWs: []arv1.ValidatingWebhook{{Name: "a", Rules: []arv1.RuleWithOperations{}}},
			desiredVWs: []arv1.ValidatingWebhook{{Name: "a", Rules: []arv1.RuleWithOperations{
				{
					Operations: []arv1.OperationType{arv1.Create},
					Rule: arv1.Rule{
						APIGroups:   []string{"apps"},
						APIVersions: []string{"v1"},
						Resources:   []string{"deployments"},
						Scope:       scopeTypePtr(arv1.AllScopes),
					},
				},
			}}},
			expectedResult: true,
		},
		{
			name: "rules are the same",
			currentVWs: []arv1.ValidatingWebhook{{Name: "a", Rules: []arv1.RuleWithOperations{
				{
					Operations: []arv1.OperationType{arv1.Create},
					Rule: arv1.Rule{
						APIGroups:   []string{"apps"},
						APIVersions: []string{"v1"},
						Resources:   []string{"deployments"},
						Scope:       scopeTypePtr(arv1.AllScopes),
					},
				},
			}}},
			desiredVWs: []arv1.ValidatingWebhook{{Name: "a", Rules: []arv1.RuleWithOperations{
				{
					Operations: []arv1.OperationType{arv1.Create},
					Rule: arv1.Rule{
						APIGroups:   []string{"apps"},
						APIVersions: []string{"v1"},
						Resources:   []string{"deployments"},
						Scope:       scopeTypePtr(arv1.AllScopes),
					},
				},
			}}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := haveValidatingWebhooksChanged(tc.currentVWs, tc.desiredVWs)
			if result != tc.expectedResult {
				t.Errorf("expected result %v, got %v", tc.expectedResult, result)
			}
		})
	}
}

func TestAreMutatingWebhooksSame(t *testing.T) {
	for _, tc := range []struct {
		name           string
		currentVWs     []arv1.MutatingWebhook
		desiredVWs     []arv1.MutatingWebhook
		expectedResult bool
	}{
		{
			name:           "second empty",
			currentVWs:     []arv1.MutatingWebhook{{}},
			desiredVWs:     []arv1.MutatingWebhook{},
			expectedResult: true,
		},
		{
			name:           "first empty",
			currentVWs:     []arv1.MutatingWebhook{},
			desiredVWs:     []arv1.MutatingWebhook{{}},
			expectedResult: true,
		},
		{
			name:       "same webhooks, different order",
			currentVWs: []arv1.MutatingWebhook{{Name: "a"}, {Name: "b"}},
			desiredVWs: []arv1.MutatingWebhook{{Name: "b"}, {Name: "a"}},
		},
		{
			name: "same service",
			currentVWs: []arv1.MutatingWebhook{
				{
					Name: "a", ClientConfig: arv1.WebhookClientConfig{
						Service: &arv1.ServiceReference{
							Namespace: "test-namespace",
							Name:      "test-service",
							Path:      pointer.StringPtr("/test"),
							Port:      pointer.Int32Ptr(8080),
						},
					},
				},
			},
			desiredVWs: []arv1.MutatingWebhook{
				{
					Name: "a", ClientConfig: arv1.WebhookClientConfig{
						Service: &arv1.ServiceReference{
							Namespace: "test-namespace",
							Name:      "test-service",
							Path:      pointer.StringPtr("/test"),
							Port:      pointer.Int32Ptr(8080),
						},
					},
				},
			},
		},
		{
			name: "service path changed",
			currentVWs: []arv1.MutatingWebhook{
				{
					Name: "a", ClientConfig: arv1.WebhookClientConfig{
						Service: &arv1.ServiceReference{
							Namespace: "test-namespace",
							Name:      "test-service",
							Path:      pointer.StringPtr("/test-old"),
							Port:      pointer.Int32Ptr(8080),
						},
					},
				},
			},
			desiredVWs: []arv1.MutatingWebhook{
				{
					Name: "a", ClientConfig: arv1.WebhookClientConfig{
						Service: &arv1.ServiceReference{
							Namespace: "test-namespace",
							Name:      "test-service",
							Path:      pointer.StringPtr("/test-new"),
							Port:      pointer.Int32Ptr(8080),
						},
					},
				},
			},
			expectedResult: true,
		},
		{
			name:       "desired side effect is nil",
			currentVWs: []arv1.MutatingWebhook{{Name: "a", SideEffects: sideEffectPtr(arv1.SideEffectClassNone)}},
			desiredVWs: []arv1.MutatingWebhook{{Name: "a"}},
		},
		{
			name:           "current side effect is nil",
			currentVWs:     []arv1.MutatingWebhook{{Name: "a"}},
			desiredVWs:     []arv1.MutatingWebhook{{Name: "a", SideEffects: sideEffectPtr(arv1.SideEffectClassNone)}},
			expectedResult: true,
		},
		{
			name:           "current and desired side effects differ",
			currentVWs:     []arv1.MutatingWebhook{{Name: "a", SideEffects: sideEffectPtr(arv1.SideEffectClassSome)}},
			desiredVWs:     []arv1.MutatingWebhook{{Name: "a", SideEffects: sideEffectPtr(arv1.SideEffectClassNone)}},
			expectedResult: true,
		},
		{
			name:       "desired match policy is nil",
			currentVWs: []arv1.MutatingWebhook{{Name: "a", MatchPolicy: matchPolicyPtr(arv1.Equivalent)}},
			desiredVWs: []arv1.MutatingWebhook{{Name: "a"}},
		},
		{
			name:           "current match policy is nil",
			currentVWs:     []arv1.MutatingWebhook{{Name: "a"}},
			desiredVWs:     []arv1.MutatingWebhook{{Name: "a", MatchPolicy: matchPolicyPtr(arv1.Equivalent)}},
			expectedResult: true,
		},
		{
			name:           "current and desired match policy differ",
			currentVWs:     []arv1.MutatingWebhook{{Name: "a", MatchPolicy: matchPolicyPtr(arv1.Exact)}},
			desiredVWs:     []arv1.MutatingWebhook{{Name: "a", MatchPolicy: matchPolicyPtr(arv1.Equivalent)}},
			expectedResult: true,
		},
		{
			name:       "desired failure policy is nil",
			currentVWs: []arv1.MutatingWebhook{{Name: "a", FailurePolicy: failurePolicyPtr(arv1.Fail)}},
			desiredVWs: []arv1.MutatingWebhook{{Name: "a"}},
		},
		{
			name:           "current failure policy is nil",
			currentVWs:     []arv1.MutatingWebhook{{Name: "a"}},
			desiredVWs:     []arv1.MutatingWebhook{{Name: "a", FailurePolicy: failurePolicyPtr(arv1.Fail)}},
			expectedResult: true,
		},
		{
			name:           "current and desired failure policy differ",
			currentVWs:     []arv1.MutatingWebhook{{Name: "a", FailurePolicy: failurePolicyPtr(arv1.Ignore)}},
			desiredVWs:     []arv1.MutatingWebhook{{Name: "a", FailurePolicy: failurePolicyPtr(arv1.Fail)}},
			expectedResult: true,
		},
		{
			name:       "rules have changed",
			currentVWs: []arv1.MutatingWebhook{{Name: "a", Rules: []arv1.RuleWithOperations{}}},
			desiredVWs: []arv1.MutatingWebhook{{Name: "a", Rules: []arv1.RuleWithOperations{
				{
					Operations: []arv1.OperationType{arv1.Create},
					Rule: arv1.Rule{
						APIGroups:   []string{"apps"},
						APIVersions: []string{"v1"},
						Resources:   []string{"deployments"},
						Scope:       scopeTypePtr(arv1.AllScopes),
					},
				},
			}}},
			expectedResult: true,
		},
		{
			name: "rules are the same",
			currentVWs: []arv1.MutatingWebhook{{Name: "a", Rules: []arv1.RuleWithOperations{
				{
					Operations: []arv1.OperationType{arv1.Create},
					Rule: arv1.Rule{
						APIGroups:   []string{"apps"},
						APIVersions: []string{"v1"},
						Resources:   []string{"deployments"},
						Scope:       scopeTypePtr(arv1.AllScopes),
					},
				},
			}}},
			desiredVWs: []arv1.MutatingWebhook{{Name: "a", Rules: []arv1.RuleWithOperations{
				{
					Operations: []arv1.OperationType{arv1.Create},
					Rule: arv1.Rule{
						APIGroups:   []string{"apps"},
						APIVersions: []string{"v1"},
						Resources:   []string{"deployments"},
						Scope:       scopeTypePtr(arv1.AllScopes),
					},
				},
			}}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := haveMutatingWebhooksChanged(tc.currentVWs, tc.desiredVWs)
			if result != tc.expectedResult {
				t.Errorf("expected result %v, got %v", tc.expectedResult, result)
			}
		})
	}
}

func testValidatingWebhooks(serviceName, serviceNamespace string) []arv1.ValidatingWebhook {
	return []arv1.ValidatingWebhook{
		{
			Name: "vtargetgroupbinding.elbv2.k8s.aws",
			ClientConfig: arv1.WebhookClientConfig{
				Service: &arv1.ServiceReference{
					Namespace: serviceNamespace,
					Name:      serviceName,
					Path:      pointer.StringPtr("/validate-elbv2-k8s-aws-v1beta1-targetgroupbinding"),
					Port:      pointer.Int32Ptr(controllerWebhookPort),
				},
			},
			Rules: []arv1.RuleWithOperations{
				{
					Operations: []arv1.OperationType{
						arv1.Create,
						arv1.Update,
					},
					Rule: arv1.Rule{
						APIGroups:   []string{"elbv2.k8s.aws"},
						APIVersions: []string{"v1beta1"},
						Resources:   []string{"targetgroupbindings"},
						Scope:       scopeTypePtr(arv1.AllScopes),
					},
				},
			},
			FailurePolicy:           failurePolicyPtr(arv1.Fail),
			MatchPolicy:             matchPolicyPtr(arv1.Equivalent),
			SideEffects:             sideEffectPtr(arv1.SideEffectClassNone),
			AdmissionReviewVersions: []string{"v1beta1"},
		},
		{
			Name: "vingress.elbv2.k8s.aws",
			ClientConfig: arv1.WebhookClientConfig{
				Service: &arv1.ServiceReference{
					Namespace: serviceNamespace,
					Name:      serviceName,
					Path:      pointer.StringPtr("/validate-networking-v1-ingress"),
					Port:      pointer.Int32Ptr(controllerWebhookPort),
				},
			},
			Rules: []arv1.RuleWithOperations{
				{
					Operations: []arv1.OperationType{
						arv1.Create,
						arv1.Update,
					},
					Rule: arv1.Rule{
						APIGroups:   []string{"networking.k8s.io"},
						APIVersions: []string{"v1"},
						Resources:   []string{"ingresses"},
						Scope:       scopeTypePtr(arv1.AllScopes),
					},
				},
			},
			FailurePolicy:           failurePolicyPtr(arv1.Fail),
			MatchPolicy:             matchPolicyPtr(arv1.Equivalent),
			SideEffects:             sideEffectPtr(arv1.SideEffectClassNone),
			AdmissionReviewVersions: []string{"v1beta1"},
		},
	}
}

func testMutatingWebhooks(serviceName, serviceNamespace string) []arv1.MutatingWebhook {
	return []arv1.MutatingWebhook{
		{
			AdmissionReviewVersions: []string{"v1beta1"},
			ClientConfig: arv1.WebhookClientConfig{
				Service: &arv1.ServiceReference{Name: serviceName,
					Namespace: serviceNamespace,
					Path:      pointer.StringPtr("/mutate-elbv2-k8s-aws-v1beta1-targetgroupbinding"),
					Port:      pointer.Int32Ptr(controllerWebhookPort),
				},
			},
			FailurePolicy: failurePolicyPtr(arv1.Fail),
			Name:          "mtargetgroupbinding.elbv2.k8s.aws",
			Rules: []arv1.RuleWithOperations{
				{
					Rule: arv1.Rule{
						APIGroups:   []string{"elbv2.k8s.aws"},
						APIVersions: []string{"v1beta1"},
						Resources:   []string{"targetgroupbindings"},
						Scope:       scopeTypePtr(arv1.AllScopes),
					},
					Operations: []arv1.OperationType{
						arv1.Create,
						arv1.Update,
					},
				},
			},
			SideEffects: sideEffectPtr(arv1.SideEffectClassNone),
		},
	}
}

func TestEnsureWebhooks(t *testing.T) {
	for _, tc := range []struct {
		name            string
		controller      *albo.AWSLoadBalancerController
		webhookService  *corev1.Service
		expectedVWC     *arv1.ValidatingWebhookConfiguration
		expectedMWC     *arv1.MutatingWebhookConfiguration
		existingObjects []client.Object
	}{
		{
			name:           "no existing webhooks",
			controller:     &albo.AWSLoadBalancerController{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}},
			webhookService: &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "test-service", Namespace: "test-namespace"}},
			expectedVWC: &arv1.ValidatingWebhookConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "aws-load-balancer-controller-cluster",
					Annotations: map[string]string{injectCABundleAnnotationKey: injectCABundleAnnotationValue},
				},
				Webhooks: testValidatingWebhooks("test-service", "test-namespace"),
			},
			expectedMWC: &arv1.MutatingWebhookConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "aws-load-balancer-controller-cluster",
					Annotations: map[string]string{injectCABundleAnnotationKey: injectCABundleAnnotationValue},
				},
				Webhooks: testMutatingWebhooks("test-service", "test-namespace"),
			},
		},
		{
			name:       "existing validating webhook",
			controller: &albo.AWSLoadBalancerController{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}},
			existingObjects: []client.Object{
				&arv1.ValidatingWebhookConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name: "aws-load-balancer-controller-cluster",
						OwnerReferences: []metav1.OwnerReference{
							{Name: "cluster", Kind: "AWSLoadBalancerController"},
						},
					},
					Webhooks: []arv1.ValidatingWebhook{
						{Name: "ingress"},
					},
				},
			},
			webhookService: &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "test-service", Namespace: "test-namespace"}},
			expectedVWC: &arv1.ValidatingWebhookConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "aws-load-balancer-controller-cluster",
					Annotations: map[string]string{injectCABundleAnnotationKey: injectCABundleAnnotationValue},
				},
				Webhooks: testValidatingWebhooks("test-service", "test-namespace"),
			},
			expectedMWC: &arv1.MutatingWebhookConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "aws-load-balancer-controller-cluster",
					Annotations: map[string]string{injectCABundleAnnotationKey: injectCABundleAnnotationValue},
				},
				Webhooks: testMutatingWebhooks("test-service", "test-namespace"),
			},
		},
		{
			name:       "existing mutating webhook",
			controller: &albo.AWSLoadBalancerController{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}},
			existingObjects: []client.Object{
				&arv1.MutatingWebhookConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name: "aws-load-balancer-controller-cluster",
						OwnerReferences: []metav1.OwnerReference{
							{Name: "cluster", Kind: "AWSLoadBalancerController"},
						},
					},
					Webhooks: []arv1.MutatingWebhook{
						{Name: "ingress"},
					},
				},
			},
			webhookService: &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "test-service", Namespace: "test-namespace"}},
			expectedVWC: &arv1.ValidatingWebhookConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "aws-load-balancer-controller-cluster",
					Annotations: map[string]string{injectCABundleAnnotationKey: injectCABundleAnnotationValue},
				},
				Webhooks: testValidatingWebhooks("test-service", "test-namespace"),
			},
			expectedMWC: &arv1.MutatingWebhookConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "aws-load-balancer-controller-cluster",
					Annotations: map[string]string{injectCABundleAnnotationKey: injectCABundleAnnotationValue},
				},
				Webhooks: testMutatingWebhooks("test-service", "test-namespace"),
			},
		},
		{
			name:       "existing webhooks with third-party annotations",
			controller: &albo.AWSLoadBalancerController{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}},
			existingObjects: []client.Object{
				&arv1.MutatingWebhookConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name: "aws-load-balancer-controller-cluster",
						Annotations: map[string]string{
							"test-key": "test-value",
						},
						OwnerReferences: []metav1.OwnerReference{
							{Name: "cluster", Kind: "AWSLoadBalancerController"},
						},
					},
					Webhooks: []arv1.MutatingWebhook{
						{Name: "ingress"},
					},
				},
				&arv1.ValidatingWebhookConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "aws-load-balancer-controller-cluster",
						Annotations: map[string]string{"test-key": "test-value"},
						OwnerReferences: []metav1.OwnerReference{
							{Name: "cluster", Kind: "AWSLoadBalancerController"},
						},
					},
					Webhooks: []arv1.ValidatingWebhook{
						{Name: "ingress"},
					},
				},
			},
			webhookService: &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "test-service", Namespace: "test-namespace"}},
			expectedVWC: &arv1.ValidatingWebhookConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name: "aws-load-balancer-controller-cluster",
					Annotations: map[string]string{
						injectCABundleAnnotationKey: injectCABundleAnnotationValue,
						"test-key":                  "test-value",
					},
				},
				Webhooks: testValidatingWebhooks("test-service", "test-namespace"),
			},
			expectedMWC: &arv1.MutatingWebhookConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name: "aws-load-balancer-controller-cluster",
					Annotations: map[string]string{
						injectCABundleAnnotationKey: injectCABundleAnnotationValue,
						"test-key":                  "test-value",
					},
				},
				Webhooks: testMutatingWebhooks("test-service", "test-namespace"),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			testClient := fake.NewClientBuilder().WithObjects(tc.existingObjects...).WithScheme(test.Scheme).Build()
			r := &AWSLoadBalancerControllerReconciler{
				Client: testClient,
				Scheme: test.Scheme,
			}
			err := r.ensureWebhooks(ctx, tc.controller, tc.webhookService)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			var (
				vwc arv1.ValidatingWebhookConfiguration
				mwc arv1.MutatingWebhookConfiguration
			)

			err = r.Get(ctx, types.NamespacedName{Name: tc.expectedVWC.Name}, &vwc)
			if err != nil {
				t.Errorf("failed to get expected validating webhook configuration: %v", err)
			}

			if diff := cmp.Diff(tc.expectedVWC.Annotations, vwc.Annotations); diff != "" {
				t.Errorf("mismatched annotations:\n%s", diff)
			}

			if !hasOwner(tc.controller, vwc.OwnerReferences) {
				t.Errorf("expected owner reference on controller")
			}

			if !cmp.Equal(tc.expectedVWC.Webhooks, vwc.Webhooks) {
				t.Errorf("unexpected values in validating webhook:\n%s", cmp.Diff(tc.expectedVWC.Webhooks, vwc.Webhooks))
			}

			err = r.Get(ctx, types.NamespacedName{Name: tc.expectedVWC.Name}, &mwc)
			if err != nil {
				t.Errorf("failed to get expected mutating webhook configuration: %v", err)
			}
			if !hasOwner(tc.controller, mwc.OwnerReferences) {
				t.Errorf("expected owner reference to controller on mutating webhook configuration")
			}

			if diff := cmp.Diff(tc.expectedMWC.Annotations, mwc.Annotations); diff != "" {
				t.Errorf("mismatched annotations:\n%s", diff)
			}

			if !cmp.Equal(tc.expectedMWC.Webhooks, mwc.Webhooks) {
				t.Errorf("unexpected values in mutating webhook:\n%s", cmp.Diff(tc.expectedMWC.Webhooks, mwc.Webhooks))
			}
		})
	}
}

func hasOwner(controller *albo.AWSLoadBalancerController, references []metav1.OwnerReference) bool {
	for _, o := range references {
		if o.Name == controller.Name && o.Kind == "AWSLoadBalancerController" {
			return true
		}
	}
	return false
}
