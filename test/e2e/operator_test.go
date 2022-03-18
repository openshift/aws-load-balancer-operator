//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"

	arv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	configv1 "github.com/openshift/api/config/v1"
	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"

	networkingolmv1alpha1 "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
)

var (
	kubeClient  client.Client
	scheme      = runtime.NewScheme()
	infraConfig configv1.Infrastructure
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(cco.Install(scheme))

	utilruntime.Must(networkingolmv1alpha1.AddToScheme(scheme))

	utilruntime.Must(cco.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme

	utilruntime.Must(configv1.Install(scheme))
	utilruntime.Must(cco.Install(scheme))
	utilruntime.Must(networkingv1.AddToScheme(scheme))
	utilruntime.Must(arv1.AddToScheme(scheme))
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

	if err = ensureCredentialsRequest(); err != nil {
		fmt.Printf("failed to create credentialsrequest for operator due to: %v\n", err)
	}

	os.Exit(m.Run())
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
			Namespace: "aws-load-balancer-operator",
		},
		Spec: cco.CredentialsRequestSpec{
			SecretRef: corev1.ObjectReference{
				Name:      "aws-load-balancer-operator",
				Namespace: "aws-load-balancer-operator",
			},
			ServiceAccountNames: []string{"aws-load-balancer-operator-controller-manager"},
			ProviderSpec:        providerSpec,
		},
	}

	return kubeClient.Create(context.Background(), &cr)
}

func TestOperatorAvailable(t *testing.T) {
	expected := []appsv1.DeploymentCondition{
		{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
	}
	if err := waitForOperatorDeploymentStatusCondition(t, kubeClient, expected...); err != nil {
		t.Errorf("Did not get expected available condition: %v", err)
	}
}
