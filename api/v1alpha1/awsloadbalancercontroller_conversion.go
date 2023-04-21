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
	configv1 "github.com/openshift/api/config/v1"

	"sigs.k8s.io/controller-runtime/pkg/conversion"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	v1 "github.com/openshift/aws-load-balancer-operator/api/v1"
)

var albcWebhookLog = logf.Log.WithName("albc-conversion-webhook")

// ConvertTo converts this AWSLoadBalancerController to the Hub version (v1).
func (src *AWSLoadBalancerController) ConvertTo(dstRaw conversion.Hub) error {
	albcWebhookLog.Info("Converting to v1", "name", src.Name)

	dst := dstRaw.(*v1.AWSLoadBalancerController)

	// ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

	// Spec
	dst.Spec.SubnetTagging = v1.SubnetTaggingPolicy(src.Spec.SubnetTagging)
	for k, v := range src.Spec.AdditionalResourceTags {
		dst.Spec.AdditionalResourceTags = append(dst.Spec.AdditionalResourceTags, v1.AWSResourceTag{Key: k, Value: v})
	}
	dst.Spec.IngressClass = src.Spec.IngressClass
	if src.Spec.Config != nil {
		dst.Spec.Config = &v1.AWSLoadBalancerDeploymentConfig{
			Replicas: src.Spec.Config.Replicas,
		}
	}
	for _, addon := range src.Spec.EnabledAddons {
		dst.Spec.EnabledAddons = append(dst.Spec.EnabledAddons, v1.AWSAddon(addon))
	}
	if src.Spec.Credentials != nil {
		dst.Spec.Credentials = &configv1.SecretNameReference{
			Name: src.Spec.Credentials.Name,
		}
	}

	// Status
	dst.Status.Conditions = src.Status.Conditions
	dst.Status.ObservedGeneration = src.Status.ObservedGeneration
	if src.Status.Subnets != nil {
		dst.Status.Subnets = &v1.AWSLoadBalancerControllerStatusSubnets{
			SubnetTagging: v1.SubnetTaggingPolicy(src.Status.Subnets.SubnetTagging),
			Internal:      src.Status.Subnets.Internal,
			Public:        src.Status.Subnets.Public,
			Tagged:        src.Status.Subnets.Tagged,
			Untagged:      src.Status.Subnets.Untagged,
		}
	}
	dst.Status.IngressClass = src.Status.IngressClass

	return nil
}

// ConvertFrom converts from the Hub version (v1) to this version.
func (dst *AWSLoadBalancerController) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1.AWSLoadBalancerController)

	albcWebhookLog.Info("Converting to v1alpha1", "name", src.Name)

	// ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

	// Spec
	dst.Spec.SubnetTagging = SubnetTaggingPolicy(src.Spec.SubnetTagging)
	if len(src.Spec.AdditionalResourceTags) > 0 {
		dst.Spec.AdditionalResourceTags = map[string]string{}
	}
	for _, t := range src.Spec.AdditionalResourceTags {
		dst.Spec.AdditionalResourceTags[t.Key] = t.Value
	}
	dst.Spec.IngressClass = src.Spec.IngressClass
	if src.Spec.Config != nil {
		dst.Spec.Config = &AWSLoadBalancerDeploymentConfig{
			Replicas: src.Spec.Config.Replicas,
		}
	}
	for _, addon := range src.Spec.EnabledAddons {
		dst.Spec.EnabledAddons = append(dst.Spec.EnabledAddons, AWSAddon(addon))
	}
	if src.Spec.Credentials != nil {
		dst.Spec.Credentials = &SecretReference{
			Name: src.Spec.Credentials.Name,
		}
	}

	// Status
	dst.Status.Conditions = src.Status.Conditions
	dst.Status.ObservedGeneration = src.Status.ObservedGeneration
	if src.Status.Subnets != nil {
		dst.Status.Subnets = &AWSLoadBalancerControllerStatusSubnets{
			SubnetTagging: SubnetTaggingPolicy(src.Status.Subnets.SubnetTagging),
			Internal:      src.Status.Subnets.Internal,
			Public:        src.Status.Subnets.Public,
			Tagged:        src.Status.Subnets.Tagged,
			Untagged:      src.Status.Subnets.Untagged,
		}
	}
	dst.Status.IngressClass = src.Status.IngressClass

	return nil
}
