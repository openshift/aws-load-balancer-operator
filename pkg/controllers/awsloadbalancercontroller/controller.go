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

package awsloadbalancercontroller

import (
	"context"
	"fmt"
	"time"

	arv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"

	configv1 "github.com/openshift/api/config/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1"
	"github.com/openshift/aws-load-balancer-operator/pkg/aws"
)

const (
	// the name of the AWSLoadBalancerController resource which will be reconciled
	controllerName = "cluster"
	// clusterInfrastructureName is the name of the 'cluster' infrastructure object.
	clusterInfrastructureName = "cluster"
	// the port on which controller metrics are served
	controllerMetricsPort = 8080
	// the port on which the controller webhook is served
	controllerWebhookPort = 9443
	// common prefix for all resource of an operand
	controllerResourcePrefix = "aws-load-balancer-controller"
	// secretMissingReEnqueueDuration is the delay to re-enqueue.
	secretMissingReEnqueueDuration = time.Second * 30
)

// AWSLoadBalancerControllerReconciler reconciles a AWSLoadBalancerController object
type AWSLoadBalancerControllerReconciler struct {
	client.Client
	Scheme                 *runtime.Scheme
	Namespace              string
	Image                  string
	EC2Client              aws.EC2Client
	ClusterName            string
	VPCID                  string
	AWSRegion              string
	TrustedCAConfigMapName string
}

//+kubebuilder:rbac:groups=networking.olm.openshift.io,resources=awsloadbalancercontrollers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.olm.openshift.io,resources=awsloadbalancercontrollers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=networking.olm.openshift.io,resources=awsloadbalancercontrollers/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=services;secrets,namespace=system,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=configmaps,namespace=system,verbs=get;list;watch
//+kubebuilder:rbac:groups="networking.k8s.io",resources=ingressclasses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="config.openshift.io",resources=infrastructures,verbs=get;list;watch
//+kubebuilder:rbac:groups="apps",resources=deployments,namespace=system,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=serviceaccounts,namespace=system,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,namespace=system,resources=roles;rolebindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=bind;get,resourceNames=aws-load-balancer-operator-controller-role
//+kubebuilder:rbac:groups=cloudcredential.openshift.io,resources=credentialsrequests;credentialsrequests/status;credentialsrequests/finalizers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations;mutatingwebhookconfigurations,verbs=get;list;watch;create;update;patch;delete

func (r *AWSLoadBalancerControllerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	lbController, exists, err := r.getAWSLoadBalancerController(ctx, req.Name)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get AWSLoadBalancerController %q: %w", req.Name, err)
	}
	if !exists {
		return ctrl.Result{}, nil
	}

	if lbController.DeletionTimestamp != nil {
		logger.Info("AWSLoadBalancerController is going to be deleted. Skipping Reconcile")
		return ctrl.Result{}, nil

	}

	servingSecretName := fmt.Sprintf("%s-serving-%s", controllerResourcePrefix, lbController.Name)

	// if the processed subnets have not yet been written into the status or if the tagging policy has changed then update the subnets
	if lbController.Status.Subnets == nil || (lbController.Spec.SubnetTagging != lbController.Status.Subnets.SubnetTagging) {
		internalSubnets, publicSubnets, untaggedSubnets, taggedSubnets, err := r.tagSubnets(ctx, lbController)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update subnets: %w", err)
		}
		err = r.updateStatusSubnets(ctx, lbController, internalSubnets, publicSubnets, untaggedSubnets, taggedSubnets, lbController.Spec.SubnetTagging)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update AWSLoadBalancerController %q status with subnets: %w", req.Name, err)
		}
		// reload the resource after updating the status
		lbController, _, err = r.getAWSLoadBalancerController(ctx, req.Name)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get AWSLoadBalancerController %q: %w", req.Name, err)
		}
	}

	infraConfig := &configv1.Infrastructure{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: clusterInfrastructureName}, infraConfig); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get infrastructure %q: %w", clusterInfrastructureName, err)
	}
	platformStatus := infraConfig.Status.PlatformStatus

	if err := r.ensureIngressClass(ctx, lbController); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure default IngressClass for AWSLoadBalancerController %q: %v", req.Name, err)
	}
	// if the ingress class in the status differs from what's in the spec update it
	if lbController.Spec.IngressClass != lbController.Status.IngressClass {
		err = r.updateStatusIngressClass(ctx, lbController, lbController.Spec.IngressClass)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update IngressClass in AWSLoadBalancerController %q Status: %w", req.Name, err)
		}
		// reload the resource after updating the status
		lbController, _, err = r.getAWSLoadBalancerController(ctx, req.Name)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get AWSLoadBalancerController %q: %w", req.Name, err)
		}
	}

	credSecretNsName := types.NamespacedName{Namespace: r.Namespace}
	if lbController.Spec.Credentials == nil {
		credentialsRequest, err := r.ensureCredentialsRequest(ctx, r.Namespace, lbController)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to ensure CredentialsRequest for AWSLoadBalancerController %q: %w", req.Name, err)
		}
		credSecretNsName.Name = credentialsRequest.Spec.SecretRef.Name
	} else {
		credSecretNsName.Name = lbController.Spec.Credentials.Name
	}

	secretProvisioned, err := r.credentialsSecretProvisioned(ctx, credSecretNsName)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to verify credentials secret %q for AWSLoadBalancerController %q has been provisioned: %w", credSecretNsName.Name, req.Name, err)
	}

	// updating CR status
	if err := r.updateControllerStatus(ctx, lbController, nil, credSecretNsName.Name, secretProvisioned); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status of AWSLoadBalancerController %q: %w", req.Name, err)
	}

	// re-enqueue if secret is not provisioned
	if !secretProvisioned {
		// retrying after delay to ensure secret provisioning.
		logger.Info("(Retrying) failed to ensure secret from credentials request", "secret", credSecretNsName.Name)
		return ctrl.Result{RequeueAfter: secretMissingReEnqueueDuration}, nil
	}

	var trustCAConfigMap *corev1.ConfigMap
	if r.TrustedCAConfigMapName != "" {
		configMap, configMapExists, err := r.getConfigMap(ctx, r.TrustedCAConfigMapName, r.Namespace)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to get the trusted CA configmap: %w", err)
		}
		if !configMapExists {
			logger.Info("(Retrying) trusted CA config map doesn't exist", "configmap", r.TrustedCAConfigMapName)
			return reconcile.Result{RequeueAfter: secretMissingReEnqueueDuration}, nil
		}
		trustCAConfigMap = configMap
	}

	sa, err := r.ensureControllerServiceAccount(ctx, r.Namespace, lbController)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure AWSLoadBalancerController %q service account: %w", req.Name, err)
	}

	err = r.ensureClusterRoleAndBinding(ctx, sa, lbController)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure ClusterRole and Binding for AWSLoadBalancerController %q: %w", req.Name, err)
	}

	deployment, err := r.ensureDeployment(ctx, sa, credSecretNsName.Name, servingSecretName, lbController, platformStatus, trustCAConfigMap)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure Deployment for AWSLoadbalancerController %q: %w", req.Name, err)
	}

	service, err := r.ensureService(ctx, r.Namespace, lbController, servingSecretName, deployment)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure service for AWSLoadBalancerController %q: %w", req.Name, err)
	}

	err = r.ensureWebhooks(ctx, lbController, service)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure webhooks for AWSLoadBalancerController %q: %w", req.Name, err)
	}

	if err := r.updateControllerStatus(ctx, lbController, deployment, credSecretNsName.Name, secretProvisioned); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status of AWSLoadBalancerController %q: %w", req.Name, err)
	}
	return ctrl.Result{}, nil
}

func (r *AWSLoadBalancerControllerReconciler) getAWSLoadBalancerController(ctx context.Context, name string) (*albo.AWSLoadBalancerController, bool, error) {
	var controller albo.AWSLoadBalancerController
	controllerKey := types.NamespacedName{Name: name}
	err := r.Get(ctx, controllerKey, &controller)
	if err != nil && errors.IsNotFound(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &controller, true, nil
}

func (r *AWSLoadBalancerControllerReconciler) getConfigMap(ctx context.Context, name, namespace string) (*corev1.ConfigMap, bool, error) {
	var cm corev1.ConfigMap
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &cm)
	if err != nil && errors.IsNotFound(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &cm, true, nil
}

// BuildManagedController returns the controller builder with all the watches set up.
func (r *AWSLoadBalancerControllerReconciler) BuildManagedController(mgr ctrl.Manager) *builder.Builder {
	bldr := ctrl.NewControllerManagedBy(mgr).
		For(&albo.AWSLoadBalancerController{}, builder.WithPredicates(predicate.NewPredicateFuncs(hasName(controllerName)))).
		Owns(&cco.CredentialsRequest{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.ClusterRoleBinding{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&arv1.ValidatingWebhookConfiguration{}).
		Owns(&arv1.MutatingWebhookConfiguration{})

	clusterALBCInstance := func(ctx context.Context, o client.Object) []reconcile.Request {
		return []reconcile.Request{
			{
				NamespacedName: types.NamespacedName{
					Name: controllerName,
				},
			},
		}
	}

	if r.TrustedCAConfigMapName != "" {
		// Requeue the only (cluster) instance of AWSLoadBalancerController
		// so that the main reconciliation loop can detect the changes in the trusted CA configmap's contents
		// and redeploy the controller if needed.
		// The change detection is achieved using the annotation which contains the configmap's contents hash.
		// The hash is recalculated at each reconciliation and put in the controller deployment's template pod spec
		// leading to a rollout in case of a change.
		bldr = bldr.Watches(&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(clusterALBCInstance),
			builder.WithPredicates(predicate.And(
				predicate.NewPredicateFuncs(inNamespace(r.Namespace))),
				predicate.NewPredicateFuncs(hasName(r.TrustedCAConfigMapName))))
	}
	// Watch Infrastructure object to detect changes in AWS user tags
	bldr = bldr.Watches(&configv1.Infrastructure{},
		handler.EnqueueRequestsFromMapFunc(clusterALBCInstance),
		builder.WithPredicates(
			predicate.NewPredicateFuncs(hasName(clusterInfrastructureName))))

	return bldr
}

// SetupWithManager sets up the controller with the Manager.
func (r *AWSLoadBalancerControllerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return r.BuildManagedController(mgr).Complete(r)
}

// hasName returns a predicate which checks whether an object has the given name.
func hasName(name string) func(o client.Object) bool {
	return func(o client.Object) bool {
		return o.GetName() == name
	}
}

// inNamespace returns a predicate which checks whether an object belongs to the given namespace.
func inNamespace(namespace string) func(o client.Object) bool {
	return func(o client.Object) bool {
		return o.GetNamespace() == namespace
	}
}
