/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"path/filepath"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"

	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	configv1 "github.com/openshift/api/config/v1"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1"
	albc "github.com/openshift/aws-load-balancer-operator/pkg/controllers/awsloadbalancercontroller"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var k8sClient client.Client
var testEnv *envtest.Environment
var cancelManager context.CancelFunc
var reconcileCollector = &AWSLoadBalancerControllerReconcileCollector{
	AWSLoadBalancerControllerReconciler: &albc.AWSLoadBalancerControllerReconciler{
		Namespace:              "aws-load-balancer-operator",
		TrustedCAConfigMapName: "test-trusted-ca",
	},
	Requests: make(chan ctrl.Request),
}

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
			filepath.Join("..", "utils", "test", "crd"),
		},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = albo.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = cco.Install(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = configv1.Install(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme.Scheme,
		LeaderElection:     false,
		MetricsBindAddress: "0",
	})
	Expect(err).NotTo(HaveOccurred())

	err = reconcileCollector.BuildManagedController(mgr).Complete(reconcileCollector)
	Expect(err).NotTo(HaveOccurred())

	operatorTestNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "aws-load-balancer-operator",
		},
	}
	Expect(k8sClient.Create(context.Background(), operatorTestNs)).Should(Succeed())

	var ctx context.Context
	ctx, cancelManager = context.WithCancel(context.Background())

	go func() {
		err = mgr.Start(ctx)
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
	}()
}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancelManager()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

// AWSLoadBalancerControllerReconcileCollector collects reconcile requests.
type AWSLoadBalancerControllerReconcileCollector struct {
	*albc.AWSLoadBalancerControllerReconciler
	Requests chan ctrl.Request
}

func (c *AWSLoadBalancerControllerReconcileCollector) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	c.Requests <- req
	return ctrl.Result{}, nil
}
