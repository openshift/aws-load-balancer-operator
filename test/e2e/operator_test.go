//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"net/http"
	"os"
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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/waf"
	waftypes "github.com/aws/aws-sdk-go-v2/service/waf/types"
	"github.com/aws/aws-sdk-go-v2/service/wafv2"
	wafv2types "github.com/aws/aws-sdk-go-v2/service/wafv2/types"
	configv1 "github.com/openshift/api/config/v1"
	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
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
	t.Logf("Ensuring test workload namespace %s", testWorkloadNamespace)
	ns := &corev1.Namespace{ObjectMeta: v1.ObjectMeta{Name: testWorkloadNamespace}}
	err := kubeClient.Create(context.TODO(), ns)
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure namespace %s: %v", testWorkloadNamespace, err)
	}

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
	echoIng := buildEchoIngress(ingName, "alb", ingAnnotations, echosvc)
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
	t.Logf("Ensuring test workload namespace %s", testWorkloadNamespace)
	ns := &corev1.Namespace{ObjectMeta: v1.ObjectMeta{Name: testWorkloadNamespace}}
	err := kubeClient.Create(context.TODO(), ns)
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure namespace %s: %v", testWorkloadNamespace, err)
	}

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

	t.Log("Creating Ingress Resource with custom ingress class")
	ingName := types.NamespacedName{Name: "echoserver", Namespace: testWorkloadNamespace}
	ingAnnotations := map[string]string{
		"alb.ingress.kubernetes.io/scheme":      "internet-facing",
		"alb.ingress.kubernetes.io/target-type": "instance",
	}
	echoIng := buildEchoIngress(ingName, ingclass.Name, ingAnnotations, echosvc)
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
			return r.StatusCode == http.StatusOK
		})
		if err != nil {
			t.Fatalf("failed verify condition with external client: %v", err)
		}
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
	t.Logf("Ensuring test workload namespace %s", testWorkloadNamespace)
	ns := &corev1.Namespace{ObjectMeta: v1.ObjectMeta{Name: testWorkloadNamespace}}
	err := kubeClient.Create(context.TODO(), ns)
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure namespace %s: %v", testWorkloadNamespace, err)
	}

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
	echoIng := buildEchoIngress(ingName, "alb", ingAnnotations, echosvc)
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

func TestAWSLoadBalancerControllerWithWAFv1(t *testing.T) {
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

	testWorkloadNamespace := "aws-load-balancer-test-wafv2"
	t.Logf("Ensuring test workload namespace %s", testWorkloadNamespace)
	ns := &corev1.Namespace{ObjectMeta: v1.ObjectMeta{Name: testWorkloadNamespace}}
	err := kubeClient.Create(context.TODO(), ns)
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure namespace %s: %v", testWorkloadNamespace, err)
	}

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

	wafClient := waf.NewFromConfig(cfg)

	token, err := wafClient.GetChangeToken(context.TODO(), &waf.GetChangeTokenInput{})
	if err != nil {
		t.Fatalf("failed to get change token for waf classic %v", err)
	}
	acl, err := wafClient.CreateWebACL(context.TODO(), &waf.CreateWebACLInput{
		DefaultAction: &waftypes.WafAction{Type: waftypes.WafActionTypeBlock},
		MetricName:    aws.String("echoserverclassicacl"),
		Name:          aws.String("echoserverclassicacl"),
		ChangeToken:   token.ChangeToken,
	})
	if err != nil {
		t.Fatalf("failed to create aws wafv2 acl due to %v", err)
	}
	defer func() {
		token, err := wafClient.GetChangeToken(context.TODO(), &waf.GetChangeTokenInput{})
		if err != nil {
			t.Fatalf("failed to get change token for waf classic %v", err)
		}

		_, err = wafClient.DeleteWebACL(context.TODO(), &waf.DeleteWebACLInput{
			ChangeToken: token.ChangeToken,
			WebACLId:    acl.WebACL.WebACLId,
		})
		if err != nil {
			t.Fatalf("failed to delete aws wafv2 acl due to %v", err)
		}
	}()

	t.Log("Creating Ingress Resource with default ingress class")
	ingName := types.NamespacedName{Name: "echoserver", Namespace: testWorkloadNamespace}
	ingAnnotations := map[string]string{
		"alb.ingress.kubernetes.io/scheme":      "internet-facing",
		"alb.ingress.kubernetes.io/target-type": "instance",
		"alb.ingress.kubernetes.io/waf-acl-id":  *acl.WebACL.WebACLId,
	}
	echoIng := buildEchoIngress(ingName, "alb", ingAnnotations, echosvc)
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
				Action:   []string{"waf:GetChangeToken", "waf:CreateWebACL", "waf:DeleteWebACL"},
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
