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

package main

import (
	"flag"
	"os"
	"path/filepath"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	configv1 "github.com/openshift/api/config/v1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	networkingolmv1alpha1 "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
	"github.com/openshift/aws-load-balancer-operator/pkg/aws"
	"github.com/openshift/aws-load-balancer-operator/pkg/controllers/awsloadbalancercontroller"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(networkingolmv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme

	utilruntime.Must(configv1.Install(scheme))
	utilruntime.Must(cco.Install(scheme))
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

	// the kube-config flag is already defined elsewhere and cannot be redeclared here. This is just falls back to the default
	// kubeconfig file path location and attempts to detect if the operator is running in-cluster or locally.
	var kubeconfig string
	if home := homeDir(); home != "" {
		kubeconfig = filepath.Join(homeDir(), ".kube", "config")
	}

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	// a client-go based Infrastructure client is required because the controller-runtime client cannot be used
	// before the controller manager is started. Open issue upstream https://github.com/kubernetes-sigs/controller-runtime/issues/607
	var clientConfig *rest.Config
	_, err := os.Stat(kubeconfig)
	if err != nil {
		if !os.IsNotExist(err) {
			setupLog.Error(err, "failed to stat the provided kube config file")
			os.Exit(1)
		} else {
			clientConfig, err = rest.InClusterConfig()
			if err != nil {
				setupLog.Error(err, "failed to get client config in-cluster")
				os.Exit(1)
			}
		}
	} else {
		clientConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			setupLog.Error(err, "failed to make client config from passed kube config file", kubeconfig)
			os.Exit(1)
		}
	}
	configClient, err := configv1client.NewForConfig(clientConfig)
	if err != nil {
		setupLog.Error(err, "failed to make configv1 client")
		os.Exit(1)
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "7de51cf3.openshift.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	ec2Client, err := aws.GetEC2Client(configClient)
	if err != nil {
		setupLog.Error(err, "unable to make EC2 Client")
		os.Exit(1)
	}

	if err = (&awsloadbalancercontroller.AWSLoadBalancerControllerReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		EC2Client: ec2Client,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AWSLoadBalancerController")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}
