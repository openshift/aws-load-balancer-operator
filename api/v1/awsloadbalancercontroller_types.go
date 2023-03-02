/*
Copyright 2023.

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

package v1

import (
	configv1 "github.com/openshift/api/config/v1"
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

// AWSLoadBalancerControllerSpec defines the desired state of AWSLoadBalancerController.
type AWSLoadBalancerControllerSpec struct {
	// subnetTagging describes how the subnet tagging will be done by the operator.
	// Allowed values are "Auto" and "Manual".  The default value is "Auto".
	// When this field is set to "Auto", the operator will detect the subnets where the load balancers
	// will be provisioned and will continuously ensure the required resource tags are on those subnets
	// whereas when set to "Manual", this responsibility lies on the user. The tags added by the operator
	// will be removed when transitioning from "Auto" to "Manual" whereas any tags added by the user
	// will be left intact when transitioning from "Manual" to "Auto". For more info about the subnet detection
	// and the required subnet tags, see https://kubernetes-sigs.github.io/aws-load-balancer-controller/v2.4/deploy/subnet_discovery.
	//
	// +kubebuilder:default:=Auto
	// +kubebuilder:validation:Optional
	// +optional
	SubnetTagging SubnetTaggingPolicy `json:"subnetTagging,omitempty"`

	// additionalResourceTags are the AWS tags that will be applied to all AWS resources managed by this
	// controller. The managed AWS resources don't include the cluster subnets which are tagged by the operator.
	// The addition of new tags as well as the update or removal of any existing tags
	// will be propagated to the AWS resources. The controller owns all the tags of the managed AWS resources,
	// unsolicited tags are removed. The controller doesn't watch for changes on AWS, so the removal of the unsolicited
	// tags can only be triggered by an event coming from OpenShift. AWS supports a maximum of 50 tags per resource.
	// AWSLoadBalancerController reserves 3 tags for its use, the rest is split between the tag annotation
	// which can be set on the ingress and this field: 23 and 24, respectively. Each tag key must be unique.
	//
	// +kubebuilder:validation:MaxItems=24
	// +kubebuilder:validation:Optional
	// +optional
	// +listType=map
	// +listMapKey=key
	// +patchMergeKey=key
	// +patchStrategy=merge
	AdditionalResourceTags []AWSResourceTag `json:"additionalResourceTags,omitempty" patchStrategy:"merge" patchMergeKey:"key"`

	// ingressClass specifies the Ingress class which the controller will reconcile.
	// This Ingress class will be created unless it already exists.
	// The value will default to "alb". The defaulting to "alb" is necessary
	// so that this controller can function as expected in parallel with openshift-router,
	// for more info see https://github.com/openshift/enhancements/blob/master/enhancements/ingress/aws-load-balancer-operator.md#parallel-operation-of-the-openshift-router-and-lb-controller.
	//
	// +kubebuilder:default:=alb
	// +kubebuilder:validation:Optional
	// +optional
	IngressClass string `json:"ingressClass,omitempty"`

	// config specifies further customization options for the controller's deployment spec.
	//
	// +kubebuilder:validation:Optional
	// +optional
	Config *AWSLoadBalancerDeploymentConfig `json:"config,omitempty"`

	// enabledAddons describes the AWS services that can be integrated with
	// the AWS Load Balancers created by the controller.
	// Enabling an addon does not enable the feature on the ingress resources.
	// Instead, it allows for configuration of the feature through the ingress
	// annotations. Similarly, removing an addon does not disable the feature
	// which is already configured on the ingress resources. Instead, it starts ignoring
	// any configuration of the disabled feature through the annotations.
	// For more info on the addon annotations see
	// https://kubernetes-sigs.github.io/aws-load-balancer-controller/v2.4/guide/ingress/annotations/#addons.
	//
	// +kubebuilder:validation:Optional
	// +optional
	EnabledAddons []AWSAddon `json:"enabledAddons,omitempty"`

	// credentials is a reference to a secret containing
	// the AWS credentials to be used by the controller.
	// The secret is required to have a "credentials" data key
	// containing the AWS CLI credentials file (static or STS),
	// for examples, see https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-files.html
	// under the "credentials" data key.
	// The secret is required to be in the operator namespace.
	// If this field is empty, the credentials will be
	// requested using the Cloud Credentials API,
	// see https://docs.openshift.com/container-platform/4.11/authentication/managing_cloud_provider_credentials/about-cloud-credential-operator.html.
	//
	// +kubebuilder:validation:Optional
	// +optional
	Credentials *configv1.SecretNameReference `json:"credentials,omitempty"`
}

// AWSResourceTag is a tag to apply to AWS resources created by the controller.
type AWSResourceTag struct {
	// key is the key of the tag.
	// See https://docs.aws.amazon.com/tag-editor/latest/userguide/tagging.html#tag-conventions
	// for information on the tagging conventions.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	// +kubebuilder:validation:Pattern=`^[0-9A-Za-z_.:/=+-@]+$`
	// +required
	Key string `json:"key"`

	// value is the value of the tag.
	// See https://docs.aws.amazon.com/tag-editor/latest/userguide/tagging.html#tag-conventions
	// for information on the tagging conventions.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=256
	// +kubebuilder:validation:Pattern=`^[0-9A-Za-z_.:/=+-@]*$`
	// +required
	Value string `json:"value"`
}

// AWSLoadBalancerDeploymentConfig defines customization options for the controller's deployment spec.
type AWSLoadBalancerDeploymentConfig struct {
	// replicas is the desired number of the controller replicas.
	// The controller exposes webhooks for the IngressClassParams and TargetGroupBinding custom resources.
	// At least 1 replica of the controller should be ready to serve the webhook requests.
	// For that reason the replicas cannot be set to 0.
	// The leader election is enabled on the controller if the number of replicas is greater than 1.
	//
	// +kubebuilder:default:=1
	// +kubebuilder:validation:Minimum:=1
	// +kubebuilder:validation:Optional
	// +optional
	Replicas int32 `json:"replicas,omitempty"`
}

// AWSLoadBalancerControllerStatus defines the observed state of AWSLoadBalancerController.
type AWSLoadBalancerControllerStatus struct {
	// conditions is a list of operator-specific conditions and their status.
	//
	// +kubebuilder:validation:Optional
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// observedGeneration is the most recent generation observed.
	//
	// +kubebuilder:validation:Optional
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// subnets contains the cluster subnet details which matter for the controller.
	// The cluster subnets are those having `kubernetes.io/cluster/${cluster-name}` tag.
	// For more info on the cluster subnets which matter for the controller see
	// https://kubernetes-sigs.github.io/aws-load-balancer-controller/v2.4/deploy/subnet_discovery.
	//
	// +kubebuilder:validation:Optional
	// +optional
	Subnets *AWSLoadBalancerControllerStatusSubnets `json:"subnets,omitempty"`

	// ingressClass is the Ingress class currently used by the controller.
	//
	// +kubebuilder:validation:Optional
	// +optional
	IngressClass string `json:"ingressClass,omitempty"`
}

// AWSLoadBalancerControllerStatusSubnets contains the cluster subnet details
// which matter for the right functioning of the controller.
type AWSLoadBalancerControllerStatusSubnets struct {
	// subnetTagging indicates the current status of the subnet tags.
	//
	// +kubebuilder:validation:Optional
	// +optional
	SubnetTagging SubnetTaggingPolicy `json:"subnetTagging,omitempty"`

	// internal is the list of subnet ids which belong to the cluster
	// and have the tag `kubernetes.io/role/internal-elb`.
	//
	// +kubebuilder:validation:Optional
	// +optional
	Internal []string `json:"internal,omitempty"`

	// public is the list of subnet ids which belong to the cluster
	// and have the tag `kubernetes.io/role/elb`.
	//
	// +kubebuilder:validation:Optional
	// +optional
	Public []string `json:"public,omitempty"`

	// tagged is the list of subnet ids which belong to the cluster
	// and have been tagged by the operator.
	//
	// +kubebuilder:validation:Optional
	// +optional
	Tagged []string `json:"tagged,omitempty"`

	// untagged is the list of subnet ids which belong to the cluster
	// and do not have any role tags.
	//
	// +kubebuilder:validation:Optional
	// +optional
	Untagged []string `json:"untagged,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster
//+kubebuilder:storageversion

// AWSLoadBalancerController is the Schema for the awsloadbalancercontrollers API.
type AWSLoadBalancerController struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AWSLoadBalancerControllerSpec   `json:"spec,omitempty"`
	Status AWSLoadBalancerControllerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AWSLoadBalancerControllerList contains a list of AWSLoadBalancerController.
type AWSLoadBalancerControllerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AWSLoadBalancerController `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AWSLoadBalancerController{}, &AWSLoadBalancerControllerList{})
}
