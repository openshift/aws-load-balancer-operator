package awsloadbalancercontroller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1"
)

const (
	clusterOwnedTagKey = "kubernetes.io/cluster/%s"
	internalELBTagKey  = "kubernetes.io/role/internal-elb"
	publicELBTagKey    = "kubernetes.io/role/elb"
	tagKeyFilterName   = "tag-key"
	tagKeyALBOTagged   = "networking.olm.openshift.io/albo/tagged"
)

// tagSubnets will add detect the subnets of the cluster and then tag them appropriately. It then writes the detected
// subnet IDs into the status along with their tagged roles.
func (r *AWSLoadBalancerControllerReconciler) tagSubnets(ctx context.Context, controller *albo.AWSLoadBalancerController) (internalSubnets, publicSubnets, untaggedSubnets, taggedSubnets []string, err error) {
	// list the subnets which are tagged as owned by the cluster
	subnetsPaginator := ec2.NewDescribeSubnetsPaginator(r.EC2Client, &ec2.DescribeSubnetsInput{
		Filters: []ec2types.Filter{
			{
				Name:   aws.String(tagKeyFilterName),
				Values: []string{fmt.Sprintf(clusterOwnedTagKey, r.ClusterName)},
			},
		},
	})

	var (
		subnets  []ec2types.Subnet
		response *ec2.DescribeSubnetsOutput
	)
	for subnetsPaginator.HasMorePages() {
		response, err = subnetsPaginator.NextPage(ctx)
		if err != nil {
			err = fmt.Errorf("failed to list subnets for cluster id %s: %w", r.ClusterName, err)
			return
		}
		subnets = append(subnets, response.Subnets...)
	}

	if len(subnets) == 0 {
		err = fmt.Errorf("no subnets with tag %s found", fmt.Sprintf(clusterOwnedTagKey, r.ClusterName))
		return
	}

	var (
		internal sets.Set[string]
		public   sets.Set[string]
		untagged sets.Set[string]
		tagged   sets.Set[string]
	)

	internal, public, tagged, untagged, err = classifySubnets(subnets)
	if err != nil {
		err = fmt.Errorf("failed to classify subnets of cluster %s: %w", r.ClusterName, err)
		return
	}

	switch controller.Spec.SubnetTagging {
	case albo.AutoSubnetTaggingPolicy:
		// in OpenShift all private subnets are tagged. So assume any untagged subnets are public
		// TODO: process the subnets based on whether they have attached internet gateways
		if untagged.Len() > 0 {
			_, err = r.EC2Client.CreateTags(ctx, &ec2.CreateTagsInput{
				Resources: sets.List(untagged),
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
				err = fmt.Errorf("failed to tag subnets %v: %w", untaggedSubnets, err)
				return
			}
		}
		// the untagged subnets are now public subnets
		public = public.Union(untagged)
		// marked the untagged subnets as now tagged
		tagged = tagged.Union(untagged)
		// there are no untagged subnets now
		untagged = sets.New[string]()
	case albo.ManualSubnetTaggingPolicy:
		// if the tagging policy was changed to Manual then remove tags from previously tagged subnets
		if tagged.Len() > 0 {
			// when values are not specified with the tag name the tag value is not considered during tag removal
			_, err = r.EC2Client.DeleteTags(ctx, &ec2.DeleteTagsInput{
				Resources: sets.List(tagged),
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
				err = fmt.Errorf("failed to remove tags from currently tagged subnets %v: %w", taggedSubnets, err)
				return
			}
		}
		// the previously tagged subnets are now untagged
		untagged = untagged.Union(tagged)
		// removed the subnets which were untagged from the public subnets
		public = public.Difference(tagged)
		// set the tagged subnets to empty
		tagged = sets.New[string]()
	default:
		err = fmt.Errorf("unknown subnetTaggingPolicy %s", controller.Spec.SubnetTagging)
	}

	untaggedSubnets = sets.List(untagged)
	taggedSubnets = sets.List(tagged)
	publicSubnets = sets.List(public)
	internalSubnets = sets.List(internal)
	return
}

func classifySubnets(subnets []ec2types.Subnet) (sets.Set[string], sets.Set[string], sets.Set[string], sets.Set[string], error) {
	var (
		internal = sets.New[string]()
		public   = sets.New[string]()
		untagged = sets.New[string]()
		tagged   = sets.New[string]()
	)

	for _, s := range subnets {
		subnetID := aws.ToString(s.SubnetId)
		if hasTag(s.Tags, internalELBTagKey) {
			internal.Insert(subnetID)
		}
		if hasTag(s.Tags, publicELBTagKey) {
			if internal.Has(subnetID) {
				return nil, nil, nil, nil, fmt.Errorf("subnet %s has both tags with keys %s and %s", subnetID, internalELBTagKey, publicELBTagKey)
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
