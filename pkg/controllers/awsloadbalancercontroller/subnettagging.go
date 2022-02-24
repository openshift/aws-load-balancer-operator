package awsloadbalancercontroller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"

	configv1 "github.com/openshift/api/config/v1"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
)

const (
	clusterOwnedTagKey = "kubernetes.io/cluster/%s"
	internalELBTagKey  = "kubernetes.io/role/internal-elb"
	publicELBTagKey    = "kubernetes.io/role/elb"
	tagKeyFilterName   = "tag-key"
	tagKeyALBOTagged   = "networking.olm.openshift.io/albo/tagged"
)

func (r *AWSLoadBalancerControllerReconciler) tagSubnets(ctx context.Context, controller *albo.AWSLoadBalancerController) error {
	// fetch the Infrastructure for the cluster ID
	infrastructureKey := types.NamespacedName{Name: "cluster"}
	var infra configv1.Infrastructure
	err := r.Get(ctx, infrastructureKey, &infra)
	if err != nil {
		return fmt.Errorf("failed to get Infrastructure '%s': %w", infrastructureKey, err)
	}
	if infra.Status.InfrastructureName == "" {
		return fmt.Errorf("could not get cluster ID from Infrastructure %s status", infrastructureKey)
	}
	clusterID := infra.Status.InfrastructureName

	// list the subnets which are tagged as owned by the cluster
	subnets, err := r.EC2Client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		Filters: []ec2types.Filter{
			{
				Name:   aws.String(tagKeyFilterName),
				Values: []string{fmt.Sprintf(clusterOwnedTagKey, clusterID)},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to list subnets for cluster id %s: %w", clusterID, err)
	}

	if len(subnets.Subnets) == 0 {
		return fmt.Errorf("no subnets with tag %s found", fmt.Sprintf(clusterOwnedTagKey, clusterID))
	}

	var (
		internalSubnets sets.String
		publicSubnets   sets.String
		untaggedSubnets sets.String
		taggedSubnets   sets.String
	)

	internalSubnets, publicSubnets, taggedSubnets, untaggedSubnets, err = classifySubnets(subnets.Subnets)
	if err != nil {
		return fmt.Errorf("failed to classify subnets of cluster %s: %w", clusterID, err)
	}

	switch controller.Spec.SubnetTagging {
	case albo.AutoSubnetTaggingPolicy:
		// in OpenShift all private subnets are tagged. So assume any untagged subnets are public
		// TODO: process the subnets based on whether they have attached internet gateways
		if untaggedSubnets.Len() > 0 {
			_, err = r.EC2Client.CreateTags(ctx, &ec2.CreateTagsInput{
				Resources: untaggedSubnets.List(),
				Tags: []ec2types.Tag{
					{
						Key:   aws.String(publicELBTagKey),
						Value: aws.String("1"),
					},
					{
						Key:   aws.String(tagKeyALBOTagged),
						Value: aws.String("1"),
					},
				},
			})
			if err != nil {
				return fmt.Errorf("failed to tag subnets %v: %w", untaggedSubnets, err)
			}
		}
		// the untagged subnets are now public subnets
		publicSubnets = publicSubnets.Union(untaggedSubnets)
		// marked the untagged subnets as now tagged
		taggedSubnets = taggedSubnets.Union(untaggedSubnets)
		// there are no untagged subnets now
		untaggedSubnets = sets.NewString()
	case albo.ManualSubnetTaggingPolicy:
		// if the tagging policy was changed to Manual then remove tags from previously tagged subnets
		if taggedSubnets.Len() > 0 {
			// when values are not specified with the tag name the tag value is not considered during tag removal
			_, err = r.EC2Client.DeleteTags(ctx, &ec2.DeleteTagsInput{
				Resources: taggedSubnets.List(),
				Tags: []ec2types.Tag{
					{
						Key: aws.String(publicELBTagKey),
					},
					{
						Key: aws.String(tagKeyALBOTagged),
					},
				},
			})
			if err != nil {
				return fmt.Errorf("failed to remove tags from currently tagged subnets %v: %w", taggedSubnets, err)
			}
		}
		// the previously tagged subnets are now untagged
		untaggedSubnets = untaggedSubnets.Union(taggedSubnets)
		// removed the subnets which were untagged from the public subnets
		publicSubnets = publicSubnets.Difference(taggedSubnets)
		// set the tagged subnets to empty
		taggedSubnets = sets.NewString()
	default:
		return fmt.Errorf("unknown subnetTaggingPolicy %s", controller.Spec.SubnetTagging)
	}

	return r.updateStatusSubnets(ctx, controller, internalSubnets.List(), publicSubnets.List(), untaggedSubnets.List(), taggedSubnets.List(), controller.Spec.SubnetTagging)
}

func classifySubnets(subnets []ec2types.Subnet) (sets.String, sets.String, sets.String, sets.String, error) {
	var (
		internal = sets.NewString()
		public   = sets.NewString()
		untagged = sets.NewString()
		tagged   = sets.NewString()
	)

	for _, s := range subnets {
		subnetID := aws.ToString(s.SubnetId)
		if hasTag(s.Tags, internalELBTagKey) {
			internal.Insert(subnetID)
		}
		if hasTag(s.Tags, publicELBTagKey) {
			if internal.Has(subnetID) {
				return nil, nil, nil, nil, fmt.Errorf("subnet %s has both both tags with keys %s and %s", subnetID, internalELBTagKey, publicELBTagKey)
			}
			public.Insert(subnetID)
			// only check operator tagging if the subnet is a public subnet
			if hasTag(s.Tags, tagKeyALBOTagged) {
				tagged.Insert(subnetID)
			}
		}
		if !internal.Has(subnetID) && !public.Has(subnetID) {
			untagged.Insert(subnetID)
		}
	}

	return internal, public, tagged, untagged, nil
}

func hasTag(tags []ec2types.Tag, key string) bool {
	for _, t := range tags {
		if aws.ToString(t.Key) == key {
			return true
		}
	}
	return false
}
