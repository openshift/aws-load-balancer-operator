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

// +kubebuilder:validation:Enum=AWSShield;AWSAddonWAFv1;AWSAddonWAFv2
type AWSAddon string

const (
	AWSAddonShield AWSAddon = "AWSShield"
	AWSAddonWAFv1  AWSAddon = "AWSAddonWAFv1"
	AWSAddonWAFv2  AWSAddon = "AWSAddonWAFv2"
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
	SubnetTagging SubnetTaggingPolicy `json:"subnetTagging,omitempty"`

	// Default AWS Tags that will be applied to all AWS resources managed by this
	// controller (default []).
	//
	// This value is required so that this controller can function as expected
	// in parallel to openshift-router.
	//
	// +kubebuilder:default:={}
	AdditionalResourceTags map[string]string `json:"additionalResourceTags,omitempty"`

	// IngressClass specifies the Ingress class which the controller will reconcile.
	// This will default to "alb".
	//
	// +kubebuilder:default:=alb
	IngressClass string `json:"ingressClass,omitempty"`

	// Config specifies further customization options for the controller's deployment spec.
	//
	Config *AWSLoadBalancerDeploymentConfig `json:"config,omitempty"`

	// AWSAddon describes the AWS services that can be integrated with
	// the AWS Load Balancer.
	//
	EnabledAddons []AWSAddon `json:"enabledAddons,omitempty"` // indicates which AWS addons should be disabled.

}

type AWSLoadBalancerDeploymentConfig struct {

	// +kubebuilder:default:=2
	Replicas int32 `json:"replicas,omitempty"`
}

// AWSLoadBalancerControllerStatus defines the observed state of AWSLoadBalancerController.
type AWSLoadBalancerControllerStatus struct {

	// Conditions is a list of operator-specific conditions
	// and their status.
	//
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed.
	//
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
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
