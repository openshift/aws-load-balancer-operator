//go:build e2e
// +build e2e

package e2e

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
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
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/pointer"

	"github.com/aws/aws-sdk-go-v2/aws"
	waf "github.com/aws/aws-sdk-go-v2/service/wafregional"
	waftypes "github.com/aws/aws-sdk-go-v2/service/wafregional/types"
	"github.com/aws/aws-sdk-go-v2/service/wafv2"
	wafv2types "github.com/aws/aws-sdk-go-v2/service/wafv2/types"
	configv1 "github.com/openshift/api/config/v1"
	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	elbv1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
)

var (
	cfg                aws.Config
	kubeClient         client.Client
	kubeClientSet      *kubernetes.Clientset
	scheme             = kscheme.Scheme
	infraConfig        configv1.Infrastructure
	operatorName       = "aws-load-balancer-operator-controller-manager"
	operatorNamespace  = "aws-load-balancer-operator"
	defaultTimeout     = 15 * time.Minute
	defaultRetryPolicy = wait.Backoff{
		Duration: 5 * time.Second,
		Factor:   1.0,
		Jitter:   1.0,
		Steps:    10,
	}
	httpClient = http.Client{Timeout: 5 * time.Second}
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
	utilruntime.Must(elbv1beta1.AddToScheme(scheme))
}

func newAWSLoadBalancerController(name types.NamespacedName, ingressClass string, addons []albo.AWSAddon) albo.AWSLoadBalancerController {
	return albo.AWSLoadBalancerController{
		ObjectMeta: v1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
		},
		Spec: albo.AWSLoadBalancerControllerSpec{
			SubnetTagging: albo.AutoSubnetTaggingPolicy,
			IngressClass:  ingressClass,
			EnabledAddons: addons,
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

	err = ensureCredentialsRequest()
	if err != nil {
		fmt.Printf("failed to create credentialsrequest for e2e: %s\n", err)
		os.Exit(1)
	}

	var infra configv1.Infrastructure
	clusterInfrastructureName := types.NamespacedName{Name: "cluster"}
	err = kubeClient.Get(context.TODO(), clusterInfrastructureName, &infra)
	if err != nil {
		fmt.Printf("failed to fetch infrastructure: %v", err)
		os.Exit(1)
	}

	if infra.Status.PlatformStatus == nil || infra.Status.PlatformStatus.AWS == nil || infra.Status.PlatformStatus.AWS.Region == "" {
		fmt.Printf("could not get AWS region from Infrastructure %q status", clusterInfrastructureName.Name)
		os.Exit(1)
	}

	secretName := types.NamespacedName{
		Name:      "aws-load-balancer-operator-e2e",
		Namespace: operatorNamespace,
	}
	cfg, err = awsCredentials(kubeClient, infra.Status.PlatformStatus.AWS.Region, secretName)
	if err != nil {
		fmt.Printf("failed to load aws config %v", err)
		os.Exit(1)
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
	if err := waitForDeploymentStatusCondition(t, kubeClient, defaultTimeout, name, expected...); err != nil {
		t.Fatalf("Did not get expected available condition: %v", err)
	}
}

func TestOperatorAvailable(t *testing.T) {
	getOperator(t)
}

// TestAWSLoadBalancerControllerWithDefaultIngressClass tests the basic happy flow for the operator, mostly
// using the default values.
func TestAWSLoadBalancerControllerWithDefaultIngressClass(t *testing.T) {
	t.Log("Creating aws load balancer controller instance with default ingress class")

	name := types.NamespacedName{Name: "cluster", Namespace: "aws-load-balancer-operator"}
	alb := newAWSLoadBalancerController(name, "alb", []albo.AWSAddon{})
	if err := kubeClient.Create(context.TODO(), &alb); err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to create aws load balancer controller %q: %v", name, err)
	}
	defer func() {
		waitForDeletion(t, kubeClient, &alb, defaultTimeout)
	}()

	expected := []appsv1.DeploymentCondition{
		{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
	}
	deploymentName := types.NamespacedName{Name: "aws-load-balancer-controller-cluster", Namespace: "aws-load-balancer-operator"}
	if err := waitForDeploymentStatusCondition(t, kubeClient, defaultTimeout, deploymentName, expected...); err != nil {
		t.Fatalf("did not get expected available condition for deployment: %v", err)
	}

	testWorkloadNamespace := "aws-load-balancer-test-default-ing"
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
			t.Logf("retrying creation of echo ingress due to %v", err)
			return !errors.IsAlreadyExists(err)
		},
		func() error { return kubeClient.Create(context.TODO(), echoIng) })
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure echo ingress %s: %v", echoIng.Name, err)
	}
	defer func() {
		waitForDeletion(t, kubeClient, echoIng, defaultTimeout)
	}()

	var address string
	if address, err = getIngress(t, kubeClient, defaultTimeout, ingName); err != nil {
		t.Fatalf("did not get expected available condition for ingress: %v", err)
	}

	t.Logf("Testing aws load balancer for ingress traffic at address %s", address)
	for _, rule := range echoIng.Spec.Rules {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s", address), nil)
		if err != nil {
			t.Fatalf("failed to build client request: %v", err)
		}
		req.Host = rule.Host

		err = waitForHTTPClientCondition(t, &httpClient, req, 5*time.Second, defaultTimeout, func(r *http.Response) bool {
			return r.StatusCode == http.StatusOK
		})
		if err != nil {
			t.Fatalf("failed verify condition with external client: %v", err)
		}
	}
}

func TestAWSLoadBalancerControllerWithCustomIngressClass(t *testing.T) {
	t.Log("Creating a custom ingress class")
	ingclassName := types.NamespacedName{Name: "custom-alb", Namespace: "aws-load-balancer-operator"}
	ingclass := buildIngressClass(ingclassName, "ingress.k8s.aws/alb")
	if err := kubeClient.Create(context.TODO(), ingclass); err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure custom ingress class %q: %v", ingclass.Name, err)
	}
	defer func() {
		waitForDeletion(t, kubeClient, ingclass, defaultTimeout)
	}()

	t.Log("Creating aws load balancer controller instance with custom ingress class")

	name := types.NamespacedName{Name: "cluster", Namespace: "aws-load-balancer-operator"}
	alb := newAWSLoadBalancerController(name, ingclassName.Name, []albo.AWSAddon{})
	if err := kubeClient.Create(context.TODO(), &alb); err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to create aws load balancer controller %q: %v", name, err)
	}
	defer func() {
		waitForDeletion(t, kubeClient, &alb, defaultTimeout)
	}()

	expected := []appsv1.DeploymentCondition{
		{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
	}
	deploymentName := types.NamespacedName{Name: "aws-load-balancer-controller-cluster", Namespace: "aws-load-balancer-operator"}
	if err := waitForDeploymentStatusCondition(t, kubeClient, defaultTimeout, deploymentName, expected...); err != nil {
		t.Fatalf("did not get expected available condition for deployment: %v", err)
	}

	testWorkloadNamespace := "aws-load-balancer-test-custom-ing"
	echoSvc := createTestWorkload(t, testWorkloadNamespace)

	t.Log("Creating Ingress Resource with custom ingress class")
	ingName := types.NamespacedName{Name: "echoserver", Namespace: testWorkloadNamespace}
	ingAnnotations := map[string]string{
		"alb.ingress.kubernetes.io/scheme":      "internet-facing",
		"alb.ingress.kubernetes.io/target-type": "instance",
	}
	echoIng := buildEchoIngress(ingName, ingclass.Name, ingAnnotations, echoSvc)
	err := retry.OnError(defaultRetryPolicy,
		func(err error) bool {
			t.Logf("retrying creation of echo ingress due to %v", err)
			return !errors.IsAlreadyExists(err)
		},
		func() error { return kubeClient.Create(context.TODO(), echoIng) })
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure echo ingress %s: %v", echoIng.Name, err)
	}
	defer func() {
		waitForDeletion(t, kubeClient, echoIng, defaultTimeout)
	}()

	var address string
	if address, err = getIngress(t, kubeClient, defaultTimeout, ingName); err != nil {
		t.Fatalf("did not get expected available condition for ingress: %v", err)
	}

	t.Logf("Testing aws load balancer for ingress traffic at address %s", address)
	for _, rule := range echoIng.Spec.Rules {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s", address), nil)
		if err != nil {
			t.Fatalf("failed to build client request: %v", err)
		}
		req.Host = rule.Host

		err = waitForHTTPClientCondition(t, &httpClient, req, 5*time.Second, defaultTimeout, func(r *http.Response) bool {
			return r.StatusCode == http.StatusOK
		})
		if err != nil {
			t.Fatalf("failed verify condition with external client: %v", err)
		}
	}
}

func TestAWSLoadBalancerControllerWithInternalLoadBalancer(t *testing.T) {
	t.Log("Creating aws load balancer controller instance with default ingress class")

	name := types.NamespacedName{Name: "cluster", Namespace: "aws-load-balancer-operator"}
	alb := newAWSLoadBalancerController(name, "alb", []albo.AWSAddon{})
	if err := kubeClient.Create(context.TODO(), &alb); err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to create aws load balancer controller %q: %v", name, err)
	}
	defer func() {
		waitForDeletion(t, kubeClient, &alb, defaultTimeout)
	}()

	expected := []appsv1.DeploymentCondition{
		{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
	}
	deploymentName := types.NamespacedName{Name: "aws-load-balancer-controller-cluster", Namespace: "aws-load-balancer-operator"}
	if err := waitForDeploymentStatusCondition(t, kubeClient, defaultTimeout, deploymentName, expected...); err != nil {
		t.Fatalf("did not get expected available condition for deployment: %v", err)
	}

	testWorkloadNamespace := "aws-load-balancer-test-internal-ing"
	echoSvc := createTestWorkload(t, testWorkloadNamespace)

	t.Log("Creating Internal Ingress Resource with default ingress class")
	ingName := types.NamespacedName{Name: "echoserver", Namespace: testWorkloadNamespace}
	ingAnnotations := map[string]string{
		"alb.ingress.kubernetes.io/scheme":      "internal",
		"alb.ingress.kubernetes.io/target-type": "instance",
	}
	echoIng := buildEchoIngress(ingName, "alb", ingAnnotations, echoSvc)
	err := retry.OnError(defaultRetryPolicy,
		func(err error) bool {
			t.Logf("retrying creation of echo ingress due to %v", err)
			return !errors.IsAlreadyExists(err)
		},
		func() error { return kubeClient.Create(context.TODO(), echoIng) })
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure echo ingress %s: %v", echoIng.Name, err)
	}
	defer func() {
		waitForDeletion(t, kubeClient, echoIng, defaultTimeout)
	}()

	var address string
	if address, err = getIngress(t, kubeClient, defaultTimeout, ingName); err != nil {
		t.Fatalf("did not get expected available condition for ingress: %v", err)
	}

	t.Logf("Testing aws load balancer for ingress traffic at address %s", address)
	for i, rule := range echoIng.Spec.Rules {
		clientPod := buildCurlPod(fmt.Sprintf("clientpod-%d", i), testWorkloadNamespace, rule.Host, address)
		if err := kubeClient.Create(context.TODO(), clientPod); err != nil {
			t.Fatalf("failed to create pod %s/%s: %v", clientPod.Namespace, clientPod.Name, err)
		}

		err = wait.PollImmediate(5*time.Second, 10*time.Minute, func() (bool, error) {
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

		waitForDeletion(t, kubeClient, clientPod, defaultTimeout)
	}
}

func TestAWSLoadBalancerControllerWithWAFv2(t *testing.T) {
	t.Log("Creating aws load balancer controller instance with default ingress class")

	name := types.NamespacedName{Name: "cluster", Namespace: "aws-load-balancer-operator"}
	alb := newAWSLoadBalancerController(name, "alb", []albo.AWSAddon{albo.AWSAddonWAFv2})
	if err := kubeClient.Create(context.TODO(), &alb); err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to create aws load balancer controller %q: %v", name, err)
	}
	defer func() {
		waitForDeletion(t, kubeClient, &alb, defaultTimeout)
	}()

	expected := []appsv1.DeploymentCondition{
		{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
	}
	deploymentName := types.NamespacedName{Name: "aws-load-balancer-controller-cluster", Namespace: "aws-load-balancer-operator"}
	if err := waitForDeploymentStatusCondition(t, kubeClient, defaultTimeout, deploymentName, expected...); err != nil {
		t.Fatalf("did not get expected available condition for deployment: %v", err)
	}

	testWorkloadNamespace := "aws-load-balancer-test-wafv2"
	echoSvc := createTestWorkload(t, testWorkloadNamespace)

	wafClient := wafv2.NewFromConfig(cfg)
	acl, err := wafClient.CreateWebACL(context.Background(), &wafv2.CreateWebACLInput{
		DefaultAction: &wafv2types.DefaultAction{Block: &wafv2types.BlockAction{}},
		Name:          aws.String("echoserver-acl"),
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
	defer func() {
		_, err = wafClient.DeleteWebACL(context.TODO(), &wafv2.DeleteWebACLInput{
			Id:        aws.String(*acl.Summary.Id),
			Name:      aws.String("echoserver-acl"),
			LockToken: acl.Summary.LockToken,
			Scope:     wafv2types.ScopeRegional,
		})
		if err != nil {
			t.Fatalf("failed to delete aws wafv2 acl due to %v", err)
		}
	}()

	t.Log("Creating Ingress Resource with default ingress class")
	ingName := types.NamespacedName{Name: "echoserver", Namespace: testWorkloadNamespace}
	ingAnnotations := map[string]string{
		"alb.ingress.kubernetes.io/scheme":        "internet-facing",
		"alb.ingress.kubernetes.io/target-type":   "instance",
		"alb.ingress.kubernetes.io/wafv2-acl-arn": *acl.Summary.ARN,
	}
	echoIng := buildEchoIngress(ingName, "alb", ingAnnotations, echoSvc)
	err = retry.OnError(defaultRetryPolicy,
		func(err error) bool {
			t.Logf("retrying creation of echo ingress due to %v", err)
			return !errors.IsAlreadyExists(err)
		},
		func() error { return kubeClient.Create(context.TODO(), echoIng) })
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure echo ingress %s: %v", echoIng.Name, err)
	}
	defer func() {
		waitForDeletion(t, kubeClient, echoIng, defaultTimeout)
	}()

	var address string
	if address, err = getIngress(t, kubeClient, defaultTimeout, ingName); err != nil {
		t.Fatalf("did not get expected available condition for ingress: %v", err)
	}

	t.Logf("Testing aws load balancer for ingress traffic at address %s", address)
	for _, rule := range echoIng.Spec.Rules {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s", address), nil)
		if err != nil {
			t.Fatalf("failed to build client request: %v", err)
		}
		req.Host = rule.Host

		err = waitForHTTPClientCondition(t, &httpClient, req, 5*time.Second, defaultTimeout, func(r *http.Response) bool {
			return r.StatusCode == http.StatusForbidden
		})
		if err != nil {
			t.Fatalf("failed verify condition with external client: %v", err)
		}
	}
}

func TestAWSLoadBalancerControllerWithWAFRegional(t *testing.T) {
	t.Log("Creating aws load balancer controller instance with default ingress class")

	name := types.NamespacedName{Name: "cluster", Namespace: "aws-load-balancer-operator"}
	alb := newAWSLoadBalancerController(name, "alb", []albo.AWSAddon{albo.AWSAddonWAFv1})
	if err := kubeClient.Create(context.TODO(), &alb); err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to create aws load balancer controller %q: %v", name, err)
	}
	defer func() {
		waitForDeletion(t, kubeClient, &alb, defaultTimeout)
	}()

	expected := []appsv1.DeploymentCondition{
		{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
	}
	deploymentName := types.NamespacedName{Name: "aws-load-balancer-controller-cluster", Namespace: "aws-load-balancer-operator"}
	if err := waitForDeploymentStatusCondition(t, kubeClient, defaultTimeout, deploymentName, expected...); err != nil {
		t.Fatalf("did not get expected available condition for deployment: %v", err)
	}

	testWorkloadNamespace := "aws-load-balancer-test-wafregional"
	echoSvc := createTestWorkload(t, testWorkloadNamespace)

	wafClient := waf.NewFromConfig(cfg)

	token, err := wafClient.GetChangeToken(context.TODO(), &waf.GetChangeTokenInput{})
	if err != nil {
		t.Fatalf("failed to get change token for waf regional classic %v", err)
	}
	acl, err := wafClient.CreateWebACL(context.TODO(), &waf.CreateWebACLInput{
		DefaultAction: &waftypes.WafAction{Type: waftypes.WafActionTypeBlock},
		MetricName:    aws.String("echoserverclassicacl"),
		Name:          aws.String("echoserverclassicacl"),
		ChangeToken:   token.ChangeToken,
	})
	if err != nil {
		t.Fatalf("failed to create aws waf regional acl due to %v", err)
	}
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

	t.Log("Creating Ingress Resource with default ingress class")
	ingName := types.NamespacedName{Name: "echoserver", Namespace: testWorkloadNamespace}
	ingAnnotations := map[string]string{
		"alb.ingress.kubernetes.io/scheme":      "internet-facing",
		"alb.ingress.kubernetes.io/target-type": "instance",
		"alb.ingress.kubernetes.io/waf-acl-id":  *acl.WebACL.WebACLId,
	}
	echoIng := buildEchoIngress(ingName, "alb", ingAnnotations, echoSvc)
	err = retry.OnError(defaultRetryPolicy,
		func(err error) bool {
			t.Logf("retrying creation of echo ingress due to %v", err)
			return !errors.IsAlreadyExists(err)
		},
		func() error { return kubeClient.Create(context.TODO(), echoIng) })
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure echo ingress %s: %v", echoIng.Name, err)
	}
	defer func() {
		waitForDeletion(t, kubeClient, echoIng, defaultTimeout)
	}()

	var address string
	if address, err = getIngress(t, kubeClient, 20*time.Minute, ingName); err != nil {
		t.Fatalf("did not get expected available condition for ingress: %v", err)
	}

	t.Logf("Testing aws load balancer for ingress traffic at address %s", address)
	for _, rule := range echoIng.Spec.Rules {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s", address), nil)
		if err != nil {
			t.Fatalf("failed to build client request: %v", err)
		}
		req.Host = rule.Host

		err = waitForHTTPClientCondition(t, &httpClient, req, 5*time.Second, defaultTimeout, func(r *http.Response) bool {
			return r.StatusCode == http.StatusForbidden
		})
		if err != nil {
			t.Fatalf("failed verify condition with external client: %v", err)
		}
	}
}

func ensureCredentialsRequest() error {
	codec, err := cco.NewCodec()
	if err != nil {
		return err
	}

	providerSpec, err := codec.EncodeProviderSpec(&cco.AWSProviderSpec{
		StatementEntries: []cco.StatementEntry{
			{
				Action:   []string{"wafv2:CreateWebACL", "wafv2:DeleteWebACL"},
				Effect:   "Allow",
				Resource: "*",
			},
			{
				Action:   []string{"waf-regional:GetChangeToken", "waf-regional:CreateWebACL", "waf-regional:DeleteWebACL"},
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
				Name:      "aws-load-balancer-operator-e2e",
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

func createTestWorkload(t *testing.T, namespace string) *corev1.Service {
	t.Logf("Ensuring test workload namespace %s", namespace)
	ns := &corev1.Namespace{ObjectMeta: v1.ObjectMeta{Name: namespace}}
	err := kubeClient.Create(context.TODO(), ns)
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure namespace %s: %v", namespace, err)
	}

	echopod := buildEchoPod("echoserver", namespace)
	err = kubeClient.Create(context.TODO(), echopod)
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure pod %s: %v", echopod.Name, err)
	}

	echosvc := buildEchoService("echoserver", namespace)
	err = kubeClient.Create(context.TODO(), echosvc)
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure service %s: %v", echosvc.Name, err)
	}

	return echosvc
}

func TestIngressGroup(t *testing.T) {
	ctx := context.TODO()
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

	if err := kubeClient.Create(ctx, ingressClassParams); err != nil {
		t.Fatalf("failed to create IngressClassParams %s: %v", ingressClassParams.Name, err)
	}
	defer func() {
		waitForDeletion(t, kubeClient, ingressClassParams, defaultTimeout)
	}()

	t.Log("Creating a custom ingress class")
	ingressClassName := types.NamespacedName{Name: "multi-ingress", Namespace: "aws-load-balancer-operator"}
	ingressClass := buildIngressClass(ingressClassName, "ingress.k8s.aws/alb")
	ingressClass.Spec.Parameters = &networkingv1.IngressClassParametersReference{
		APIGroup: pointer.String(elbv1beta1.GroupVersion.Group),
		Kind:     "IngressClassParams",
		Name:     ingressClassParams.Name,
	}
	if err := kubeClient.Create(context.TODO(), ingressClass); err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure custom ingress class %q: %v", ingressClass.Name, err)
	}
	defer func() {
		waitForDeletion(t, kubeClient, ingressClass, defaultTimeout)
	}()

	t.Log("Creating aws load balancer controller instance with custom ingress class")

	name := types.NamespacedName{Name: "cluster", Namespace: "aws-load-balancer-operator"}
	alb := newAWSLoadBalancerController(name, ingressClassName.Name, []albo.AWSAddon{})
	if err := kubeClient.Create(context.TODO(), &alb); err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to create aws load balancer controller %q: %v", name, err)
	}
	defer func() {
		waitForDeletion(t, kubeClient, &alb, defaultTimeout)
	}()

	expected := []appsv1.DeploymentCondition{
		{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
	}
	deploymentName := types.NamespacedName{Name: "aws-load-balancer-controller-cluster", Namespace: "aws-load-balancer-operator"}
	if err := waitForDeploymentStatusCondition(t, kubeClient, defaultTimeout, deploymentName, expected...); err != nil {
		t.Fatalf("did not get expected available condition for deployment: %v", err)
	}

	testWorkloadNamespace := "aws-load-balancer-test-custom-ing"
	echoSvc := createTestWorkload(t, testWorkloadNamespace)
	ingAnnotations := map[string]string{
		"alb.ingress.kubernetes.io/scheme": "internet-facing",
	}

	t.Log("Creating Ingress Resource 1 with custom ingress class")
	ingName1 := types.NamespacedName{Name: "echoserver-1", Namespace: testWorkloadNamespace}
	echoIng1 := buildEchoIngress(ingName1, ingressClass.Name, ingAnnotations, echoSvc)
	err := retry.OnError(defaultRetryPolicy,
		func(err error) bool {
			t.Logf("retrying creation of echo ingress due to %v", err)
			return !errors.IsAlreadyExists(err)
		},
		func() error { return kubeClient.Create(context.TODO(), echoIng1) })
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure echo ingress %s: %v", echoIng1.Name, err)
	}
	defer func() {
		waitForDeletion(t, kubeClient, echoIng1, defaultTimeout)
	}()

	t.Log("Creating Ingress Resource 2 with custom ingress class")
	ingName2 := types.NamespacedName{Name: "echoserver-2", Namespace: testWorkloadNamespace}
	echoIng2 := buildEchoIngress(ingName2, ingressClass.Name, ingAnnotations, echoSvc)
	err = retry.OnError(defaultRetryPolicy,
		func(err error) bool {
			t.Logf("retrying creation of echo ingress due to %v", err)
			return !errors.IsAlreadyExists(err)
		},
		func() error { return kubeClient.Create(context.TODO(), echoIng2) })
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure echo ingress %s: %v", echoIng2.Name, err)
	}
	defer func() {
		waitForDeletion(t, kubeClient, echoIng2, defaultTimeout)
	}()

	t.Log("Creating Ingress Resource 3 with custom ingress class")
	ingName3 := types.NamespacedName{Name: "echoserver-3", Namespace: testWorkloadNamespace}
	echoIng3 := buildEchoIngress(ingName3, ingressClass.Name, ingAnnotations, echoSvc)
	err = retry.OnError(defaultRetryPolicy,
		func(err error) bool {
			t.Logf("retrying creation of echo ingress due to %v", err)
			return !errors.IsAlreadyExists(err)
		},
		func() error { return kubeClient.Create(context.TODO(), echoIng3) })
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure echo ingress %s: %v", echoIng3.Name, err)
	}
	defer func() {
		waitForDeletion(t, kubeClient, echoIng3, defaultTimeout)
	}()

	var firstAddress string
	if firstAddress, err = getIngress(t, kubeClient, defaultTimeout, ingName1); err != nil {
		t.Fatalf("did not get expected available condition for ingress: %v", err)
	}

	for _, ingressName := range []types.NamespacedName{ingName2, ingName3} {
		var address string
		if address, err = getIngress(t, kubeClient, defaultTimeout, ingressName); err != nil {
			t.Fatalf("did not get expected available condition for ingress %s: %v", ingressName, err)
		}
		if address != firstAddress {
			t.Errorf("ingress %s does not have the address %s, instead has %s", ingressName, firstAddress, address)
		}
	}

}
