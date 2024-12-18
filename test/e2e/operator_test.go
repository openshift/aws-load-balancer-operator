//go:build e2e
// +build e2e

package e2e

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"sort"
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
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	waf "github.com/aws/aws-sdk-go-v2/service/wafregional"
	waftypes "github.com/aws/aws-sdk-go-v2/service/wafregional/types"
	"github.com/aws/aws-sdk-go-v2/service/wafv2"
	wafv2types "github.com/aws/aws-sdk-go-v2/service/wafv2/types"
	configv1 "github.com/openshift/api/config/v1"
	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	elbv1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1"
	albov1alpha1 "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
)

const (
	e2ePlatformVarName = "ALBO_E2E_PLATFORM"

	// WAF/WAFV2 web ACLs are needed to test the ALB WAF addons.
	// The environment variables below allow the user to provide the existing ACLs.
	// These variables are required for the e2e when it's run on the ROSA/STS cluster.
	// The e2e cannot build a valid AWS config to create the ACLs itself.
	// Because the e2e binary doesn't have the token required by AWS to provision the temporary STS credentials
	// unlike the operator and the controller which use the service account token signed by OpenShift.
	wafv2WebACLARNVarName = "ALBO_E2E_WAFV2_WEBACL_ARN"
	wafWebACLIDVarName    = "ALBO_E2E_WAF_WEBACL_ID"
	// controllerRoleARNVarName contains IAM role ARN to be used by the controller on a ROSA/STS cluster.
	controllerRoleARNVarName = "ALBO_E2E_CONTROLLER_ROLE_ARN"

	// controllerSecretName is the name of the controller's cloud credential secret provisioned by the CI.
	controllerSecretName = "aws-load-balancer-controller-cluster"

	// awsLoadBalancerControllerContainerName is the name of the AWS load balancer controller's container.
	awsLoadBalancerControllerContainerName = "controller"
)

var (
	cfg                aws.Config
	kubeClient         client.Client
	kubeClientSet      *kubernetes.Clientset
	infra              configv1.Infrastructure
	scheme             = clientgoscheme.Scheme
	operatorName       = "aws-load-balancer-operator-controller-manager"
	operatorNamespace  = "aws-load-balancer-operator"
	defaultTimeout     = 15 * time.Minute
	defaultRetryPolicy = wait.Backoff{
		Duration: 5 * time.Second,
		Factor:   1.0,
		Jitter:   1.0,
		Steps:    10,
	}
	httpClient    = http.Client{Timeout: 5 * time.Second}
	e2eSecretName = types.NamespacedName{
		Name:      "aws-load-balancer-operator-e2e",
		Namespace: operatorNamespace,
	}
	controllerRoleARN string
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(cco.Install(scheme))
	utilruntime.Must(albo.AddToScheme(scheme))
	utilruntime.Must(albov1alpha1.AddToScheme(scheme))
	utilruntime.Must(cco.AddToScheme(scheme))
	utilruntime.Must(configv1.Install(scheme))
	utilruntime.Must(cco.Install(scheme))
	utilruntime.Must(networkingv1.AddToScheme(scheme))
	utilruntime.Must(arv1.AddToScheme(scheme))
	utilruntime.Must(elbv1beta1.AddToScheme(scheme))
}

func TestMain(m *testing.M) {
	kubeConfig, err := config.GetConfig()
	if err != nil {
		fmt.Printf("failed to get kube config: %s\n", err)
		os.Exit(1)
	}
	kubeClient, err = client.New(kubeConfig, client.Options{})
	if err != nil {
		fmt.Printf("failed to create kube client: %s\n", err)
		os.Exit(1)
	}
	kubeClientSet, err = kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		fmt.Printf("failed to create kube clientset: %s\n", err)
		os.Exit(1)
	}

	err = kubeClient.Get(context.TODO(), types.NamespacedName{Name: "cluster"}, &infra)
	if err != nil {
		fmt.Printf("failed to fetch infrastructure: %v\n", err)
		os.Exit(1)
	}

	if infra.Status.PlatformStatus == nil || infra.Status.PlatformStatus.AWS == nil || infra.Status.PlatformStatus.AWS.Region == "" {
		fmt.Println("could not get AWS region from Infrastructure status")
		os.Exit(1)
	}

	if !stsModeRequested() {
		if err := ensureCredentialsRequest(e2eSecretName); err != nil {
			fmt.Printf("failed to create credentialsrequest for e2e: %s\n", err)
			os.Exit(1)
		}
		cfg, err = awsConfigWithCredentials(context.TODO(), kubeClient, infra.Status.PlatformStatus.AWS.Region, e2eSecretName)
		if err != nil {
			fmt.Printf("failed to load aws config %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Println("Controller role is expected to exist when the test is run on a ROSA STS cluster")
		controllerRoleARN = mustGetEnv(controllerRoleARNVarName)
	}

	os.Exit(m.Run())
}

func getOperator(t *testing.T) {
	expected := []appsv1.DeploymentCondition{
		{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
	}
	name := types.NamespacedName{
		Name:      operatorName,
		Namespace: operatorNamespace,
	}
	if err := waitForDeploymentStatusCondition(context.TODO(), t, kubeClient, defaultTimeout, name, expected...); err != nil {
		t.Fatalf("Did not get expected available condition: %v", err)
	}
}

func TestOperatorAvailable(t *testing.T) {
	getOperator(t)
}

// TestAWSLoadBalancerControllerWithDefaultIngressClass tests the basic happy flow for the operator, mostly
// using the default values.
func TestAWSLoadBalancerControllerWithDefaultIngressClass(t *testing.T) {
	// The test namespace should be created earlier
	// to let the pull secret for internal registry images to be created.
	// The test workload uses the tools image from the internal image registry.
	testWorkloadNamespace := "aws-load-balancer-test-default-ing"
	t.Logf("Creating test namespace %q", testWorkloadNamespace)
	echoNs := createTestNamespace(t, testWorkloadNamespace)
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, echoNs, defaultTimeout)
	}()

	t.Log("Creating aws load balancer controller instance with default ingress class")

	alb := newALBCBuilder().withRoleARNIf(stsModeRequested(), controllerRoleARN).build()
	if err := kubeClient.Create(context.TODO(), alb); err != nil {
		t.Fatalf("failed to create aws load balancer controller: %v", err)
	}
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, alb, defaultTimeout)
	}()

	expected := []appsv1.DeploymentCondition{
		{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
	}
	deploymentName := types.NamespacedName{Name: "aws-load-balancer-controller-cluster", Namespace: "aws-load-balancer-operator"}
	if err := waitForDeploymentStatusCondition(context.TODO(), t, kubeClient, defaultTimeout, deploymentName, expected...); err != nil {
		t.Fatalf("did not get expected available condition for deployment: %v", err)
	}

	t.Logf("Creating test workload in %q namespace", testWorkloadNamespace)
	echoSvc := createTestWorkload(t, testWorkloadNamespace)

	t.Log("Creating Ingress Resource with default ingress class")
	ingName := types.NamespacedName{Name: "echoserver", Namespace: testWorkloadNamespace}
	ingAnnotations := map[string]string{
		"alb.ingress.kubernetes.io/scheme":      "internet-facing",
		"alb.ingress.kubernetes.io/target-type": "instance",
	}
	echoIng := buildEchoIngress(ingName, "alb", ingAnnotations, echoSvc)
	err := retry.OnError(defaultRetryPolicy,
		func(err error) bool {
			if errors.IsAlreadyExists(err) {
				return false
			}
			t.Logf("retrying creation of echo ingress due to %v", err)
			return true
		},
		func() error { return kubeClient.Create(context.TODO(), echoIng) })
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure echo ingress %s: %v", echoIng.Name, err)
	}
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, echoIng, defaultTimeout)
	}()

	address, err := getIngress(context.TODO(), t, kubeClient, defaultTimeout, ingName)
	if err != nil {
		t.Fatalf("did not get load balancer address for ingress: %v", err)
	}

	t.Logf("Testing aws load balancer for ingress traffic at address %s", address)
	for _, rule := range echoIng.Spec.Rules {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s", address), nil)
		if err != nil {
			t.Fatalf("failed to build client request: %v", err)
		}
		req.Host = rule.Host

		err = waitForHTTPClientCondition(context.TODO(), t, &httpClient, req, 5*time.Second, defaultTimeout, func(r *http.Response) bool {
			return r.StatusCode == http.StatusOK
		})
		if err != nil {
			t.Fatalf("failed to verify condition with external client: %v", err)
		}
	}
}

// TestAWSLoadBalancerControllersV1Alpha1 tests the basic happy flow for the operator using v1alpha1 ALBC.
func TestAWSLoadBalancerControllersV1Alpha1(t *testing.T) {
	testWorkloadNamespace := "aws-load-balancer-test-v1alpha1"
	t.Logf("Creating test namespace %q", testWorkloadNamespace)
	echoNs := createTestNamespace(t, testWorkloadNamespace)
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, echoNs, defaultTimeout)
	}()

	t.Log("Creating v1alpha1 aws load balancer controller instance with default ingress class, additional resource tags and credentials secret")

	// The additional resource tags and the credentials secret are added to ALBC
	// because they changed in v1.
	alb := newALBCBuilder().withResourceTags(map[string]string{"testtag": "testval"}).withCredSecret(controllerSecretName).buildv1alpha1()
	if err := kubeClient.Create(context.TODO(), alb); err != nil {
		t.Fatalf("failed to create aws load balancer controller: %v", err)
	}
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, alb, defaultTimeout)
	}()

	expected := []appsv1.DeploymentCondition{
		{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
	}
	deploymentName := types.NamespacedName{Name: "aws-load-balancer-controller-cluster", Namespace: "aws-load-balancer-operator"}
	if err := waitForDeploymentStatusCondition(context.TODO(), t, kubeClient, defaultTimeout, deploymentName, expected...); err != nil {
		t.Fatalf("did not get expected available condition for deployment: %v", err)
	}

	t.Logf("Creating test workload in %q namespace", testWorkloadNamespace)
	echoSvc := createTestWorkload(t, testWorkloadNamespace)
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, echoSvc, defaultTimeout)
	}()

	t.Log("Creating Ingress Resource with default ingress class")
	ingName := types.NamespacedName{Name: "echoserver", Namespace: testWorkloadNamespace}
	ingAnnotations := map[string]string{
		"alb.ingress.kubernetes.io/scheme":      "internet-facing",
		"alb.ingress.kubernetes.io/target-type": "instance",
	}
	echoIng := buildEchoIngress(ingName, "alb", ingAnnotations, echoSvc)
	err := retry.OnError(defaultRetryPolicy,
		func(err error) bool {
			if errors.IsAlreadyExists(err) {
				return false
			}
			t.Logf("retrying creation of echo ingress due to %v", err)
			return true
		},
		func() error { return kubeClient.Create(context.TODO(), echoIng) })
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure echo ingress %s: %v", echoIng.Name, err)
	}
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, echoIng, defaultTimeout)
	}()

	address, err := getIngress(context.TODO(), t, kubeClient, defaultTimeout, ingName)
	if err != nil {
		t.Fatalf("did not get load balancer address for ingress: %v", err)
	}

	t.Logf("Testing aws load balancer for ingress traffic at address %s", address)
	for _, rule := range echoIng.Spec.Rules {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s", address), nil)
		if err != nil {
			t.Fatalf("failed to build client request: %v", err)
		}
		req.Host = rule.Host

		err = waitForHTTPClientCondition(context.TODO(), t, &httpClient, req, 5*time.Second, defaultTimeout, func(r *http.Response) bool {
			return r.StatusCode == http.StatusOK
		})
		if err != nil {
			t.Fatalf("failed to verify condition with external client: %v", err)
		}
	}
}

// TestAWSLoadBalancerControllerWithCredentialsSecret tests the basic happy flow for the operator
// using the explicitly specified credentials secret.
func TestAWSLoadBalancerControllerWithCredentialsSecret(t *testing.T) {
	testWorkloadNamespace := "aws-load-balancer-test-cred-secret"
	t.Logf("Creating test namespace %q", testWorkloadNamespace)
	echoNs := createTestNamespace(t, testWorkloadNamespace)
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, echoNs, defaultTimeout)
	}()

	t.Log("Creating aws load balancer controller instance with credentials secret")
	alb := newALBCBuilder().withCredSecret(controllerSecretName).build()
	if err := kubeClient.Create(context.TODO(), alb); err != nil {
		t.Fatalf("failed to create aws load balancer controller: %v", err)
	}
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, alb, defaultTimeout)
	}()

	expected := []appsv1.DeploymentCondition{
		{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
	}
	deploymentName := types.NamespacedName{Name: "aws-load-balancer-controller-cluster", Namespace: "aws-load-balancer-operator"}
	if err := waitForDeploymentStatusCondition(context.TODO(), t, kubeClient, defaultTimeout, deploymentName, expected...); err != nil {
		t.Fatalf("did not get expected available condition for deployment: %v", err)
	}

	t.Logf("Creating test workload in %q namespace", testWorkloadNamespace)
	echoSvc := createTestWorkload(t, testWorkloadNamespace)
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, echoSvc, defaultTimeout)
	}()

	t.Log("Creating Ingress Resource with default ingress class")
	ingName := types.NamespacedName{Name: "echoserver", Namespace: testWorkloadNamespace}
	ingAnnotations := map[string]string{
		"alb.ingress.kubernetes.io/scheme":      "internet-facing",
		"alb.ingress.kubernetes.io/target-type": "instance",
	}
	echoIng := buildEchoIngress(ingName, "alb", ingAnnotations, echoSvc)
	err := retry.OnError(defaultRetryPolicy,
		func(err error) bool {
			if errors.IsAlreadyExists(err) {
				return false
			}
			t.Logf("retrying creation of echo ingress due to %v", err)
			return true
		},
		func() error { return kubeClient.Create(context.TODO(), echoIng) })
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure echo ingress %s: %v", echoIng.Name, err)
	}
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, echoIng, defaultTimeout)
	}()

	address, err := getIngress(context.TODO(), t, kubeClient, defaultTimeout, ingName)
	if err != nil {
		t.Fatalf("did not get load balancer address for ingress: %v", err)
	}

	t.Logf("Testing aws load balancer for ingress traffic at address %s", address)
	for _, rule := range echoIng.Spec.Rules {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s", address), nil)
		if err != nil {
			t.Fatalf("failed to build client request: %v", err)
		}
		req.Host = rule.Host

		err = waitForHTTPClientCondition(context.TODO(), t, &httpClient, req, 5*time.Second, defaultTimeout, func(r *http.Response) bool {
			return r.StatusCode == http.StatusOK
		})
		if err != nil {
			t.Fatalf("failed to verify condition with external client: %v", err)
		}
	}
}

func TestAWSLoadBalancerControllerWithCustomIngressClass(t *testing.T) {
	testWorkloadNamespace := "aws-load-balancer-test-custom-ing"
	t.Logf("Creating test namespace %q", testWorkloadNamespace)
	echoNs := createTestNamespace(t, testWorkloadNamespace)
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, echoNs, defaultTimeout)
	}()

	t.Log("Creating a custom ingress class")
	ingclassName := types.NamespacedName{Name: "custom-alb", Namespace: "aws-load-balancer-operator"}
	ingclass := buildIngressClass(ingclassName, "ingress.k8s.aws/alb")
	if err := kubeClient.Create(context.TODO(), ingclass); err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure custom ingress class %q: %v", ingclass.Name, err)
	}
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, ingclass, defaultTimeout)
	}()

	t.Log("Creating aws load balancer controller instance with custom ingress class")

	alb := newALBCBuilder().withIngressClass(ingclassName.Name).withRoleARNIf(stsModeRequested(), controllerRoleARN).build()
	if err := kubeClient.Create(context.TODO(), alb); err != nil {
		t.Fatalf("failed to create aws load balancer controller: %v", err)
	}
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, alb, defaultTimeout)
	}()

	expected := []appsv1.DeploymentCondition{
		{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
	}
	deploymentName := types.NamespacedName{Name: "aws-load-balancer-controller-cluster", Namespace: "aws-load-balancer-operator"}
	if err := waitForDeploymentStatusCondition(context.TODO(), t, kubeClient, defaultTimeout, deploymentName, expected...); err != nil {
		t.Fatalf("did not get expected available condition for deployment: %v", err)
	}

	t.Logf("Creating test workload in %q namespace", testWorkloadNamespace)
	echoSvc := createTestWorkload(t, testWorkloadNamespace)
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, echoSvc, defaultTimeout)
	}()

	t.Log("Creating Ingress Resource with custom ingress class")
	ingName := types.NamespacedName{Name: "echoserver", Namespace: testWorkloadNamespace}
	ingAnnotations := map[string]string{
		"alb.ingress.kubernetes.io/scheme":      "internet-facing",
		"alb.ingress.kubernetes.io/target-type": "instance",
	}
	echoIng := buildEchoIngress(ingName, ingclass.Name, ingAnnotations, echoSvc)
	err := retry.OnError(defaultRetryPolicy,
		func(err error) bool {
			if errors.IsAlreadyExists(err) {
				return false
			}
			t.Logf("retrying creation of echo ingress due to %v", err)
			return true
		},
		func() error { return kubeClient.Create(context.TODO(), echoIng) })
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure echo ingress %s: %v", echoIng.Name, err)
	}
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, echoIng, defaultTimeout)
	}()

	address, err := getIngress(context.TODO(), t, kubeClient, defaultTimeout, ingName)
	if err != nil {
		t.Fatalf("did not get load balancer address for ingress: %v", err)
	}

	t.Logf("Testing aws load balancer for ingress traffic at address %s", address)
	for _, rule := range echoIng.Spec.Rules {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s", address), nil)
		if err != nil {
			t.Fatalf("failed to build client request: %v", err)
		}
		req.Host = rule.Host

		err = waitForHTTPClientCondition(context.TODO(), t, &httpClient, req, 5*time.Second, defaultTimeout, func(r *http.Response) bool {
			return r.StatusCode == http.StatusOK
		})
		if err != nil {
			t.Fatalf("failed to verify condition with external client: %v", err)
		}
	}
}

func TestAWSLoadBalancerControllerWithInternalLoadBalancer(t *testing.T) {
	testWorkloadNamespace := "aws-load-balancer-test-internal-ing"
	t.Logf("Creating test namespace %q", testWorkloadNamespace)
	echoNs := createTestNamespace(t, testWorkloadNamespace)
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, echoNs, defaultTimeout)
	}()

	t.Log("Creating aws load balancer controller instance with default ingress class")

	alb := newALBCBuilder().withRoleARNIf(stsModeRequested(), controllerRoleARN).build()
	if err := kubeClient.Create(context.TODO(), alb); err != nil {
		t.Fatalf("failed to create aws load balancer controller: %v", err)
	}
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, alb, defaultTimeout)
	}()

	expected := []appsv1.DeploymentCondition{
		{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
	}
	deploymentName := types.NamespacedName{Name: "aws-load-balancer-controller-cluster", Namespace: "aws-load-balancer-operator"}
	if err := waitForDeploymentStatusCondition(context.TODO(), t, kubeClient, defaultTimeout, deploymentName, expected...); err != nil {
		t.Fatalf("did not get expected available condition for deployment: %v", err)
	}

	t.Logf("Creating test workload in %q namespace", testWorkloadNamespace)
	echoSvc := createTestWorkload(t, testWorkloadNamespace)
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, echoSvc, defaultTimeout)
	}()

	t.Log("Creating Internal Ingress Resource with default ingress class")
	ingName := types.NamespacedName{Name: "echoserver", Namespace: testWorkloadNamespace}
	ingAnnotations := map[string]string{
		"alb.ingress.kubernetes.io/scheme":      "internal",
		"alb.ingress.kubernetes.io/target-type": "instance",
	}
	echoIng := buildEchoIngress(ingName, "alb", ingAnnotations, echoSvc)
	err := retry.OnError(defaultRetryPolicy,
		func(err error) bool {
			if errors.IsAlreadyExists(err) {
				return false
			}
			t.Logf("retrying creation of echo ingress due to %v", err)
			return true
		},
		func() error { return kubeClient.Create(context.TODO(), echoIng) })
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure echo ingress %s: %v", echoIng.Name, err)
	}
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, echoIng, defaultTimeout)
	}()

	address, err := getIngress(context.TODO(), t, kubeClient, defaultTimeout, ingName)
	if err != nil {
		t.Fatalf("did not get load balancer address for ingress: %v", err)
	}

	t.Logf("Testing aws load balancer for ingress traffic at address %s", address)
	for i, rule := range echoIng.Spec.Rules {
		clientPod := buildCurlPod(fmt.Sprintf("clientpod-%d", i), testWorkloadNamespace, rule.Host, address)
		if err := kubeClient.Create(context.TODO(), clientPod); err != nil {
			t.Fatalf("failed to create pod %s/%s: %v", clientPod.Namespace, clientPod.Name, err)
		}

		err = wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 10*time.Minute, true, func(ctx context.Context) (bool, error) {
			readCloser, err := kubeClientSet.CoreV1().Pods(clientPod.Namespace).GetLogs(clientPod.Name, &corev1.PodLogOptions{
				Container: "curl",
				Follow:    false,
			}).Stream(ctx)
			if err != nil {
				t.Logf("failed to read output from pod %s: %v (retrying)", clientPod.Name, err)
				return false, nil
			}
			scanner := bufio.NewScanner(readCloser)
			defer func() {
				if err := readCloser.Close(); err != nil {
					t.Fatalf("failed to close reader for pod %s: %v", clientPod.Name, err)
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

		waitForDeletion(context.TODO(), t, kubeClient, clientPod, defaultTimeout)
	}
}

func TestAWSLoadBalancerControllerWithWAFv2(t *testing.T) {
	// Creating the Web ACL as early as possible as it's not available immediately
	// for the association with a load balancer.
	t.Logf("Getting WAFv2 WebACL")
	var aclARN string
	if !stsModeRequested() {
		wafClient := wafv2.NewFromConfig(cfg)
		webACLName := fmt.Sprintf("echoserver-acl-%d", time.Now().Unix())
		acl, err := findAWSWebACL(wafClient, webACLName)
		if err != nil {
			t.Logf("failed to find %q aws wafv2 acl due to %v, continue to creation anyway", webACLName, err)
		}
		if acl == nil {
			t.Logf("WAFv2 ACL %q was not found, creating one", webACLName)
			createdACL, err := wafClient.CreateWebACL(context.Background(), &wafv2.CreateWebACLInput{
				DefaultAction: &wafv2types.DefaultAction{Block: &wafv2types.BlockAction{}},
				Name:          aws.String(webACLName),
				Scope:         wafv2types.ScopeRegional,
				VisibilityConfig: &wafv2types.VisibilityConfig{
					CloudWatchMetricsEnabled: false,
					MetricName:               aws.String("echoserver"),
					SampledRequestsEnabled:   false,
				},
			})
			if err != nil {
				t.Fatalf("failed to create aws wafv2 acl due to %v", err)
			}
			acl = createdACL.Summary
		}

		aclARN = *acl.ARN
		t.Logf("Found WAFv2 WebACL. ID: %s, Name: %s, ARN: %s", *acl.Id, *acl.Name, aclARN)

		defer func() {
			_, err = wafClient.DeleteWebACL(context.TODO(), &wafv2.DeleteWebACLInput{
				Id:        aws.String(*acl.Id),
				Name:      aws.String(webACLName),
				LockToken: acl.LockToken,
				Scope:     wafv2types.ScopeRegional,
			})
			if err != nil {
				t.Fatalf("failed to delete aws wafv2 acl due to %v", err)
			}
		}()
	} else {
		// Web ACLs are provisioned by CI on ROSA cluster.
		aclARN = os.Getenv(wafv2WebACLARNVarName)
		if aclARN == "" {
			t.Fatalf("no wafv2 webacl arn provided")
		}
	}
	t.Logf("Got WAFv2 WebACL. ARN: %s", aclARN)

	testWorkloadNamespace := "aws-load-balancer-test-wafv2"
	t.Logf("Creating test namespace %q", testWorkloadNamespace)
	echoNs := createTestNamespace(t, testWorkloadNamespace)
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, echoNs, defaultTimeout)
	}()

	t.Log("Creating aws load balancer controller instance with default ingress class")

	alb := newALBCBuilder().withAddons(albo.AWSAddonWAFv2).withRoleARNIf(stsModeRequested(), controllerRoleARN).build()
	if err := kubeClient.Create(context.TODO(), alb); err != nil {
		t.Fatalf("failed to create aws load balancer controller: %v", err)
	}
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, alb, defaultTimeout)
	}()

	expected := []appsv1.DeploymentCondition{
		{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
	}
	deploymentName := types.NamespacedName{Name: "aws-load-balancer-controller-cluster", Namespace: "aws-load-balancer-operator"}
	if err := waitForDeploymentStatusCondition(context.TODO(), t, kubeClient, defaultTimeout, deploymentName, expected...); err != nil {
		t.Fatalf("did not get expected available condition for deployment: %v", err)
	}

	t.Logf("Creating test workload in %q namespace", testWorkloadNamespace)
	echoSvc := createTestWorkload(t, testWorkloadNamespace)
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, echoSvc, defaultTimeout)
	}()

	t.Log("Creating Ingress Resource with default ingress class")
	ingName := types.NamespacedName{Name: "echoserver", Namespace: testWorkloadNamespace}
	ingAnnotations := map[string]string{
		"alb.ingress.kubernetes.io/scheme":        "internet-facing",
		"alb.ingress.kubernetes.io/target-type":   "instance",
		"alb.ingress.kubernetes.io/wafv2-acl-arn": aclARN,
	}
	echoIng := buildEchoIngress(ingName, "alb", ingAnnotations, echoSvc)
	err := retry.OnError(defaultRetryPolicy,
		func(err error) bool {
			if errors.IsAlreadyExists(err) {
				return false
			}
			t.Logf("retrying creation of echo ingress due to %v", err)
			return true
		},
		func() error { return kubeClient.Create(context.TODO(), echoIng) })
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure echo ingress %s: %v", echoIng.Name, err)
	}
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, echoIng, defaultTimeout)
	}()

	address, err := getIngress(context.TODO(), t, kubeClient, defaultTimeout, ingName)
	if err != nil {
		t.Fatalf("did not get load balancer address for ingress: %v", err)
	}

	t.Logf("Testing aws load balancer for ingress traffic at address %s", address)
	for _, rule := range echoIng.Spec.Rules {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s", address), nil)
		if err != nil {
			t.Fatalf("failed to build client request: %v", err)
		}
		req.Host = rule.Host

		err = waitForHTTPClientCondition(context.TODO(), t, &httpClient, req, 5*time.Second, defaultTimeout, func(r *http.Response) bool {
			return r.StatusCode == http.StatusForbidden
		})
		if err != nil {
			t.Fatalf("failed to verify condition with external client: %v", err)
		}
	}
}

func TestAWSLoadBalancerControllerWithWAFRegional(t *testing.T) {
	t.Log("Getting WAFRegional WebACL")
	var webACLID string
	if !stsModeRequested() {
		wafClient := waf.NewFromConfig(cfg)

		token, err := wafClient.GetChangeToken(context.TODO(), &waf.GetChangeTokenInput{})
		if err != nil {
			t.Fatalf("failed to get change token for waf regional classic %v", err)
		}
		aclName := fmt.Sprintf("echoserverclassicacl%d", time.Now().Unix())
		acl, err := wafClient.CreateWebACL(context.TODO(), &waf.CreateWebACLInput{
			DefaultAction: &waftypes.WafAction{Type: waftypes.WafActionTypeBlock},
			MetricName:    aws.String(aclName),
			Name:          aws.String(aclName),
			ChangeToken:   token.ChangeToken,
		})
		if err != nil {
			t.Fatalf("failed to create aws waf regional acl due to %v", err)
		}
		webACLID = *acl.WebACL.WebACLId
		defer func() {
			token, err := wafClient.GetChangeToken(context.TODO(), &waf.GetChangeTokenInput{})
			if err != nil {
				t.Fatalf("failed to get change token for waf regional classic %v", err)
			}

			_, err = wafClient.DeleteWebACL(context.TODO(), &waf.DeleteWebACLInput{
				ChangeToken: token.ChangeToken,
				WebACLId:    acl.WebACL.WebACLId,
			})
			if err != nil {
				t.Fatalf("failed to delete aws waf regional acl due to %v", err)
			}
		}()
	} else {
		// Web ACLs are provisioned by CI on ROSA cluster.
		webACLID = os.Getenv(wafWebACLIDVarName)
		if webACLID == "" {
			t.Fatalf("no wafregional webacl id provided")
		}
	}
	t.Logf("Got WAFRegional WebACL. ID: %s", webACLID)

	testWorkloadNamespace := "aws-load-balancer-test-wafregional"
	t.Logf("Creating test namespace %q", testWorkloadNamespace)
	echoNs := createTestNamespace(t, testWorkloadNamespace)
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, echoNs, defaultTimeout)
	}()

	t.Log("Creating aws load balancer controller instance with default ingress class")

	alb := newALBCBuilder().withAddons(albo.AWSAddonWAFv1).withRoleARNIf(stsModeRequested(), controllerRoleARN).build()
	if err := kubeClient.Create(context.TODO(), alb); err != nil {
		t.Fatalf("failed to create aws load balancer controller: %v", err)
	}
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, alb, defaultTimeout)
	}()

	expected := []appsv1.DeploymentCondition{
		{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
	}
	deploymentName := types.NamespacedName{Name: "aws-load-balancer-controller-cluster", Namespace: "aws-load-balancer-operator"}
	if err := waitForDeploymentStatusCondition(context.TODO(), t, kubeClient, defaultTimeout, deploymentName, expected...); err != nil {
		t.Fatalf("did not get expected available condition for deployment: %v", err)
	}

	echoSvc := createTestWorkload(t, testWorkloadNamespace)
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, echoSvc, defaultTimeout)
	}()

	t.Log("Creating Ingress Resource with default ingress class")
	ingName := types.NamespacedName{Name: "echoserver", Namespace: testWorkloadNamespace}
	ingAnnotations := map[string]string{
		"alb.ingress.kubernetes.io/scheme":      "internet-facing",
		"alb.ingress.kubernetes.io/target-type": "instance",
		"alb.ingress.kubernetes.io/waf-acl-id":  webACLID,
	}
	echoIng := buildEchoIngress(ingName, "alb", ingAnnotations, echoSvc)
	err := retry.OnError(defaultRetryPolicy,
		func(err error) bool {
			if errors.IsAlreadyExists(err) {
				return false
			}
			t.Logf("retrying creation of echo ingress due to %v", err)
			return true
		},
		func() error { return kubeClient.Create(context.TODO(), echoIng) })
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure echo ingress %s: %v", echoIng.Name, err)
	}
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, echoIng, defaultTimeout)
	}()

	address, err := getIngress(context.TODO(), t, kubeClient, 20*time.Minute, ingName)
	if err != nil {
		t.Fatalf("did not get load balancer address for ingress: %v", err)
	}

	t.Logf("Testing aws load balancer for ingress traffic at address %s", address)
	for _, rule := range echoIng.Spec.Rules {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s", address), nil)
		if err != nil {
			t.Fatalf("failed to build client request: %v", err)
		}
		req.Host = rule.Host

		err = waitForHTTPClientCondition(context.TODO(), t, &httpClient, req, 5*time.Second, defaultTimeout, func(r *http.Response) bool {
			return r.StatusCode == http.StatusForbidden
		})
		if err != nil {
			t.Fatalf("failed to verify condition with external client: %v", err)
		}
	}
}

func TestAWSLoadBalancerControllerWithIngressGroup(t *testing.T) {
	testWorkloadNamespace := "aws-load-balancer-test-ing-group"
	t.Logf("Creating test namespace %q", testWorkloadNamespace)
	echoNs := createTestNamespace(t, testWorkloadNamespace)
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, echoNs, defaultTimeout)
	}()

	t.Logf("Creating a custom IngressClassParams")
	ingressClassParams := &elbv1beta1.IngressClassParams{
		ObjectMeta: v1.ObjectMeta{
			Name: "multi-ingress-params",
		},
		Spec: elbv1beta1.IngressClassParamsSpec{
			Group: &elbv1beta1.IngressGroup{
				Name: "multi-ingress",
			},
		},
	}

	if err := kubeClient.Create(context.TODO(), ingressClassParams); err != nil {
		t.Fatalf("failed to create IngressClassParams %s: %v", ingressClassParams.Name, err)
	}
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, ingressClassParams, defaultTimeout)
	}()

	t.Log("Creating a custom ingress class")
	ingressClassName := types.NamespacedName{Name: "multi-ingress", Namespace: "aws-load-balancer-operator"}
	ingressClass := buildIngressClass(ingressClassName, "ingress.k8s.aws/alb")
	ingressClass.Spec.Parameters = &networkingv1.IngressClassParametersReference{
		APIGroup: ptr.To[string](elbv1beta1.GroupVersion.Group),
		Kind:     "IngressClassParams",
		Name:     ingressClassParams.Name,
	}
	if err := kubeClient.Create(context.TODO(), ingressClass); err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure custom ingress class %q: %v", ingressClass.Name, err)
	}
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, ingressClass, defaultTimeout)
	}()

	t.Log("Creating aws load balancer controller instance with custom ingress class")

	alb := newALBCBuilder().withIngressClass(ingressClassName.Name).withRoleARNIf(stsModeRequested(), controllerRoleARN).build()
	if err := kubeClient.Create(context.TODO(), alb); err != nil {
		t.Fatalf("failed to create aws load balancer controller: %v", err)
	}
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, alb, defaultTimeout)
	}()

	expected := []appsv1.DeploymentCondition{
		{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
	}
	deploymentName := types.NamespacedName{Name: "aws-load-balancer-controller-cluster", Namespace: "aws-load-balancer-operator"}
	if err := waitForDeploymentStatusCondition(context.TODO(), t, kubeClient, defaultTimeout, deploymentName, expected...); err != nil {
		t.Fatalf("did not get expected available condition for deployment: %v", err)
	}

	t.Logf("Creating test workload in %q namespace", testWorkloadNamespace)
	echoSvc := createTestWorkload(t, testWorkloadNamespace)
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, echoSvc, defaultTimeout)
	}()

	t.Log("Creating Ingress Resource 1 with custom ingress class")
	ingName1 := types.NamespacedName{Name: "echoserver-1", Namespace: testWorkloadNamespace}
	ingAnnotations := map[string]string{
		"alb.ingress.kubernetes.io/scheme": "internet-facing",
	}
	echoIng1 := buildEchoIngress(ingName1, ingressClass.Name, ingAnnotations, echoSvc)
	err := retry.OnError(defaultRetryPolicy,
		func(err error) bool {
			if errors.IsAlreadyExists(err) {
				return false
			}
			t.Logf("retrying creation of echo ingress due to %v", err)
			return true
		},
		func() error { return kubeClient.Create(context.TODO(), echoIng1) })
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure echo ingress %s: %v", echoIng1.Name, err)
	}
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, echoIng1, defaultTimeout)
	}()

	t.Log("Creating Ingress Resource 2 with custom ingress class")
	ingName2 := types.NamespacedName{Name: "echoserver-2", Namespace: testWorkloadNamespace}
	echoIng2 := buildEchoIngress(ingName2, ingressClass.Name, ingAnnotations, echoSvc)
	err = retry.OnError(defaultRetryPolicy,
		func(err error) bool {
			if errors.IsAlreadyExists(err) {
				return false
			}
			t.Logf("retrying creation of echo ingress due to %v", err)
			return true
		},
		func() error { return kubeClient.Create(context.TODO(), echoIng2) })
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure echo ingress %s: %v", echoIng2.Name, err)
	}
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, echoIng2, defaultTimeout)
	}()

	t.Log("Creating Ingress Resource 3 with custom ingress class")
	ingName3 := types.NamespacedName{Name: "echoserver-3", Namespace: testWorkloadNamespace}
	echoIng3 := buildEchoIngress(ingName3, ingressClass.Name, ingAnnotations, echoSvc)
	err = retry.OnError(defaultRetryPolicy,
		func(err error) bool {
			if errors.IsAlreadyExists(err) {
				return false
			}
			t.Logf("retrying creation of echo ingress due to %v", err)
			return true
		},
		func() error { return kubeClient.Create(context.TODO(), echoIng3) })
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure echo ingress %s: %v", echoIng3.Name, err)
	}
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, echoIng3, defaultTimeout)
	}()

	firstAddress, err := getIngress(context.TODO(), t, kubeClient, defaultTimeout, ingName1)
	if err != nil {
		t.Fatalf("did not get load balancer address for ingress: %v", err)
	}

	for _, ingressName := range []types.NamespacedName{ingName2, ingName3} {
		address, err := getIngress(context.TODO(), t, kubeClient, defaultTimeout, ingressName)
		if err != nil {
			t.Fatalf("did not get load balancer address for ingress %s: %v", ingressName, err)
		}
		if address != firstAddress {
			t.Errorf("ingress %s does not have the address %s, instead has %s", ingressName, firstAddress, address)
		}
	}
}

// TestAWSLoadBalancerControllerWithDefaultLoadBalancerClass tests the basic happy flow for NLB.
// "service.k8s.aws/nlb" load balancer class is used as the default for
// the service reconciliation done by aws-load-balancer-controller.
func TestAWSLoadBalancerControllerWithDefaultLoadBalancerClass(t *testing.T) {
	testWorkloadNamespace := "aws-load-balancer-test-default-lb-class"
	t.Logf("Creating test namespace %q", testWorkloadNamespace)
	echoNs := createTestNamespace(t, testWorkloadNamespace)
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, echoNs, defaultTimeout)
	}()

	t.Log("Creating aws load balancer controller instance")

	alb := newALBCBuilder().withRoleARNIf(stsModeRequested(), controllerRoleARN).build()
	if err := kubeClient.Create(context.TODO(), alb); err != nil {
		t.Fatalf("failed to create aws load balancer controller: %v", err)
	}
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, alb, defaultTimeout)
	}()

	expected := []appsv1.DeploymentCondition{
		{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
	}
	deploymentName := types.NamespacedName{Name: "aws-load-balancer-controller-cluster", Namespace: "aws-load-balancer-operator"}
	if err := waitForDeploymentStatusCondition(context.TODO(), t, kubeClient, defaultTimeout, deploymentName, expected...); err != nil {
		t.Fatalf("did not get expected available condition for deployment: %v", err)
	}

	t.Logf("Creating test workload in %q namespace", testWorkloadNamespace)
	customize := func(svc *corev1.Service) {
		svc.Spec.Type = corev1.ServiceTypeLoadBalancer
		svc.Spec.LoadBalancerClass = ptr.To[string]("service.k8s.aws/nlb")
		svc.Annotations = map[string]string{
			"service.beta.kubernetes.io/aws-load-balancer-scheme": "internet-facing",
		}
		// by default ALBC uses instance target type if there is LoadBalancerClass
	}
	echoSvc := createTestWorkloadWithCustomize(t, testWorkloadNamespace, customize)
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, echoSvc, defaultTimeout)
	}()

	address, err := getService(context.TODO(), t, kubeClient, defaultTimeout, types.NamespacedName{
		Namespace: echoSvc.Namespace,
		Name:      echoSvc.Name,
	})
	if err != nil {
		t.Fatalf("did not get load balancer address for service: %v", err)
	}

	t.Logf("Testing aws network load balancer for ingress traffic at address %s", address)
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s", address), nil)
	if err != nil {
		t.Fatalf("failed to build client request: %v", err)
	}

	err = waitForHTTPClientCondition(context.TODO(), t, &httpClient, req, 5*time.Second, defaultTimeout, func(r *http.Response) bool {
		return r.StatusCode == http.StatusOK
	})
	if err != nil {
		t.Fatalf("failed to verify condition with external client: %v", err)
	}
}

// TestAWSLoadBalancerControllerWithInternalNLB tests the happy flow for internal NLB.
// "service.k8s.aws/nlb" load balancer class is used as the default for
// the service reconciliation done by aws-load-balancer-controller.
func TestAWSLoadBalancerControllerWithInternalNLB(t *testing.T) {
	testWorkloadNamespace := "aws-load-balancer-test-internal-nlb"
	t.Logf("Creating test namespace %q", testWorkloadNamespace)
	echoNs := createTestNamespace(t, testWorkloadNamespace)
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, echoNs, defaultTimeout)
	}()

	t.Log("Creating aws load balancer controller instance")
	alb := newALBCBuilder().withRoleARNIf(stsModeRequested(), controllerRoleARN).build()
	if err := kubeClient.Create(context.TODO(), alb); err != nil {
		t.Fatalf("failed to create aws load balancer controller: %v", err)
	}
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, alb, defaultTimeout)
	}()

	expected := []appsv1.DeploymentCondition{
		{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
	}
	deploymentName := types.NamespacedName{Name: "aws-load-balancer-controller-cluster", Namespace: "aws-load-balancer-operator"}
	if err := waitForDeploymentStatusCondition(context.TODO(), t, kubeClient, defaultTimeout, deploymentName, expected...); err != nil {
		t.Fatalf("did not get expected available condition for deployment: %v", err)
	}

	t.Logf("Creating test workload in %q namespace", testWorkloadNamespace)
	customize := func(svc *corev1.Service) {
		svc.Spec.Type = corev1.ServiceTypeLoadBalancer
		svc.Spec.LoadBalancerClass = ptr.To[string]("service.k8s.aws/nlb")
		// by default ALBC uses instance target type if there is LoadBalancerClass
		// by default ALBC creates internal NLB
	}
	echoSvc := createTestWorkloadWithCustomize(t, testWorkloadNamespace, customize)
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, echoSvc, defaultTimeout)
	}()

	address, err := getService(context.TODO(), t, kubeClient, defaultTimeout, types.NamespacedName{
		Namespace: echoSvc.Namespace,
		Name:      echoSvc.Name,
	})
	if err != nil {
		t.Fatalf("did not get load balancer address for service: %v", err)
	}

	i := 0
	t.Logf("Testing aws network load balancer for ingress traffic at address %s", address)
	err = wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 10*time.Minute, true, func(ctx context.Context) (bool, error) {
		clientPod := buildCurlPod(fmt.Sprintf("clientpod-%d", i), testWorkloadNamespace, "", address)
		i++
		if err := kubeClient.Create(context.TODO(), clientPod); err != nil {
			t.Logf("failed to create client pod %s/%s: %v (retrying)", clientPod.Namespace, clientPod.Name, err)
			return false, nil
		}

		if phase, err := waitForPodPhases(context.TODO(), t, kubeClient, defaultTimeout, types.NamespacedName{
			Namespace: clientPod.Namespace,
			Name:      clientPod.Name,
		}, corev1.PodSucceeded, corev1.PodFailed); err != nil {
			t.Logf("timed out waiting for client pod to become completed (retrying)")
			return false, nil
		} else {
			if phase == corev1.PodFailed {
				return false, nil
			}
		}
		return true, nil
	})
	if err != nil {
		t.Fatalf("failed to receive successful response: %v", err)
	}
}

// TestAWSLoadBalancerControllerWithExternalTypeNLBAndNonStandardPort tests the NLB flow
// which uses the legacy "service.beta.kubernetes.io/aws-load-balancer-type" annotation as well as
// the usage of the service port different from the standard HTTP (80).
func TestAWSLoadBalancerControllerWithExternalTypeNLBAndNonStandardPort(t *testing.T) {
	testWorkloadNamespace := "aws-load-balancer-test-lb-nonstd-port"
	t.Logf("Creating test namespace %q", testWorkloadNamespace)
	echoNs := createTestNamespace(t, testWorkloadNamespace)
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, echoNs, defaultTimeout)
	}()

	t.Log("Creating aws load balancer controller instance")
	alb := newALBCBuilder().withRoleARNIf(stsModeRequested(), controllerRoleARN).build()
	if err := kubeClient.Create(context.TODO(), alb); err != nil {
		t.Fatalf("failed to create aws load balancer controller: %v", err)
	}
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, alb, defaultTimeout)
	}()

	expected := []appsv1.DeploymentCondition{
		{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
	}
	deploymentName := types.NamespacedName{Name: "aws-load-balancer-controller-cluster", Namespace: "aws-load-balancer-operator"}
	if err := waitForDeploymentStatusCondition(context.TODO(), t, kubeClient, defaultTimeout, deploymentName, expected...); err != nil {
		t.Fatalf("did not get expected available condition for deployment: %v", err)
	}

	t.Logf("Creating test workload in %q namespace", testWorkloadNamespace)
	nonStandardPort := int32(8880)
	customize := func(svc *corev1.Service) {
		svc.Spec.Type = corev1.ServiceTypeLoadBalancer
		svc.Spec.Ports[0].Port = nonStandardPort
		svc.Annotations = map[string]string{
			"service.beta.kubernetes.io/aws-load-balancer-type":            "external",
			"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type": "instance",
			"service.beta.kubernetes.io/aws-load-balancer-scheme":          "internet-facing",
		}
	}
	echoSvc := createTestWorkloadWithCustomize(t, testWorkloadNamespace, customize)
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, echoSvc, defaultTimeout)
	}()

	address, err := getService(context.TODO(), t, kubeClient, defaultTimeout, types.NamespacedName{
		Namespace: echoSvc.Namespace,
		Name:      echoSvc.Name,
	})
	if err != nil {
		t.Fatalf("did not get load balancer address for service: %v", err)
	}

	t.Logf("Testing aws network load balancer for ingress traffic at address %s", address)
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s:%d", address, nonStandardPort), nil)
	if err != nil {
		t.Fatalf("failed to build client request: %v", err)
	}

	err = waitForHTTPClientCondition(context.TODO(), t, &httpClient, req, 5*time.Second, defaultTimeout, func(r *http.Response) bool {
		return r.StatusCode == http.StatusOK
	})
	if err != nil {
		t.Fatalf("failed to verify condition with external client: %v", err)
	}
}

// TestAWSLoadBalancerControllerOpenAPIValidation tests validations added to AWSLoadBalancerController CRD.
func TestAWSLoadBalancerControllerOpenAPIValidation(t *testing.T) {
	alb1 := newALBCBuilder().withCredSecret("dummy").withRoleARN("arn:aws:iam::777777777777:role/test").build()
	if err := kubeClient.Create(context.TODO(), alb1); err == nil {
		defer func() {
			waitForDeletion(context.TODO(), t, kubeClient, alb1, defaultTimeout)
		}()
		t.Fatalf("didn't fail to create aws load balancer controller with conflicting credentials")
	}

	alb2 := newALBCBuilder().withRoleARN("arn:aws:iam::777777777777:rolex/test").build()
	if err := kubeClient.Create(context.TODO(), alb2); err == nil {
		defer func() {
			waitForDeletion(context.TODO(), t, kubeClient, alb2, defaultTimeout)
		}()
		t.Fatalf("didn't fail to create aws load balancer controller with invalid role arn")
	}
}

// TestAWSLoadBalancerControllerUserTags verifies that the user-defined tags are correctly applied.
// It verifies that the controller deployment's `--default-tags` argument is updated
// to include the tags from both the controller spec and the infrastructure status,
// giving precedence to the controller spec in case of key conflicts.
func TestAWSLoadBalancerControllerUserTags(t *testing.T) {
	managed, err := isManagedServiceCluster(context.TODO(), kubeClientSet)
	if err != nil {
		t.Fatalf("failed to check managed cluster %v", err)
	}
	if managed {
		t.Skip("Infrastructure status cannot be directly updated on managed cluster, skipping...")
	}

	testWorkloadNamespace := "aws-load-balancer-test-user-tags"
	t.Logf("Creating test namespace %q", testWorkloadNamespace)
	echoNs := createTestNamespace(t, testWorkloadNamespace)
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, echoNs, defaultTimeout)
	}()

	t.Log("Creating aws load balancer controller instance with default ingress class and user tags")

	// create albc with additional resource tags added in albc spec
	albc := newALBCBuilder().
		withRoleARNIf(stsModeRequested(), controllerRoleARN).
		withResourceTags(map[string]string{
			"op-key1":       "op-value1",
			"conflict-key1": "op-value2",
			"conflict-key2": "op-value3",
		}).
		build()

	if err := kubeClient.Create(context.TODO(), albc); err != nil {
		t.Fatalf("failed to create aws load balancer controller: %v", err)
	}
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, albc, defaultTimeout)
	}()

	expected := []appsv1.DeploymentCondition{
		{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
	}
	deploymentName := types.NamespacedName{Name: "aws-load-balancer-controller-cluster", Namespace: "aws-load-balancer-operator"}
	if err := waitForDeploymentStatusCondition(context.TODO(), t, kubeClient, defaultTimeout, deploymentName, expected...); err != nil {
		t.Fatalf("did not get expected available condition for deployment: %v", err)
	}

	dep := &appsv1.Deployment{}
	if err := kubeClient.Get(context.TODO(), deploymentName, dep); err != nil {
		t.Fatalf("failed to get deployment %s: %v", deploymentName.Name, err)
	}
	depGeneration := dep.Generation

	t.Logf("Creating test workload in %q namespace", testWorkloadNamespace)
	echoSvc := createTestWorkload(t, testWorkloadNamespace)

	t.Log("Creating Ingress Resource with default ingress class")
	ingName := types.NamespacedName{Name: "echoserver", Namespace: testWorkloadNamespace}
	ingAnnotations := map[string]string{
		"alb.ingress.kubernetes.io/scheme":      "internet-facing",
		"alb.ingress.kubernetes.io/target-type": "instance",
	}
	echoIng := buildEchoIngress(ingName, "alb", ingAnnotations, echoSvc)
	err = retry.OnError(defaultRetryPolicy,
		func(err error) bool {
			if errors.IsAlreadyExists(err) {
				return false
			}
			t.Logf("retrying creation of echo ingress due to %v", err)
			return true
		},
		func() error { return kubeClient.Create(context.TODO(), echoIng) })
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure echo ingress %s: %v", echoIng.Name, err)
	}
	defer func() {
		waitForDeletion(context.TODO(), t, kubeClient, echoIng, defaultTimeout)
	}()

	_, err = getIngress(context.TODO(), t, kubeClient, defaultTimeout, ingName)
	if err != nil {
		t.Fatalf("did not get load balancer hostname for ingress: %v", err)
	}

	// Save a copy of the original infra Config, to revert changes before exiting.
	originalInfra := infra.DeepCopy()
	defer func() {
		if err := updateInfrastructureConfigStatusWithRetryOnConflict(t, 5*time.Minute, kubeClient, func(infra *configv1.Infrastructure) {
			infra.Status = originalInfra.Status
		}); err != nil {
			t.Fatalf("Unable to revert the infrastructure changes: %v", err)
		}
	}()

	// Update infrastructure status with initialInfraTags
	initialInfraTags := []configv1.AWSResourceTag{
		{Key: "plat-key1", Value: "plat-value1"},
		{Key: "conflict-key1", Value: "plat-value2"},
		{Key: "conflict-key2", Value: "plat-value3"},
	}
	t.Logf("Updating cluster infrastructure config with resource tags: %v", initialInfraTags)
	err = updateInfrastructureConfigStatusWithRetryOnConflict(t, 5*time.Minute, kubeClient, func(infra *configv1.Infrastructure) {
		if infra.Status.PlatformStatus == nil {
			infra.Status.PlatformStatus = &configv1.PlatformStatus{}
		}
		if infra.Status.PlatformStatus.AWS == nil {
			infra.Status.PlatformStatus.AWS = &configv1.AWSPlatformStatus{}
		}
		infra.Status.PlatformStatus.AWS.ResourceTags = initialInfraTags
	})
	if err != nil {
		t.Errorf("failed to update infrastructure status: %v", err)
	}

	// wait for the deployment to restart and become available
	if err := waitForDeploymentRollout(context.TODO(), t, kubeClient, defaultTimeout, deploymentName, depGeneration); err != nil {
		t.Fatalf("deployment did not roll out within timeout: %v", err)
	}
	if err := waitForDeploymentStatusCondition(context.TODO(), t, kubeClient, defaultTimeout, deploymentName, expected...); err != nil {
		t.Fatalf("did not get expected available condition for deployment: %v", err)
	}

	// get the updated deployment
	if err := kubeClient.Get(context.TODO(), deploymentName, dep); err != nil {
		t.Fatalf("failed to get deployment %s: %v", deploymentName.Name, err)
	}
	depGeneration = dep.Generation

	t.Logf("Testing aws tags present in alb instance and infra status (initialInfraTags)")
	expectedTags := map[string]string{
		"conflict-key1": "op-value2", "conflict-key2": "op-value3", "op-key1": "op-value1", "plat-key1": "plat-value1",
	}
	// Check `--default-tags` container argument
	assertContainerArgFromDeployment(t, dep, awsLoadBalancerControllerContainerName, "--default-tags", convertTagsMapToString(expectedTags))
	// Check the actual AWS ELB instance
	assertELBbyTags(t, expectedTags, []string{})

	// Update the status again, removing one tag.
	updatedInfraTags := []configv1.AWSResourceTag{
		{Key: "conflict-key1", Value: "plat-value2"},
		{Key: "conflict-key2", Value: "plat-value3"},
	}
	t.Logf("Updating AWS ResourceTags in the cluster infrastructure config: %v", updatedInfraTags)
	err = updateInfrastructureConfigStatusWithRetryOnConflict(t, 5*time.Minute, kubeClient, func(infra *configv1.Infrastructure) {
		infra.Status.PlatformStatus.AWS.ResourceTags = updatedInfraTags
	})
	if err != nil {
		t.Errorf("failed to update infrastructure status: %v", err)
	}

	// wait for the deployment to restart and become available
	if err := waitForDeploymentRollout(context.TODO(), t, kubeClient, defaultTimeout, deploymentName, depGeneration); err != nil {
		t.Fatalf("deployment did not roll out within timeout: %v", err)
	}
	if err := waitForDeploymentStatusCondition(context.TODO(), t, kubeClient, defaultTimeout, deploymentName, expected...); err != nil {
		t.Fatalf("did not get expected available condition for deployment: %v", err)
	}

	// get the updated deployment
	if err := kubeClient.Get(context.TODO(), deploymentName, dep); err != nil {
		t.Fatalf("failed to get deployment %s: %v", deploymentName.Name, err)
	}

	t.Logf("Testing aws tags present in alb instance and infra status (updatedInfraTags)")
	expectedTags = map[string]string{
		"conflict-key1": "op-value2", "conflict-key2": "op-value3", "op-key1": "op-value1",
	}
	expectedAbsentTags := []string{"plat-key1"}
	// Check `--default-tags` container argument
	assertContainerArgFromDeployment(t, dep, awsLoadBalancerControllerContainerName, "--default-tags", convertTagsMapToString(expectedTags))
	// Check the actual AWS ELB instance
	assertELBbyTags(t, expectedTags, expectedAbsentTags)
}

// ensureCredentialsRequest creates CredentialsRequest to provision a secret with the cloud credentials required by this e2e test.
func ensureCredentialsRequest(secret types.NamespacedName) error {
	providerSpec, err := cco.Codec.EncodeProviderSpec(&cco.AWSProviderSpec{
		StatementEntries: []cco.StatementEntry{
			{
				Action:   []string{"wafv2:CreateWebACL", "wafv2:DeleteWebACL", "wafv2:ListWebACLs"},
				Effect:   "Allow",
				Resource: "*",
			},
			{
				Action:   []string{"waf-regional:GetChangeToken", "waf-regional:CreateWebACL", "waf-regional:DeleteWebACL", "waf-regional:ListWebACLs"},
				Effect:   "Allow",
				Resource: "*",
			},
			{
				Action:   []string{"tag:GetResources"},
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
			Name:      "aws-load-balancer-operator-e2e",
			Namespace: "openshift-cloud-credential-operator",
		},
		Spec: cco.CredentialsRequestSpec{
			SecretRef: corev1.ObjectReference{
				Name:      secret.Name,
				Namespace: secret.Namespace,
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

func createTestWorkload(t *testing.T, namespace string) *corev1.Service {
	t.Helper()
	return createTestWorkloadWithCustomize(t, namespace, nil)
}

func createTestWorkloadWithCustomize(t *testing.T, namespace string, customize func(*corev1.Service)) *corev1.Service {
	t.Helper()
	echopod := buildEchoPod("echoserver", namespace)
	err := kubeClient.Create(context.TODO(), echopod)
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to create pod %s: %v", echopod.Name, err)
	}

	echosvc := buildEchoService("echoserver", namespace)
	if customize != nil {
		customize(echosvc)
	}
	err = kubeClient.Create(context.TODO(), echosvc)
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to create service %s: %v", echosvc.Name, err)
	}

	return echosvc
}

func createTestNamespace(t *testing.T, namespace string) *corev1.Namespace {
	t.Helper()
	ns := &corev1.Namespace{ObjectMeta: v1.ObjectMeta{Name: namespace}}
	err := kubeClient.Create(context.TODO(), ns)
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to create namespace %s: %v", namespace, err)
	}
	return ns
}

// stsModeRequested returns true if the specified e2e platform is STS enabled.
func stsModeRequested() bool {
	switch strings.ToUpper(os.Getenv(e2ePlatformVarName)) {
	case "OCPSTS", "ROSASTS", "ROSA" /*ROSA uses STS mode by default*/ :
		return true
	}
	return false
}

// extractArg extracts the value of a specific argument from a list of container arguments.
// Returns an error if the argument is present but malformed, or if not found.
func extractArg(args []string, argKey string) (string, error) {
	for _, arg := range args {
		if strings.HasPrefix(arg, argKey+"=") {
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) != 2 {
				return "", fmt.Errorf("invalid format for argument %q: %q", argKey, arg)
			}
			return parts[1], nil
		}
	}
	return "", fmt.Errorf("argument %q not found", argKey)
}

// assertContainerArgFromDeployment asserts that a container in a deployment has a
// specific argument with an expected value.
func assertContainerArgFromDeployment(t *testing.T, dep *appsv1.Deployment, containerName, argKey, expectedArgValue string) {
	t.Helper()
	t.Logf("Asserting container %q has argument %q with value %q", containerName, argKey, expectedArgValue)
	for _, c := range dep.Spec.Template.Spec.Containers {
		if c.Name == containerName {
			actualArgValue, err := extractArg(c.Args, argKey)
			if err != nil {
				t.Fatalf("failed to extract argument %q for container %q: %v", argKey, containerName, err)
			}

			if actualArgValue != expectedArgValue {
				t.Fatalf("container %q, argument %q: expected value %q, but got %q", containerName, argKey, expectedArgValue, actualArgValue)
			}
			return
		}
	}
	t.Fatalf("container %q not found in deployment", containerName)
}

// assertELBbyTags asserts that an ELB instance has the expected tags and doesn't have the expected absent tags
func assertELBbyTags(t *testing.T, expectedTags map[string]string, expectedAbsentTags []string) {
	t.Helper()
	t.Logf("Asserting ELBs by tags %v", expectedTags)

	rgtClient := resourcegroupstaggingapi.NewFromConfig(cfg)

	err := wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 5*time.Minute, false, func(ctx context.Context) (bool, error) {
		lbARNsAndTags, err := getLoadBalancerARNsByTags(ctx, rgtClient, expectedTags)
		if err != nil {
			return false, fmt.Errorf("unable to get ELBs for %v tags: %v", expectedTags, err)
		}
		// There must be exactly 1 ELB with the given tags
		if len(lbARNsAndTags) != 1 {
			t.Logf("expected single ELB with %v tags, but got %d (with arns and tags: %v), retrying... ", expectedTags, len(lbARNsAndTags), lbARNsAndTags)
			return false, nil
		}

		// expectedAbsentTags must not exist
		for _, absentTagKey := range expectedAbsentTags {
			for arn, tags := range lbARNsAndTags {
				if _, exists := tags[absentTagKey]; exists {
					t.Logf("found unexpected tag %s on ELB %s (with tags: %v), retrying...", absentTagKey, arn, tags)
					return false, nil
				}
			}
		}

		t.Logf("found ELB with %v tags (with arn and tags: %v)", expectedTags, lbARNsAndTags)
		return true, nil
	})

	if err != nil {
		t.Fatalf("timed out waiting for %v tags to match an ELB: %v", expectedTags, err)
	}
}

// This logic was inspired by
// https://github.com/openshift/origin/pull/29216/files#diff-35a89a7a7362642eebb559fb8564e857b00d6f7dd6322c3adabaf1adbd609d35R2267-R2278
// implementation.
func isManagedServiceCluster(ctx context.Context, adminClient kubernetes.Interface) (bool, error) {
	_, err := adminClient.CoreV1().Namespaces().Get(ctx, "openshift-backplane", v1.GetOptions{})
	if err == nil {
		return true, nil
	}

	if !errors.IsNotFound(err) {
		return false, err
	}

	return false, nil
}

// convertTagsMapToString converts a map of tags into a sorted comma-separated string.
// Eeach key-value pair is formatted as "key=value".
func convertTagsMapToString(tagsMap map[string]string) string {
	var tags []string
	for key, value := range tagsMap {
		tags = append(tags, fmt.Sprintf("%s=%s", key, value))
	}

	sort.Strings(tags)
	return strings.Join(tags, ",")
}
