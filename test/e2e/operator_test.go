//go:build e2e
// +build e2e

package e2e

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	arv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	kscheme "k8s.io/client-go/kubernetes/scheme"

	configv1 "github.com/openshift/api/config/v1"
	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
)

var (
	kubeClient        client.Client
	kubeClientSet     *kubernetes.Clientset
	scheme            = kscheme.Scheme
	infraConfig       configv1.Infrastructure
	operatorName      = "aws-load-balancer-operator-controller-manager"
	operatorNamespace = "aws-load-balancer-operator"
	defaultTimeout    = 5 * time.Minute
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(cco.Install(scheme))
	utilruntime.Must(albo.AddToScheme(scheme))
	utilruntime.Must(cco.AddToScheme(scheme))
	utilruntime.Must(configv1.Install(scheme))
	utilruntime.Must(cco.Install(scheme))
	utilruntime.Must(networkingv1.AddToScheme(scheme))
	utilruntime.Must(arv1.AddToScheme(scheme))
}

func ensureCredentialsRequest() error {
	codec, err := cco.NewCodec()
	if err != nil {
		return err
	}

	providerSpec, err := codec.EncodeProviderSpec(&cco.AWSProviderSpec{
		StatementEntries: []cco.StatementEntry{
			{
				Action:   []string{"ec2:DescribeSubnets"},
				Effect:   "Allow",
				Resource: "*",
			},
			{
				Action:   []string{"ec2:CreateTags", "ec2:DeleteTags"},
				Effect:   "Allow",
				Resource: "arn:aws:ec2:*:*:subnet/*",
			},
			{
				Action:   []string{"ec2:DescribeVpcs"},
				Effect:   "Allow",
				Resource: "*",
			},
		},
	})
	if err != nil {
		return err
	}

	cr := cco.CredentialsRequest{
		ObjectMeta: v1.ObjectMeta{
			Name:      "aws-load-balancer-operator",
			Namespace: operatorNamespace,
		},
		Spec: cco.CredentialsRequestSpec{
			SecretRef: corev1.ObjectReference{
				Name:      "aws-load-balancer-operator",
				Namespace: operatorNamespace,
			},
			ServiceAccountNames: []string{operatorName},
			ProviderSpec:        providerSpec,
		},
	}

	if err = kubeClient.Create(context.Background(), &cr); err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	return nil
}

func newAWSLoadBalancerController(name types.NamespacedName) albo.AWSLoadBalancerController {
	return albo.AWSLoadBalancerController{
		ObjectMeta: v1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
		},
		Spec: albo.AWSLoadBalancerControllerSpec{
			SubnetTagging: albo.AutoSubnetTaggingPolicy,
		},
	}
}

func TestMain(m *testing.M) {
	kubeConfig, err := config.GetConfig()
	if err != nil {
		fmt.Printf("failed to get kube config: %s\n", err)
		os.Exit(1)
	}
	cl, err := client.New(kubeConfig, client.Options{})
	if err != nil {
		fmt.Printf("failed to create kube client: %s\n", err)
		os.Exit(1)
	}
	kubeClient = cl

	if err := kubeClient.Get(context.TODO(), types.NamespacedName{Name: "cluster"}, &infraConfig); err != nil {
		fmt.Printf("failed to get infrastructure config: %v\n", err)
		os.Exit(1)
	}

	kubeClientSet, err = kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		fmt.Printf("failed to create kube clientset: %s\n", err)
		os.Exit(1)
	}

	if err = ensureCredentialsRequest(); err != nil {
		fmt.Printf("failed to create credentialsrequest for operator due to: %v\n", err)
	}

	os.Exit(m.Run())
}

func TestOperatorAvailable(t *testing.T) {
	expected := []appsv1.DeploymentCondition{
		{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
	}
	name := types.NamespacedName{
		Name:      operatorName,
		Namespace: operatorNamespace,
	}
	if err := waitForDeploymentStatusCondition(t, kubeClient, defaultTimeout, name, expected...); err != nil {
		t.Errorf("Did not get expected available condition: %v", err)
	}
}

// TestAWSLoadBalancerControllerWithDefaultIngressClass tests the basic happy flow for the operator, mostly
// using the default values.
func TestAWSLoadBalancerControllerWithDefaultIngressClass(t *testing.T) {
	t.Log("Creating aws load balancer controller instance with default ingress class")

	name := types.NamespacedName{Name: "cluster", Namespace: "aws-load-balancer-operator"}
	alb := newAWSLoadBalancerController(name)
	if err := kubeClient.Create(context.TODO(), &alb); err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to create aws load balancer controller %q: %v", name, err)
	}
	defer func() {
		if err := kubeClient.Delete(context.TODO(), &alb); err != nil {
			t.Fatalf("failed to delete aws load balancer controller %s: %v", name, err)
		}
	}()

	expected := []appsv1.DeploymentCondition{
		{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
	}
	deploymentName := types.NamespacedName{Name: "aws-load-balancer-controller-cluster", Namespace: "aws-load-balancer-operator"}
	if err := waitForDeploymentStatusCondition(t, kubeClient, defaultTimeout, deploymentName, expected...); err != nil {
		t.Errorf("did not get expected available condition: %v", err)
	}

	// TODO: verify aws resources subnets, etc.

	testWorkloadNamespace := "aws-load-balancer-test"
	t.Logf("Ensuring test workload namespace %s", testWorkloadNamespace)
	ns := &corev1.Namespace{ObjectMeta: v1.ObjectMeta{Name: testWorkloadNamespace}}
	err := kubeClient.Create(context.TODO(), ns)
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure namespace %s: %v", testWorkloadNamespace, err)
	}
	defer func() {
		if err := kubeClient.Delete(context.TODO(), ns); err != nil {
			t.Fatalf("failed to delete namespace %s: %v", ns, err)
		}
	}()

	echopod := buildEchoPod("echoserver", testWorkloadNamespace)
	err = kubeClient.Create(context.TODO(), echopod)
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure echo pod %s: %v", echopod.Name, err)
	}

	echosvc := buildEchoService("echoserver", testWorkloadNamespace)
	err = kubeClient.Create(context.TODO(), echosvc)
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure echo service %s: %v", echosvc.Name, err)
	}

	t.Log("Creating Ingress Resource with default ingress class")
	ingName := types.NamespacedName{Name: "echoserver", Namespace: testWorkloadNamespace}
	ingAnnotations := map[string]string{
		"alb.ingress.kubernetes.io/scheme":      "internet-facing",
		"alb.ingress.kubernetes.io/target-type": "instance",
	}
	echoIng := buildEchoIngress(ingName, ingAnnotations, echosvc)
	err = kubeClient.Create(context.TODO(), echoIng)
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure echo service %s: %v", echoIng.Name, err)
	}

	var address string
	if address, err = verifyIngress(t, kubeClient, defaultTimeout, ingName); err != nil {
		t.Errorf("did not get expected available condition: %v", err)
	}

	t.Logf("Testing aws load balancer for ingress traffic at address %s", address)
	for i, rule := range echoIng.Spec.Rules {
		clientPod := buildCurlPod(fmt.Sprintf("clientpod-%d", i), testWorkloadNamespace, rule.Host, address)
		if err := kubeClient.Create(context.TODO(), clientPod); err != nil {
			t.Fatalf("failed to create pod %s/%s: %v", clientPod.Namespace, clientPod.Name, err)
		}

		err = wait.PollImmediate(1*time.Second, 10*time.Minute, func() (bool, error) {
			readCloser, err := kubeClientSet.CoreV1().Pods(clientPod.Namespace).GetLogs(clientPod.Name, &corev1.PodLogOptions{
				Container: "curl",
				Follow:    false,
			}).Stream(context.TODO())
			if err != nil {
				t.Logf("failed to read output from pod %s: %v (retrying)", clientPod.Name, err)
				return false, nil
			}
			scanner := bufio.NewScanner(readCloser)
			defer func() {
				if err := readCloser.Close(); err != nil {
					t.Errorf("failed to close reader for pod %s: %v", clientPod.Name, err)
				}
			}()
			for scanner.Scan() {
				line := scanner.Text()
				if strings.Contains(line, "HTTP/1.1 200 OK") {
					return true, nil
				}
			}
			return false, nil
		})
		if err != nil {
			t.Fatalf("failed to observe the expected log message: %v", err)
		}
	}
}