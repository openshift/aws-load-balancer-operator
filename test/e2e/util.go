//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"reflect"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/aws/aws-sdk-go-v2/aws"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func waitForDeploymentStatusConditionUntilN(t *testing.T, cl client.Client, timeout time.Duration, count int, deploymentName types.NamespacedName, conditions ...appsv1.DeploymentCondition) error {
	t.Helper()
	ctr := 0
	return wait.PollImmediate(1*time.Second, timeout, func() (bool, error) {
		dep := &appsv1.Deployment{}
		if err := cl.Get(context.TODO(), deploymentName, dep); err != nil {
			t.Logf("failed to get deployment %s: %v (retrying)", deploymentName.Name, err)
			return false, nil
		}

		expected := deploymentConditionMap(conditions...)
		current := deploymentConditionMap(dep.Status.Conditions...)
		if conditionsMatchExpected(expected, current) {
			ctr++
		} else {
			return false, nil
		}

		return ctr >= count, nil
	})
}

func getIngress(t *testing.T, cl client.Client, timeout time.Duration, ingressName types.NamespacedName) (string, error) {
	t.Helper()
	var address string
	return address, wait.PollImmediate(1*time.Second, timeout, func() (bool, error) {
		ing := &networkingv1.Ingress{}
		if err := cl.Get(context.TODO(), ingressName, ing); err != nil {
			t.Logf("failed to get deployment %s: %v (retrying)", ingressName.Name, err)
			return false, nil
		}
		if len(ing.Status.LoadBalancer.Ingress) <= 0 || len(ing.Status.LoadBalancer.Ingress[0].Hostname) <= 0 {
			return false, nil
		}
		address = ing.Status.LoadBalancer.Ingress[0].Hostname
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
				},
			},
		},
	}
}

// buildCurlPod returns a pod definition for a pod with the given name and image
// and in the given namespace that curls the specified host and address.
func buildCurlPod(name, namespace, host, address string, extraArgs ...string) *corev1.Pod {
	curlArgs := []string{
		"-s",
		"-v",
		"--header", "HOST:" + host,
		"--retry", "300", "--retry-delay", "5", "--max-time", "2",
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
		annotations:  make(map[string]string),
		ingressclass: "alb",
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
