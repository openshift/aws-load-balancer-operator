//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"net/http"
	"reflect"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"

	configv1 "github.com/openshift/api/config/v1"

	"github.com/aws/aws-sdk-go-v2/service/wafv2"
	wafv2types "github.com/aws/aws-sdk-go-v2/service/wafv2/types"

	"github.com/aws/aws-sdk-go-v2/aws"
	"sigs.k8s.io/controller-runtime/pkg/client"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1"
	albov1alpha1 "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
)

var (
	allCapabilities          = "ALL"
	privileged               = false
	runAsNonRoot             = true
	allowPrivilegeEscalation = false
)

func waitForDeploymentStatusCondition(ctx context.Context, t *testing.T, cl client.Client, timeout time.Duration, deploymentName types.NamespacedName, conditions ...appsv1.DeploymentCondition) error {
	t.Helper()

	return wait.PollUntilContextTimeout(ctx, 10*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		dep := &appsv1.Deployment{}
		if err := cl.Get(ctx, deploymentName, dep); err != nil {
			t.Logf("failed to get deployment %s: %v (retrying)", deploymentName.Name, err)
			return false, nil
		}

		expected := deploymentConditionMap(conditions...)
		current := deploymentConditionMap(dep.Status.Conditions...)
		return conditionsMatchExpected(expected, current), nil
	})
}

// waitForPodPhases waits until one of the given phases is set on the given pod.
// Returns the found phase or error in case of timeout.
func waitForPodPhases(ctx context.Context, t *testing.T, cl client.Client, timeout time.Duration, podName types.NamespacedName, phases ...corev1.PodPhase) (corev1.PodPhase, error) {
	t.Helper()
	var foundPhase corev1.PodPhase
	err := wait.PollUntilContextTimeout(ctx, 10*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		pod := &corev1.Pod{}
		if err := cl.Get(ctx, podName, pod); err != nil {
			t.Logf("failed to get pod %s: %v (retrying)", podName.Name, err)
			return false, nil
		}

		for _, p := range phases {
			if pod.Status.Phase == p {
				foundPhase = p
				return true, nil
			}
		}
		return false, nil
	})
	return foundPhase, err
}

func getIngress(ctx context.Context, t *testing.T, cl client.Client, timeout time.Duration, ingressName types.NamespacedName) (string, error) {
	t.Helper()
	var address string
	return address, wait.PollUntilContextTimeout(ctx, 10*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		ing := &networkingv1.Ingress{}
		if err := cl.Get(ctx, ingressName, ing); err != nil {
			t.Logf("failed to get ingress %s: %v (retrying)", ingressName.Name, err)
			return false, nil
		}
		if len(ing.Status.LoadBalancer.Ingress) <= 0 || len(ing.Status.LoadBalancer.Ingress[0].Hostname) <= 0 {
			return false, nil
		}
		address = ing.Status.LoadBalancer.Ingress[0].Hostname
		return true, nil
	})
}

func getService(ctx context.Context, t *testing.T, cl client.Client, timeout time.Duration, svcName types.NamespacedName) (string, error) {
	t.Helper()
	var address string
	return address, wait.PollUntilContextTimeout(ctx, 10*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		svc := &corev1.Service{}
		if err := cl.Get(ctx, svcName, svc); err != nil {
			t.Logf("failed to get service %s: %v (retrying)", svcName.Name, err)
			return false, nil
		}
		if len(svc.Status.LoadBalancer.Ingress) <= 0 || len(svc.Status.LoadBalancer.Ingress[0].Hostname) <= 0 {
			return false, nil
		}
		address = svc.Status.LoadBalancer.Ingress[0].Hostname
		return true, nil
	})
}

func waitForDeletion(ctx context.Context, t *testing.T, cl client.Client, obj client.Object, timeout time.Duration) {
	t.Helper()
	deletionPolicy := v1.DeletePropagationForeground
	_ = wait.PollUntilContextTimeout(ctx, 10*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		err := cl.Delete(ctx, obj, &client.DeleteOptions{PropagationPolicy: &deletionPolicy})
		if err != nil && !errors.IsNotFound(err) {
			t.Logf("failed to delete resource %s/%s: %v", obj.GetName(), obj.GetNamespace(), err)
			return false, nil
		} else if err == nil {
			t.Logf("retrying deletion of resource %q/%q", obj.GetName(), obj.GetNamespace())
			return false, nil
		}
		t.Logf("deleted resource %s/%s", obj.GetName(), obj.GetNamespace())
		return true, nil
	})
}

func deploymentConditionMap(conditions ...appsv1.DeploymentCondition) map[string]string {
	conds := map[string]string{}
	for _, cond := range conditions {
		conds[string(cond.Type)] = string(cond.Status)
	}
	return conds
}

func conditionsMatchExpected(expected, actual map[string]string) bool {
	filtered := map[string]string{}
	for k := range actual {
		if _, comparable := expected[k]; comparable {
			filtered[k] = actual[k]
		}
	}
	return reflect.DeepEqual(expected, filtered)
}

// buildEchoPod returns a pod definition for an socat-based echo server.
func buildEchoPod(name, namespace string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"app": "echo",
			},
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					// Note that HTTP/1.0 will strip the HSTS response header
					Args: []string{
						"TCP4-LISTEN:8080,reuseaddr,fork",
						`EXEC:'/bin/bash -c \"printf \\\"HTTP/1.0 200 OK\r\n\r\n\\\"; sed -e \\\"/^\r/q\\\"\"'`,
					},
					Command: []string{"/bin/socat"},
					Image:   "openshift/origin-node",
					Name:    "echo",
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: int32(8080),
							Protocol:      corev1.ProtocolTCP,
						},
					},
					SecurityContext: &corev1.SecurityContext{
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{corev1.Capability(allCapabilities)},
						},
						Privileged:               &privileged,
						RunAsNonRoot:             &runAsNonRoot,
						AllowPrivilegeEscalation: &allowPrivilegeEscalation,
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
				},
			},
		},
	}
}

func waitForHTTPClientCondition(ctx context.Context, t *testing.T, httpClient *http.Client, req *http.Request, interval, timeout time.Duration, compareFunc func(*http.Response) bool) error {
	t.Helper()
	return wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (done bool, err error) {
		resp, err := httpClient.Do(req)
		if err == nil {
			return compareFunc(resp), nil
		} else {
			t.Logf("retrying client call due to: %+v", err)
			return false, nil
		}
	})
}

// buildCurlPod returns a pod definition for a pod with the given name and image
// and in the given namespace that curls the specified host and address.
func buildCurlPod(name, namespace, host, address string, extraArgs ...string) *corev1.Pod {
	curlArgs := []string{
		"-s",
		"-v",
		"-f",
		"--retry", "300", "--retry-delay", "5", "--max-time", "2",
	}

	if len(host) != 0 {
		curlArgs = append(curlArgs, "--header", "HOST:"+host)
	}

	curlArgs = append(curlArgs, extraArgs...)
	curlArgs = append(curlArgs, "http://"+address)
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "curl",
					Image:   "openshift/origin-node",
					Command: []string{"/bin/curl"},
					Args:    curlArgs,
					SecurityContext: &corev1.SecurityContext{
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{corev1.Capability(allCapabilities)},
						},
						Privileged:               &privileged,
						RunAsNonRoot:             &runAsNonRoot,
						AllowPrivilegeEscalation: &allowPrivilegeEscalation,
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}
}

// buildEchoService returns a service definition for an HTTP service.
func buildEchoService(name, namespace string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       int32(80),
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(8080),
				},
			},
			Selector: map[string]string{
				"app": "echo",
			},
		},
	}
}

func buildIngressRule(host string, path *networkingv1.HTTPIngressRuleValue) networkingv1.IngressRule {
	return networkingv1.IngressRule{
		Host: host,
		IngressRuleValue: networkingv1.IngressRuleValue{
			HTTP: path,
		},
	}
}

func buildIngressPath(path string, pathtype networkingv1.PathType, backend networkingv1.IngressBackend) networkingv1.HTTPIngressPath {
	return networkingv1.HTTPIngressPath{
		Path:     path,
		PathType: &pathtype,
		Backend:  backend,
	}
}

func buildIngressBackend(svc *corev1.Service) networkingv1.IngressBackend {
	return networkingv1.IngressBackend{
		Service: &networkingv1.IngressServiceBackend{
			Name: svc.Name,
			Port: networkingv1.ServiceBackendPort{
				Number: svc.Spec.Ports[0].Port,
			},
		},
	}
}

type ingressbuilder struct {
	name         types.NamespacedName
	annotations  map[string]string
	ingressclass string
	rules        []networkingv1.IngressRule
}

func newIngressBuilder() *ingressbuilder {
	return &ingressbuilder{
		name:         types.NamespacedName{Name: "sample", Namespace: "aws-load-balancer-operator"},
		annotations:  make(map[string]string),
		ingressclass: "alb",
		rules:        []networkingv1.IngressRule{},
	}
}

func (b *ingressbuilder) withName(name types.NamespacedName) *ingressbuilder {
	b.name = name
	return b
}

func (b *ingressbuilder) withAnnotations(annotations map[string]string) *ingressbuilder {
	b.annotations = annotations
	return b
}

func (b *ingressbuilder) withIngressClass(class string) *ingressbuilder {
	b.ingressclass = class
	return b
}

func (b *ingressbuilder) withRules(rules []networkingv1.IngressRule) *ingressbuilder {
	b.rules = rules
	return b
}

func (b ingressbuilder) build() *networkingv1.Ingress {
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        b.name.Name,
			Namespace:   b.name.Namespace,
			Annotations: b.annotations,
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: aws.String(b.ingressclass),
			Rules:            b.rules,
		},
	}
}

func buildEchoIngress(name types.NamespacedName, ingClass string, annotations map[string]string, backendSvc *corev1.Service) *networkingv1.Ingress {
	return newIngressBuilder().
		withName(name).
		withAnnotations(annotations).
		withIngressClass(ingClass).
		withRules([]networkingv1.IngressRule{
			buildIngressRule("echoserver.example.com", &networkingv1.HTTPIngressRuleValue{
				Paths: []networkingv1.HTTPIngressPath{
					buildIngressPath("/", networkingv1.PathTypeExact, buildIngressBackend(backendSvc)),
				},
			}),
		}).
		build()
}

// albcBuilder is a helper to build ALBC CRs.
type albcBuilder struct {
	name         string
	tagPolicy    albo.SubnetTaggingPolicy
	ingressClass string
	addons       []albo.AWSAddon
	credentials  *configv1.SecretNameReference
	tags         map[string]string
}

func newALBCBuilder() *albcBuilder {
	return &albcBuilder{
		name:         "cluster",
		tagPolicy:    albo.AutoSubnetTaggingPolicy,
		ingressClass: "alb",
		addons:       []albo.AWSAddon{},
	}
}

func (b *albcBuilder) withIngressClass(class string) *albcBuilder {
	b.ingressClass = class
	return b
}

func (b *albcBuilder) withAddons(addons ...albo.AWSAddon) *albcBuilder {
	b.addons = addons
	return b
}

func (b *albcBuilder) withCredSecret(name string) *albcBuilder {
	b.credentials = &configv1.SecretNameReference{Name: name}
	return b
}

// withCredSecretIf adds the credentials secret only if the given condition is true.
func (b *albcBuilder) withCredSecretIf(condition bool, name string) *albcBuilder {
	if condition {
		b.credentials = &configv1.SecretNameReference{Name: name}
	}
	return b
}

func (b *albcBuilder) withResourceTags(tags map[string]string) *albcBuilder {
	b.tags = tags
	return b
}

func (b *albcBuilder) buildv1alpha1() *albov1alpha1.AWSLoadBalancerController {
	albc := &albov1alpha1.AWSLoadBalancerController{
		ObjectMeta: v1.ObjectMeta{
			Name: b.name,
		},
		Spec: albov1alpha1.AWSLoadBalancerControllerSpec{
			SubnetTagging:          albov1alpha1.SubnetTaggingPolicy(b.tagPolicy),
			IngressClass:           b.ingressClass,
			Credentials:            (*albov1alpha1.SecretReference)(b.credentials),
			AdditionalResourceTags: b.tags,
		},
	}
	for _, a := range b.addons {
		albc.Spec.EnabledAddons = append(albc.Spec.EnabledAddons, albov1alpha1.AWSAddon(a))
	}
	return albc
}

func (b *albcBuilder) build() *albo.AWSLoadBalancerController {
	albc := &albo.AWSLoadBalancerController{
		ObjectMeta: v1.ObjectMeta{
			Name: b.name,
		},
		Spec: albo.AWSLoadBalancerControllerSpec{
			SubnetTagging: b.tagPolicy,
			IngressClass:  b.ingressClass,
			EnabledAddons: b.addons,
			Credentials:   b.credentials,
		},
	}
	for k, v := range b.tags {
		albc.Spec.AdditionalResourceTags = append(albc.Spec.AdditionalResourceTags, albo.AWSResourceTag{Key: k, Value: v})
	}
	return albc
}

func buildIngressClass(name types.NamespacedName, controller string) *networkingv1.IngressClass {
	return &networkingv1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
		},
		Spec: networkingv1.IngressClassSpec{
			Controller: controller,
		},
	}
}

func findAWSWebACL(wafClient *wafv2.Client, aclName string) (*wafv2types.WebACLSummary, error) {
	return findAWSWebACLRecursive(wafClient, aclName, nil)
}

func findAWSWebACLRecursive(wafClient *wafv2.Client, aclName string, nextMarker *string) (*wafv2types.WebACLSummary, error) {
	output, err := wafClient.ListWebACLs(context.Background(), &wafv2.ListWebACLsInput{
		Scope:      wafv2types.ScopeRegional,
		NextMarker: nextMarker,
	})
	if err != nil {
		return nil, err
	}
	for i, acl := range output.WebACLs {
		if acl.Name != nil && *acl.Name == aclName {
			return &output.WebACLs[i], nil
		}
	}
	if output.NextMarker != nil {
		return findAWSWebACLRecursive(wafClient, aclName, output.NextMarker)
	}
	return nil, nil
}
