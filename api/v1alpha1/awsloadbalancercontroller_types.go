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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:validation:Enum=AWSShield;AWSWAFv1;AWSWAFv2
type AWSAddon string

const (
	AWSAddonShield AWSAddon = "AWSShield"
	AWSAddonWAFv1  AWSAddon = "AWSWAFv1"
	AWSAddonWAFv2  AWSAddon = "AWSWAFv2"
)

// +kubebuilder:validation:Enum=Auto;Manual
type SubnetTaggingPolicy string

const (

	// AutoSubnetTaggingPolicy enables automatic subnet tagging.
	AutoSubnetTaggingPolicy SubnetTaggingPolicy = "Auto"

	// ManualSubnetTaggingPolicy disables automatic subnet tagging.
	ManualSubnetTaggingPolicy SubnetTaggingPolicy = "Manual"
)

// AWSLoadBalancerControllerSpec defines the desired state of AWSLoadBalancerController
type AWSLoadBalancerControllerSpec struct {

	// SubnetTagging describes how resource tagging will be done by the operator.
	//
	// When in "Auto", the operator will detect the subnets where the load balancers
	// will be provisioned and have the required resource tags on them. Whereas when
	// set to manual, this responsibility lies on the user.
	//
	// +kubebuilder:default:=Auto
	// +kubebuilder:validation:Optional
	// +optional
	SubnetTagging SubnetTaggingPolicy `json:"subnetTagging,omitempty"`

	// Default AWS Tags that will be applied to all AWS resources managed by this
	// controller (default []).
	//
	// This value is required so that this controller can function as expected
	// in parallel to openshift-router.
	//
	// +kubebuilder:default:={}
	// +kubebuilder:validation:Optional
	// +optional
	AdditionalResourceTags map[string]string `json:"additionalResourceTags,omitempty"`

	// IngressClass specifies the Ingress class which the controller will reconcile.
	// This Ingress class will be created unless it already exists.
	// The value will default to "alb".
	//
	// +kubebuilder:default:=alb
	// +kubebuilder:validation:Optional
	// +optional
	IngressClass string `json:"ingressClass,omitempty"`

	// Config specifies further customization options for the controller's deployment spec.
	//
	// +kubebuilder:validation:Optional
	// +optional
	Config *AWSLoadBalancerDeploymentConfig `json:"config,omitempty"`

	// AWSAddon describes the AWS services that can be integrated with
	// the AWS Load Balancer.
	//
	// +kubebuilder:validation:Optional
	// +optional
	EnabledAddons []AWSAddon `json:"enabledAddons,omitempty"` // indicates which AWS addons should be disabled.

	// Credentials is a reference to a secret containing
	// the AWS credentials to be used by the controller.
	// The secret is required to be in the operator namespace.
	// If this field is empty - the credentials will be
	// requested using the Cloud Credentials API,
	// see https://docs.openshift.com/container-platform/4.11/authentication/managing_cloud_provider_credentials/about-cloud-credential-operator.html.
	//
	// +kubebuilder:validation:Optional
	// +optional
	Credentials *SecretReference `json:"credentials,omitempty"`
}

type AWSLoadBalancerDeploymentConfig struct {

	// +kubebuilder:default:=2
	// +kubebuilder:validation:Optional
	// +optional
	Replicas int32 `json:"replicas,omitempty"`
}

// SecretReference contains the information to let you locate the desired secret.
// Secret is required to be in the operator namespace.
type SecretReference struct {
	// Name is the name of the secret.
	//
	// +kubebuilder:validation:Required
	// +required
	Name string `json:"name"`
}

// AWSLoadBalancerControllerStatus defines the observed state of AWSLoadBalancerController.
type AWSLoadBalancerControllerStatus struct {

	// Conditions is a list of operator-specific conditions
	// and their status.
	//
	// +kubebuilder:validation:Optional
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed.
	//
	// +kubebuilder:validation:Optional
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Subnets contains details of the subnets of the cluster
	//
	// +kubebuilder:validation:Optional
	// +optional
	Subnets *AWSLoadBalancerControllerStatusSubnets `json:"subnets,omitempty"`

	// IngressClass is the current default Ingress class.
	//
	// +kubebuilder:validation:Optional
	// +optional
	IngressClass string `json:"ingressClass,omitempty"`
}

type AWSLoadBalancerControllerStatusSubnets struct {
	// SubnetTagging indicates the current status of the subnet tags
	// +kubebuilder:validation:Optional
	// +optional
	SubnetTagging SubnetTaggingPolicy `json:"subnetTagging,omitempty"`

	// Internal is the list of subnet ids which have the tag `kubernetes.io/role/internal-elb`
	//
	// +kubebuilder:validation:Optional
	// +optional
	Internal []string `json:"internal,omitempty"`

	// Public is the list of subnet ids which have the tag `kubernetes.io/role/elb`
	//
	// +kubebuilder:validation:Optional
	// +optional
	Public []string `json:"public,omitempty"`

	// Tagged is the list of subnet ids which have been tagged by the operator
	//
	// +kubebuilder:validation:Optional
	// +optional
	Tagged []string `json:"tagged,omitempty"`

	// Untagged is the list of subnet ids which do not have any role tags
	//
	// +kubebuilder:validation:Optional
	// +optional
	Untagged []string `json:"untagged,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster

// AWSLoadBalancerController is the Schema for the awsloadbalancercontrollers API
type AWSLoadBalancerController struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AWSLoadBalancerControllerSpec   `json:"spec,omitempty"`
	Status AWSLoadBalancerControllerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AWSLoadBalancerControllerList contains a list of AWSLoadBalancerController
type AWSLoadBalancerControllerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AWSLoadBalancerController `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AWSLoadBalancerController{}, &AWSLoadBalancerControllerList{})
}
