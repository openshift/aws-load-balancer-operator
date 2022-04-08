package awsloadbalancercontroller

import (
	"context"
	"fmt"
	"sort"
	"strings"

	arv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
)

const (
	injectCABundleAnnotationKey   = "service.beta.openshift.io/inject-cabundle"
	injectCABundleAnnotationValue = "true"
)

// ensureWebhooks ensures that the ValidatingWebhookConfiguration and MutatingWebhookConfiguration resources associated with the controller
// are created and up-to-date.
func (r *AWSLoadBalancerControllerReconciler) ensureWebhooks(ctx context.Context, controller *albo.AWSLoadBalancerController, service *corev1.Service) error {
	reqLogger := log.FromContext(ctx).WithValues("webhook", controller.Name)
	reqLogger.Info("ensuring validating and mutating webhook configurations for aws-load-balancer-controller instance")

	desiredVWC := desiredValidatingWebhookConfiguration(controller, service)
	err := controllerutil.SetControllerReference(controller, desiredVWC, r.Scheme)
	if err != nil {
		return fmt.Errorf("failed to set owner reference on desired ValidatingWebhookConfiguration %q: %w", desiredVWC.Name, err)
	}

	currentVWC, exists, err := r.currentValidatingWebhookConfiguration(ctx, desiredVWC.Name)
	if err != nil {
		return fmt.Errorf("failed to get current ValidatingWebhookConfiguration %q: %w", desiredVWC.Name, err)
	}
	if !exists {
		reqLogger.Info("creating validating webhook configuration")
		err = r.Create(ctx, desiredVWC)
		if err != nil {
			return fmt.Errorf("failed to create ValidatingWebhookConfiguration %q: %w", desiredVWC.Name, err)
		}
	} else {
		reqLogger.Info("updating validating webhook configuration")
		err = r.updateValidatingWebhookConfiguration(ctx, currentVWC, desiredVWC)
		if err != nil {
			return fmt.Errorf("failed to updated ValidatingWebhookConfiguration %q: %w", currentVWC.Name, err)
		}
	}

	desiredMWC := desiredMutatingWebhookConfiguration(controller, service)
	err = controllerutil.SetControllerReference(controller, desiredMWC, r.Scheme)
	if err != nil {
		return fmt.Errorf("failed to set owner reference on desired MutatingWebhookConfiguration %q: %w", desiredMWC.Name, err)
	}

	currentMWC, exists, err := r.currentMutatingWebhookConfiguration(ctx, desiredMWC.Name)
	if err != nil {
		return fmt.Errorf("failed to get current MutatingWebhookConfiguration %q: %w", desiredMWC.Name, err)
	}
	if !exists {
		reqLogger.Info("creating mutating webhook configuration")
		err := r.Create(ctx, desiredMWC)
		if err != nil {
			return fmt.Errorf("failed to create MutatingWebhookConfiguration %q: %w", desiredMWC.Name, err)
		}
		return nil
	}

	reqLogger.Info("updating mutating webhook configuration")
	err = r.updateMutatingWebhookConfiguration(ctx, currentMWC, desiredMWC)
	if err != nil {
		return fmt.Errorf("failed to updated ValidatingWebhookConfiguration %q: %w", currentVWC.Name, err)
	}
	return nil
}

func (r *AWSLoadBalancerControllerReconciler) currentValidatingWebhookConfiguration(ctx context.Context, name string) (*arv1.ValidatingWebhookConfiguration, bool, error) {
	var currentVWC arv1.ValidatingWebhookConfiguration
	err := r.Get(ctx, types.NamespacedName{Name: name}, &currentVWC)
	if err != nil && errors.IsNotFound(err) {
		return nil, false, nil
	}
	return &currentVWC, true, err
}

func (r *AWSLoadBalancerControllerReconciler) currentMutatingWebhookConfiguration(ctx context.Context, name string) (*arv1.MutatingWebhookConfiguration, bool, error) {
	var currentMWC arv1.MutatingWebhookConfiguration
	err := r.Get(ctx, types.NamespacedName{Name: name}, &currentMWC)
	if err != nil && errors.IsNotFound(err) {
		return nil, false, nil
	}
	return &currentMWC, true, err
}

func desiredValidatingWebhookConfiguration(controller *albo.AWSLoadBalancerController, webhookService *corev1.Service) *arv1.ValidatingWebhookConfiguration {
	return &arv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s", controllerResourcePrefix, controller.Name),
			Annotations: map[string]string{
				injectCABundleAnnotationKey: injectCABundleAnnotationValue,
			},
		},
		Webhooks: []arv1.ValidatingWebhook{
			{
				Name: "vtargetgroupbinding.elbv2.k8s.aws",
				ClientConfig: arv1.WebhookClientConfig{
					Service: &arv1.ServiceReference{
						Namespace: webhookService.Namespace,
						Name:      webhookService.Name,
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
						Namespace: webhookService.Namespace,
						Name:      webhookService.Name,
						Path:      pointer.StringPtr("/validate-networking-v1-ingress"),
						Port:      pointer.Int32(controllerWebhookPort),
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
						},
					},
				},
				FailurePolicy:           failurePolicyPtr(arv1.Fail),
				MatchPolicy:             matchPolicyPtr(arv1.Equivalent),
				SideEffects:             sideEffectPtr(arv1.SideEffectClassNone),
				AdmissionReviewVersions: []string{"v1beta1"},
			},
		},
	}
}

func sideEffectPtr(sideEffectClass arv1.SideEffectClass) *arv1.SideEffectClass {
	return &sideEffectClass
}

func matchPolicyPtr(matchPolicyType arv1.MatchPolicyType) *arv1.MatchPolicyType {
	return &matchPolicyType
}

func failurePolicyPtr(failurePolicyType arv1.FailurePolicyType) *arv1.FailurePolicyType {
	return &failurePolicyType
}

func (r *AWSLoadBalancerControllerReconciler) updateValidatingWebhookConfiguration(ctx context.Context, current, desired *arv1.ValidatingWebhookConfiguration) error {
	updatedVWC := current.DeepCopy()
	var updated bool

	updatedVWC.Annotations, updated = updateAnnotations(updatedVWC.Annotations, desired.Annotations)

	if haveValidatingWebhooksChanged(updatedVWC.Webhooks, desired.Webhooks) {
		updatedVWC.Webhooks = desired.Webhooks
		updated = true
	}

	if updated {
		return r.Update(ctx, updatedVWC)
	}
	return nil
}

// updateAnnotations will return an updated map of annotations and indicate if an update actually occurred
func updateAnnotations(current map[string]string, desired map[string]string) (map[string]string, bool) {
	annotations := make(map[string]string)
	for k, v := range current {
		annotations[k] = v
	}
	var updated bool
	for desiredKey, desiredValue := range desired {
		if currentValue, ok := annotations[desiredKey]; !ok || currentValue != desiredValue {
			annotations[desiredKey] = desiredValue
			updated = true
		}
	}
	return annotations, updated
}

type sortableValidatingWebhooks []arv1.ValidatingWebhook

func (s sortableValidatingWebhooks) Len() int {
	return len(s)
}

func (s sortableValidatingWebhooks) Less(i, j int) bool {
	return strings.Compare(s[i].Name, s[j].Name) < 0
}

func (s sortableValidatingWebhooks) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// haveValidatingWebhooksChanged indicates if the current ValidatingWebhooks are up-to-date. It sorts the webhooks in the
// desired and current configurations and compares each of the fields. Some fields are ignored if they are set in the
// current configuration if the desired is nil.
func haveValidatingWebhooksChanged(updated sortableValidatingWebhooks, desired sortableValidatingWebhooks) bool {
	if len(updated) != len(desired) {
		return true
	}
	updatedCopy := make(sortableValidatingWebhooks, len(updated))
	desiredCopy := make(sortableValidatingWebhooks, len(desired))

	copy(updatedCopy, updated)
	copy(desiredCopy, desired)
	sort.Sort(updatedCopy)
	sort.Sort(desiredCopy)

	for i := 0; i < len(updatedCopy); i++ {
		u := updatedCopy[i]
		d := desiredCopy[i]
		if u.Name != d.Name {
			return true
		}
		if !equality.Semantic.DeepEqual(u.ClientConfig.Service, d.ClientConfig.Service) {
			return true
		}
		if !equality.Semantic.DeepEqual(u.Rules, d.Rules) {
			return true
		}
		if d.FailurePolicy != nil {
			if u.FailurePolicy == nil {
				return true
			}
			if *u.FailurePolicy != *d.FailurePolicy {
				return true
			}
		}
		if d.MatchPolicy != nil {
			if u.MatchPolicy == nil {
				return true
			}
			if *u.MatchPolicy != *d.MatchPolicy {
				return true
			}
		}
		if d.SideEffects != nil {
			if u.SideEffects == nil {
				return true
			}
			if *u.SideEffects != *d.SideEffects {
				return true
			}
		}
		if !equality.Semantic.DeepEqual(u.AdmissionReviewVersions, d.AdmissionReviewVersions) {
			return true
		}
	}
	return false
}

func desiredMutatingWebhookConfiguration(controller *albo.AWSLoadBalancerController, webhookService *corev1.Service) *arv1.MutatingWebhookConfiguration {
	return &arv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s", controllerResourcePrefix, controller.Name),
			Annotations: map[string]string{
				injectCABundleAnnotationKey: injectCABundleAnnotationValue,
			},
		},
		Webhooks: []arv1.MutatingWebhook{
			{
				AdmissionReviewVersions: []string{"v1beta1"},
				ClientConfig: arv1.WebhookClientConfig{
					Service: &arv1.ServiceReference{Name: webhookService.Name,
						Namespace: webhookService.Namespace,
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
						},
						Operations: []arv1.OperationType{
							arv1.Create,
							arv1.Update,
						},
					},
				},
				SideEffects: sideEffectPtr(arv1.SideEffectClassNone),
			},
		},
	}
}

func (r *AWSLoadBalancerControllerReconciler) updateMutatingWebhookConfiguration(ctx context.Context, current, desired *arv1.MutatingWebhookConfiguration) error {
	updatedMWC := current.DeepCopy()
	var updated bool

	updatedMWC.Annotations, updated = updateAnnotations(updatedMWC.Annotations, desired.Annotations)

	if haveMutatingWebhooksChanged(updatedMWC.Webhooks, desired.Webhooks) {
		updated = true
		updatedMWC.Webhooks = desired.Webhooks
	}
	if updated {
		return r.Update(ctx, updatedMWC)
	}
	return nil
}

type sortableMutatingWebhooks []arv1.MutatingWebhook

func (s sortableMutatingWebhooks) Len() int {
	return len(s)
}

func (s sortableMutatingWebhooks) Less(i, j int) bool {
	return strings.Compare(s[i].Name, s[j].Name) < 0
}

func (s sortableMutatingWebhooks) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// haveMutatingWebhooksChanged indicates if the current MutatingWebhooks are up-to-date. It sorts the webhooks in the
// desired and current configurations and compares each of the fields. Some fields are ignored if they are set in the
// current configuration if the desired is nil.
func haveMutatingWebhooksChanged(updated, desired sortableMutatingWebhooks) bool {
	if len(updated) != len(desired) {
		return true
	}
	updatedCopy := make(sortableMutatingWebhooks, len(updated))
	desiredCopy := make(sortableMutatingWebhooks, len(desired))
	copy(updatedCopy, updated)
	copy(desiredCopy, desired)
	sort.Sort(updatedCopy)
	sort.Sort(desiredCopy)

	for i := 0; i < len(updatedCopy); i++ {
		u := updatedCopy[i]
		d := desiredCopy[i]
		if u.Name != d.Name {
			return true
		}
		if !equality.Semantic.DeepEqual(u.ClientConfig.Service, d.ClientConfig.Service) {
			return true
		}
		if !equality.Semantic.DeepEqual(u.Rules, d.Rules) {
			return true
		}
		if d.FailurePolicy != nil {
			if u.FailurePolicy == nil {
				return true
			}
			if *u.FailurePolicy != *d.FailurePolicy {
				return true
			}
		}
		if d.MatchPolicy != nil {
			if u.MatchPolicy == nil {
				return true
			}
			if *u.MatchPolicy != *d.MatchPolicy {
				return true
			}
		}
		if d.SideEffects != nil {
			if u.SideEffects == nil {
				return true
			}
			if *u.SideEffects != *d.SideEffects {
				return true
			}
		}
		if !equality.Semantic.DeepEqual(u.AdmissionReviewVersions, d.AdmissionReviewVersions) {
			return true
		}
	}
	return false
}
