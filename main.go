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
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"reflect"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	arv1 "k8s.io/api/admissionregistration/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/discovery"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"

	configv1 "github.com/openshift/api/config/v1"
	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	libgocrypto "github.com/openshift/library-go/pkg/crypto"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metrics "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	networkingolmv1 "github.com/openshift/aws-load-balancer-operator/api/v1"
	networkingolmv1alpha1 "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
	"github.com/openshift/aws-load-balancer-operator/pkg/aws"
	"github.com/openshift/aws-load-balancer-operator/pkg/controllers/awsloadbalancercontroller"
	"github.com/openshift/aws-load-balancer-operator/pkg/operator"
	//+kubebuilder:scaffold:imports
)

const (
	clusterInfrastructureName = "cluster"
	// apiServerGetTimeout bounds the read of the cluster APIServer so
	// that a hung Get is treated as a failed read.
	apiServerGetTimeout = 10 * time.Second
	// It's been noticed that the freshly provisioned credentials
	// may not be usable immediately. Therefore the first AWS call needs to be
	// repeated until it succeeds or times out.
	awsRequestTimeout      = 20 * time.Second
	awsRequestPollInterval = 1 * time.Second
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(cco.Install(scheme))

	utilruntime.Must(networkingolmv1alpha1.AddToScheme(scheme))

	utilruntime.Must(cco.AddToScheme(scheme))
	utilruntime.Must(networkingolmv1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme

	utilruntime.Must(configv1.Install(scheme))
	utilruntime.Must(cco.Install(scheme))
	utilruntime.Must(networkingv1.AddToScheme(scheme))
	utilruntime.Must(arv1.AddToScheme(scheme))
}

func main() {
	var (
		metricsAddr            string
		metricsTLSCertDir      string
		enableLeaderElection   bool
		probeAddr              string
		namespace              string
		image                  string
		trustedCAConfigMapName string
		webhookDisableHTTP2    bool
	)
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8443", "The address the metric endpoint binds to.")
	flag.StringVar(&metricsTLSCertDir, "metrics-tls-cert-dir", "", "The directory containing TLS certificates for the metrics endpoint.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&namespace, "namespace", "aws-load-balancer-operator", "The namespace where operands should be installed")
	flag.StringVar(&image, "image", "quay.io/aws-load-balancer-operator/aws-load-balancer-controller:latest", "The image to be used for the operand")
	flag.StringVar(&trustedCAConfigMapName, "trusted-ca-configmap", "", "The name of the config map containing TLS CA(s) which should be trusted by the controller's containers. PEM encoded file under \"ca-bundle.crt\" key is expected.")
	flag.BoolVar(&webhookDisableHTTP2, "webhook-disable-http2", false, "Disable HTTP/2 for the webhook server.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	webhookSrv := webhook.NewServer(webhook.Options{
		TLSOpts: []func(config *tls.Config){
			func(config *tls.Config) {
				if webhookDisableHTTP2 {
					config.NextProtos = []string{"http/1.1"}
				}
			},
		},
		Port: 9443,
	})

	// The manager runs with a cancelable context so that the TLS profile watcher
	// can trigger a graceful shutdown when the cluster TLS configuration changes,
	// letting the Deployment restart the pod to pick up the new profile.
	ctx, cancel := context.WithCancel(ctrl.SetupSignalHandler())
	defer cancel()

	restConfig := ctrl.GetConfigOrDie()
	profile, err := getTLSSecurityProfile(ctx, restConfig)
	if err != nil {
		setupLog.Error(err, "unable to determine the cluster TLS security profile")
		os.Exit(1)
	}
	tlsConfig, err := getTLSConfigFromProfile(profile.spec)
	if err != nil {
		setupLog.Error(err, "unable to get TLS configuration from profile")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme: scheme,
		Metrics: metrics.Options{
			BindAddress:    metricsAddr,
			SecureServing:  true,
			CertDir:        metricsTLSCertDir,
			FilterProvider: filters.WithAuthenticationAndAuthorization,
			TLSOpts: []func(*tls.Config){
				func(config *tls.Config) {
					config.MinVersion = tlsConfig.MinVersion
					config.CipherSuites = tlsConfig.CipherSuites
					config.CurvePreferences = tlsConfig.CurvePreferences
					config.NextProtos = []string{"http/1.1"}
				},
			},
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "7de51cf3.openshift.io",
		// The default cached client does not always return an updated value after write operations. So we use a non-cache client
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime#hdr-Clients_and_Caches
		NewClient: func(config *rest.Config, options client.Options) (client.Client, error) {
			// Must override the cache, otherwise the client will use it.
			options.Cache = nil
			return client.New(config, options)
		},
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				namespace: {},
			},
			DefaultEnableWatchBookmarks: ptr.To(true),
		},
		WebhookServer: webhookSrv,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// get the cluster details
	clusterName, awsRegion, err := clusterInfo(context.TODO(), mgr.GetClient())
	if err != nil {
		setupLog.Error(err, "failed to get cluster details")
		os.Exit(1)
	}

	// self provision with AWS credentials
	setupLog.Info("provisioning credentials")
	awsSharedCredFileName, err := operator.ProvisionCredentials(context.TODO(), mgr.GetClient(), namespace)
	if err != nil {
		setupLog.Error(err, "unable to provision cloud credentials")
		os.Exit(1)
	}

	// make and aws.EC2Client
	ec2Client, err := aws.NewClient(context.TODO(), awsRegion, awsSharedCredFileName)
	if err != nil {
		setupLog.Error(err, "failed to make aws client")
		os.Exit(1)
	}

	// get the VPC ID where the cluster is running
	vpcID, err := getVPCId(context.TODO(), ec2Client, clusterName, awsRequestTimeout, awsRequestPollInterval)
	if err != nil {
		setupLog.Error(err, "failed to get VPC ID")
		os.Exit(1)
	}

	if err = (&awsloadbalancercontroller.AWSLoadBalancerControllerReconciler{
		Client:                 mgr.GetClient(),
		Scheme:                 mgr.GetScheme(),
		EC2Client:              ec2Client,
		Namespace:              namespace,
		Image:                  image,
		VPCID:                  vpcID,
		ClusterName:            clusterName,
		AWSRegion:              awsRegion,
		TrustedCAConfigMapName: trustedCAConfigMapName,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AWSLoadBalancerController")
		os.Exit(1)
	}
	if err = (&networkingolmv1.AWSLoadBalancerController{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "AWSLoadBalancerController")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	// Watch the cluster APIServer so that runtime changes to the TLS security
	// profile or adherence policy trigger a restart to re-apply them.
	if err = setupTLSProfileWatch(mgr, cancel, profile); err != nil {
		setupLog.Error(err, "unable to set up TLS profile watch")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func clusterInfo(ctx context.Context, client client.Client) (clusterName, awsRegion string, err error) {
	var infra configv1.Infrastructure
	infraKey := types.NamespacedName{
		Name: clusterInfrastructureName,
	}
	err = client.Get(ctx, infraKey, &infra)
	if err != nil {
		err = fmt.Errorf("failed to get Infrastructure %q: %w", clusterInfrastructureName, err)
		return
	}

	if infra.Status.InfrastructureName == "" {
		err = fmt.Errorf("could not get AWS region from Infrastructure %q status", clusterInfrastructureName)
		return
	}
	clusterName = infra.Status.InfrastructureName

	if infra.Status.PlatformStatus == nil || infra.Status.PlatformStatus.AWS == nil || infra.Status.PlatformStatus.AWS.Region == "" {
		err = fmt.Errorf("could not get AWS region from Infrastructure %q status", clusterInfrastructureName)
		return
	}
	awsRegion = infra.Status.PlatformStatus.AWS.Region
	return
}

// getVPCId tries to retrieve VPC ID of the given cluster polling until it succeeds or times out.
func getVPCId(ctx context.Context, ec2Client aws.EC2Client, clusterName string, timeout, pollInterval time.Duration) (string, error) {
	timeoutCh := time.After(timeout)
	ticker := time.NewTicker(pollInterval)

	for {
		select {
		case <-timeoutCh:
			return "", fmt.Errorf("timed out trying to get vpc id")
		case <-ticker.C:
			vpcID, err := aws.GetVPCId(ctx, ec2Client, clusterName)
			if err != nil {
				setupLog.Info("failed to get VPC ID", "error", err)
				continue
			}
			return vpcID, nil
		}
	}
}

// tlsGroupToCurveID maps a configv1.TLSGroup to a crypto/tls CurveID.
// Groups not supported by the Go runtime are returned with ok=false.
var tlsGroupToCurveID = map[configv1.TLSGroup]tls.CurveID{
	configv1.TLSGroupX25519:         tls.X25519,
	configv1.TLSGroupSecP256r1:      tls.CurveP256,
	configv1.TLSGroupSecP384r1:      tls.CurveP384,
	configv1.TLSGroupSecP521r1:      tls.CurveP521,
	configv1.TLSGroupX25519MLKEM768: tls.X25519MLKEM768,
}

// shouldHonorClusterTLSProfile returns true if the component should honor the
// cluster-wide TLS security profile settings from apiserver.config.openshift.io/cluster.
// Unknown enum values are treated as StrictAllComponents for forward compatibility.
func shouldHonorClusterTLSProfile(adherence configv1.TLSAdherencePolicy) bool {
	switch adherence {
	case configv1.TLSAdherencePolicyNoOpinion, configv1.TLSAdherencePolicyLegacyAdheringComponentsOnly:
		return false
	default:
		return true
	}
}

// tlsProfileWatcher watches the cluster APIServer for changes to the TLS security
// profile or adherence policy. When either changes from the value observed at
// startup, it cancels the manager context so the pod restarts and re-applies the
// new configuration to its TLS servers.
type tlsProfileWatcher struct {
	client           client.Client
	cancel           context.CancelFunc
	initialProfile   *configv1.TLSSecurityProfile
	initialAdherence configv1.TLSAdherencePolicy
}

func (w *tlsProfileWatcher) Reconcile(ctx context.Context, _ reconcile.Request) (reconcile.Result, error) {
	var apiServer configv1.APIServer
	if err := w.client.Get(ctx, types.NamespacedName{Name: "cluster"}, &apiServer); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	// An adherence change matters only when it flips whether the cluster profile
	// is honored, moving between NoOpinion and LegacyAdheringComponentsOnly
	// leaves the operator's TLS configuration untouched. A profile change only
	// matters when the current adherence policy makes the operator honor the
	// profile; in legacy or unset mode the profile does not affect the operator's
	// TLS configuration, so a profile-only change must not trigger a restart.
	adherenceChanged := shouldHonorClusterTLSProfile(apiServer.Spec.TLSAdherence) !=
		shouldHonorClusterTLSProfile(w.initialAdherence)
	profileChanged := shouldHonorClusterTLSProfile(apiServer.Spec.TLSAdherence) &&
		!reflect.DeepEqual(apiServer.Spec.TLSSecurityProfile, w.initialProfile)

	if adherenceChanged || profileChanged {
		setupLog.Info("cluster TLS configuration changed, shutting down to re-apply it",
			"oldAdherence", w.initialAdherence, "newAdherence", apiServer.Spec.TLSAdherence)
		w.cancel()
	}

	return reconcile.Result{}, nil
}

// setupTLSProfileWatch registers a controller that watches the cluster APIServer
// and cancels the manager context when the TLS profile or adherence policy
// changes. It is a no-op when the APIServer config API is not served
// (e.g. on non-OpenShift clusters).
func setupTLSProfileWatch(mgr ctrl.Manager, cancel context.CancelFunc, profile *tlsProfile) error {
	if !profile.watch {
		return nil
	}

	watcher := &tlsProfileWatcher{
		client:           mgr.GetClient(),
		cancel:           cancel,
		initialProfile:   profile.spec,
		initialAdherence: profile.adherence,
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named("tlsprofilewatcher").
		WithOptions(controller.Options{NeedLeaderElection: ptr.To(false)}).
		For(&configv1.APIServer{}).
		Complete(watcher)
}

type tlsProfile struct {
	spec      *configv1.TLSSecurityProfile
	adherence configv1.TLSAdherencePolicy
	// watch reports whether the TLS profile watch should be set up.
	watch bool
}

// getTLSSecurityProfile resolves the TLS security profile and the adherence
// policy from the cluster APIServer.  A failure to discover whether the APIServer API
// is served is fatal: it is not possible to tell a cluster from a broken one.
// When the API is not served, the default profile is used and no watch is set up.
// When the API is served but the cluster APIServer cannot be read, the default
// profile is used and the watch is still set up, so a later successful read can
// replace it.
func getTLSSecurityProfile(ctx context.Context, config *rest.Config) (*tlsProfile, error) {
	cl, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create client to fetch APIServer config: %w", err)
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery client: %w", err)
	}

	served, err := apiServerAPIServed(discoveryClient)
	if err != nil {
		return nil, err
	}
	if !served {
		setupLog.Info("APIServer config API is not served, using default TLS profile")
		return &tlsProfile{}, nil
	}

	profile := &tlsProfile{
		// The baseline adherence is deliberately StrictAllComponents
		// which corresponds to the default TLS profile. The watch will restart
		// the operator as soon as the real object will show something else.
		adherence: configv1.TLSAdherencePolicyStrictAllComponents,
		watch:     true,
	}

	getCtx, cancel := context.WithTimeoutCause(ctx, apiServerGetTimeout, fmt.Errorf("get did not complete within %s", apiServerGetTimeout))
	defer cancel()

	var apiServer configv1.APIServer
	if err := cl.Get(getCtx, types.NamespacedName{Name: "cluster"}, &apiServer); err != nil {
		setupLog.Error(err, "unable to fetch APIServer config, using default TLS profile", "name", "cluster")
		return profile, nil
	}

	profile.adherence = apiServer.Spec.TLSAdherence

	// Only honor the cluster TLS profile if tlsAdherence is set to StrictAllComponents
	if !shouldHonorClusterTLSProfile(apiServer.Spec.TLSAdherence) {
		setupLog.Info("not honoring cluster TLS profile due to tlsAdherence policy", "tlsAdherence", apiServer.Spec.TLSAdherence)
		return profile, nil
	}

	profile.spec = apiServer.Spec.TLSSecurityProfile

	return profile, nil
}

func getTLSConfigFromProfile(profile *configv1.TLSSecurityProfile) (*tls.Config, error) {
	profileSpec := configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
	if profile != nil {
		if profile.Type == configv1.TLSProfileCustomType && profile.Custom != nil {
			profileSpec = &profile.Custom.TLSProfileSpec
		} else if spec, ok := configv1.TLSProfiles[profile.Type]; ok {
			profileSpec = spec
		}
	}

	cfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if len(profileSpec.Ciphers) > 0 {
		ianaNames := libgocrypto.OpenSSLToIANACipherSuites(profileSpec.Ciphers)
		var suites []uint16
		for _, name := range ianaNames {
			id, err := libgocrypto.CipherSuite(name)
			if err != nil {
				setupLog.Info("skipping unsupported TLS cipher", "cipher", name, "error", err)
				continue
			}
			suites = append(suites, id)
		}
		cfg.CipherSuites = suites
	}

	if len(profileSpec.MinTLSVersion) > 0 {
		v, err := libgocrypto.TLSVersion(string(profileSpec.MinTLSVersion))
		if err != nil {
			return nil, fmt.Errorf("invalid TLS version %q: %w", profileSpec.MinTLSVersion, err)
		}
		cfg.MinVersion = v
	}

	if len(profileSpec.Groups) > 0 {
		var curves []tls.CurveID
		for _, g := range profileSpec.Groups {
			if id, ok := tlsGroupToCurveID[g]; ok {
				curves = append(curves, id)
			} else {
				setupLog.Info("skipping unsupported TLS group", "group", g)
			}
		}
		if len(curves) > 0 {
			cfg.CurvePreferences = curves
		}
	}

	return libgocrypto.SecureTLSConfig(cfg), nil
}

// apiServerAPIServed reports whether the cluster serves the APIServer resource clusters.
func apiServerAPIServed(discoveryClient discovery.ServerResourcesInterface) (bool, error) {
	groupVersion := configv1.GroupVersion.String()

	list, err := discoveryClient.ServerResourcesForGroupVersion(groupVersion)
	if err != nil {
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to discover %s: %w", groupVersion, err)
	}

	for _, resource := range list.APIResources {
		if resource.Kind == "APIServer" {
			return true, nil
		}
	}

	return false, nil
}
