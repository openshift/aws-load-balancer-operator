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
	"strings"

	arv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"

	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
	"github.com/openshift/aws-load-balancer-operator/pkg/aws"
)

const (
	controllerName               = "cluster"
	controllerSecretName         = "albo-cluster-credentials"
	controllerServiceAccountName = "albo-cluster-sa"
	// the port on which controller metrics are served
	controllerMetricsPort = 8080
	// the port on which the controller webhook is served
	controllerWebhookPort = 9443
)

// AWSLoadBalancerControllerReconciler reconciles a AWSLoadBalancerController object
type AWSLoadBalancerControllerReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Namespace string
	Image     string
	EC2Client aws.EC2Client
}

//+kubebuilder:rbac:groups=networking.olm.openshift.io,resources=awsloadbalancercontrollers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.olm.openshift.io,resources=awsloadbalancercontrollers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=networking.olm.openshift.io,resources=awsloadbalancercontrollers/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="config.openshift.io",resources=infrastructures,verbs=get;list;watch
//+kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings;clusterroles;clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cloudcredential.openshift.io,resources=credentialsrequests;credentialsrequests/status;credentialsrequests/finalizers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations;mutatingwebhookconfigurations,verbs=get;list;watch;create;update;patch;delete

func (r *AWSLoadBalancerControllerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	lbController := &albo.AWSLoadBalancerController{}
	if err := r.Client.Get(ctx, req.NamespacedName, lbController); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get AWSLoadBalancerController %s: %w", req, err)
	}

	// if the processed subnets have not yet been written into the status or if the tagging policy has changed then update the subnets
	if lbController.Status.Subnets == nil || (lbController.Spec.SubnetTagging != lbController.Status.Subnets.SubnetTagging) {
		err := r.tagSubnets(ctx, lbController)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update subnets: %w", err)
		}
	}

	if err := r.ensureCredentialsRequest(ctx, r.Namespace, lbController); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure CredentialsRequest for AWSLoadBalancerController %s: %w", req, err)
	}

	haveServiceAccount, sa, err := r.ensureControllerServiceAccount(ctx, r.Namespace, lbController)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure AWSLoadBalancerController service account: %w", err)
	} else if !haveServiceAccount {
		return ctrl.Result{}, fmt.Errorf("failed to ensure ServiceAccount for AWSLoadBalancerControler %s: %w", req, err)
	}

	err = r.ensureClusterRoleAndBindings(ctx, lbController)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure ClusterRole and Binding for AWSLoadBalancerController %s: %w", req, err)
	}

	deployment, err := r.ensureControllerDeployment(ctx, r.Namespace, r.Image, sa, lbController)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure Deployment for AWSLoadbalancerController %s: %w", req, err)
	}

	service, err := r.ensureService(ctx, r.Namespace, lbController, deployment)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure service for AWSLoadBalancerController %s: %w", req, err)
	}

	err = r.ensureWebhooks(ctx, lbController, service)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure webhooks for AWSLoadBalancerController %s: %w", req, err)
	}

	if err := r.updateControllerStatus(ctx, lbController, deployment); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status of AWSLoadBalancerController %s: %w", req, err)
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AWSLoadBalancerControllerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&albo.AWSLoadBalancerController{}).
		Owns(&cco.CredentialsRequest{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.ClusterRole{}).
		Owns(&rbacv1.ClusterRoleBinding{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&arv1.ValidatingWebhookConfiguration{}).
		Owns(&arv1.MutatingWebhookConfiguration{}).
		WithEventFilter(reconcileClusterNamedResource()).
		Complete(r)
}

func reconcileClusterNamedResource() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return strings.EqualFold(controllerName, e.Object.GetName())
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return strings.EqualFold(controllerName, e.ObjectNew.GetName())
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return strings.EqualFold(controllerName, e.Object.GetName())
		},
	}
}
