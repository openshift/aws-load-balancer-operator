package awsloadbalancercontroller

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	configv1 "github.com/openshift/api/config/v1"
	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(albo.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme

	utilruntime.Must(configv1.Install(scheme))
	utilruntime.Must(cco.Install(scheme))
}

func TestClassifySubnet(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		inputSubnets            []ec2types.Subnet
		expectedPublicSubnets   []string
		expectedInternalSubnets []string
		expectedUntaggedSubnets []string
		expectedTaggedSubnets   []string
		expectedError           string
	}{
		{
			name: "mixed subnets",
			inputSubnets: []ec2types.Subnet{
				testSubnet("subnet-1", publicELBTagKey),
				testSubnet("subnet-2", internalELBTagKey),
				testSubnet("subnet-3", publicELBTagKey),
				testSubnet("subnet-4", internalELBTagKey),
				testSubnet("subnet-5"),
				testSubnet("subnet-6"),
			},
			expectedInternalSubnets: []string{"subnet-2", "subnet-4"},
			expectedPublicSubnets:   []string{"subnet-1", "subnet-3"},
			expectedUntaggedSubnets: []string{"subnet-5", "subnet-6"},
		},
		{
			name: "conflicting subnets",
			inputSubnets: []ec2types.Subnet{
				testSubnet("subnet-1", publicELBTagKey),
				testSubnet("subnet-2", publicELBTagKey, internalELBTagKey),
			},
			expectedError: "subnet subnet-2 has both tags with keys kubernetes.io/role/internal-elb and kubernetes.io/role/elb",
		},
		{
			name: "tagged subnets",
			inputSubnets: []ec2types.Subnet{
				testSubnet("subnet-1", publicELBTagKey, tagKeyALBOTagged),
				testSubnet("subnet-2", internalELBTagKey),
				testSubnet("subnet-3"),
			},
			expectedInternalSubnets: []string{"subnet-2"},
			expectedPublicSubnets:   []string{"subnet-1"},
			expectedTaggedSubnets:   []string{"subnet-1"},
			expectedUntaggedSubnets: []string{"subnet-3"},
		},
		{
			name: "ignore internal subnets with ALBO tag",
			inputSubnets: []ec2types.Subnet{
				testSubnet("subnet-1", publicELBTagKey, tagKeyALBOTagged),
				testSubnet("subnet-2", internalELBTagKey, tagKeyALBOTagged),
			},
			expectedInternalSubnets: []string{"subnet-2"},
			expectedPublicSubnets:   []string{"subnet-1"},
			expectedTaggedSubnets:   []string{"subnet-1"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			internal, public, tagged, untagged, err := classifySubnets(tc.inputSubnets)
			if tc.expectedError != "" {
				if err == nil {
					t.Errorf("expected error, got nil")
					return
				}
				if !strings.Contains(err.Error(), tc.expectedError) {
					t.Errorf("expected error %s, instead got %s", tc.expectedError, err.Error())
					return
				}
			}
			if !internal.Equal(sets.NewString(tc.expectedInternalSubnets...)) {
				t.Errorf("expected internal subnets %v, got %v", tc.expectedInternalSubnets, internal.List())
			}
			if !public.Equal(sets.NewString(tc.expectedPublicSubnets...)) {
				t.Errorf("expected public subnets %v, got %v", tc.expectedPublicSubnets, public.List())
			}
			if !untagged.Equal(sets.NewString(tc.expectedUntaggedSubnets...)) {
				t.Errorf("expected untagged subnets %v, got %v", tc.expectedUntaggedSubnets, untagged.List())
			}
			if !tagged.Equal(sets.NewString(tc.expectedTaggedSubnets...)) {
				t.Errorf("expected tagged subnets %v, got %v", tc.expectedTaggedSubnets, tagged.List())
			}
		})
	}
}

func testSubnet(name string, tagKeys ...string) ec2types.Subnet {
	s := ec2types.Subnet{
		SubnetId: aws.String(name),
	}
	for _, k := range tagKeys {
		s.Tags = append(s.Tags, ec2types.Tag{
			Key:   aws.String(k),
			Value: aws.String("1"),
		})
	}
	return s
}

func TestTagSubnets(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		currentSubnets              []ec2types.Subnet
		statusUntaggedSubnets       []string
		expectedTaggedSubnets       []string
		expectedUntaggedSubnets     []string
		taggingPolicy               albo.SubnetTaggingPolicy
		expectedPublicSubnets       []string
		expectedInternalSubnets     []string
		expectedCreateTagOperations []string
		expectedRemoveTagOperations []string
	}{
		{
			name: "auto tagging, no preexisting tagged subnets",
			currentSubnets: []ec2types.Subnet{
				testSubnet("subnet-1"),
				testSubnet("subnet-2", internalELBTagKey),
				testSubnet("subnet-3", publicELBTagKey),
			},
			taggingPolicy:               albo.AutoSubnetTaggingPolicy,
			expectedTaggedSubnets:       []string{"subnet-1"},
			expectedPublicSubnets:       []string{"subnet-1", "subnet-3"},
			expectedInternalSubnets:     []string{"subnet-2"},
			expectedCreateTagOperations: []string{"subnet-1"},
		},
		{
			name: "auto tagging, with preexisting tagged subnets",
			currentSubnets: []ec2types.Subnet{
				testSubnet("subnet-1", publicELBTagKey, tagKeyALBOTagged),
				testSubnet("subnet-2", internalELBTagKey),
				testSubnet("subnet-3", publicELBTagKey),
			},
			taggingPolicy:           albo.AutoSubnetTaggingPolicy,
			expectedTaggedSubnets:   []string{"subnet-1"},
			expectedPublicSubnets:   []string{"subnet-1", "subnet-3"},
			expectedInternalSubnets: []string{"subnet-2"},
		},
		{
			name: "manual tagging, with no preexisting tagged subnets",
			currentSubnets: []ec2types.Subnet{
				testSubnet("subnet-1"),
				testSubnet("subnet-2", internalELBTagKey),
				testSubnet("subnet-3", publicELBTagKey),
			},
			taggingPolicy:           albo.ManualSubnetTaggingPolicy,
			expectedInternalSubnets: []string{"subnet-2"},
			expectedPublicSubnets:   []string{"subnet-3"},
			expectedUntaggedSubnets: []string{"subnet-1"},
		},
		{
			name: "manual tagging, with preexisting tagged subnets",
			currentSubnets: []ec2types.Subnet{
				testSubnet("subnet-1", publicELBTagKey, tagKeyALBOTagged),
				testSubnet("subnet-2", internalELBTagKey),
				testSubnet("subnet-3", publicELBTagKey),
			},
			taggingPolicy:               albo.ManualSubnetTaggingPolicy,
			expectedInternalSubnets:     []string{"subnet-2"},
			expectedRemoveTagOperations: []string{"subnet-1"},
			expectedUntaggedSubnets:     []string{"subnet-1"},
			expectedPublicSubnets:       []string{"subnet-3"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			controller := testALBC(tc.taggingPolicy)
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
				testInfrastructure("test-cluster", "us-west-1"),
				controller,
			).Build()
			ec2Client := &testEC2Client{
				t:         t,
				subnets:   tc.currentSubnets,
				clusterID: "test-cluster",
			}
			r := &AWSLoadBalancerControllerReconciler{
				Client:    client,
				EC2Client: ec2Client,
			}

			err := r.tagSubnets(context.Background(), controller)
			if err != nil {
				t.Errorf("got unexpected error: %v", err)
				return
			}

			if !equalStrings(tc.expectedCreateTagOperations, ec2Client.taggedResources) {
				t.Errorf("expected subnets %v to be tagged, instead got %v", tc.expectedCreateTagOperations, ec2Client.taggedResources)
			}

			if !equalStrings(tc.expectedRemoveTagOperations, ec2Client.untaggedResources) {
				t.Errorf("expected subnets %v to have been untagged, instead got %v", tc.expectedRemoveTagOperations, ec2Client.untaggedResources)
			}

			var albc albo.AWSLoadBalancerController
			controllerKey := types.NamespacedName{Name: controllerName}
			err = client.Get(context.Background(), controllerKey, &albc)
			if err != nil {
				t.Errorf("unexpected error getting Infrastructure: %v", err)
				return
			}

			if albc.Status.Subnets.SubnetTagging != tc.taggingPolicy {
				t.Errorf("tagging policy in status not updated")
				return
			}
			if !equalStrings(tc.expectedPublicSubnets, albc.Status.Subnets.Public) {
				t.Errorf("expected public subnets %v, got %v", tc.expectedPublicSubnets, albc.Status.Subnets.Public)
			}
			if !equalStrings(tc.expectedInternalSubnets, albc.Status.Subnets.Internal) {
				t.Errorf("expected internal subnets %v, got %v", tc.expectedInternalSubnets, albc.Status.Subnets.Internal)
			}
			if !equalStrings(tc.expectedTaggedSubnets, albc.Status.Subnets.Tagged) {
				t.Errorf("expected tagged subnets %v, got %v", tc.expectedTaggedSubnets, albc.Status.Subnets.Tagged)
			}
			if !equalStrings(tc.expectedUntaggedSubnets, albc.Status.Subnets.Untagged) {
				t.Errorf("expected untagged subnets %v, got %v", tc.expectedUntaggedSubnets, albc.Status.Subnets.Untagged)
			}
		})
	}
}

func testInfrastructure(clusterName, awsRegion string) *configv1.Infrastructure {
	return &configv1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Status: configv1.InfrastructureStatus{
			InfrastructureName: clusterName,
			PlatformStatus: &configv1.PlatformStatus{
				AWS: &configv1.AWSPlatformStatus{
					Region: awsRegion,
				},
			},
		},
	}
}

func testALBC(taggingPolicy albo.SubnetTaggingPolicy) *albo.AWSLoadBalancerController {
	return &albo.AWSLoadBalancerController{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec:       albo.AWSLoadBalancerControllerSpec{SubnetTagging: taggingPolicy},
		Status: albo.AWSLoadBalancerControllerStatus{
			Subnets: &albo.AWSLoadBalancerControllerStatusSubnets{
				SubnetTagging: taggingPolicy,
			},
		},
	}
}

type testEC2Client struct {
	t                 *testing.T
	subnets           []ec2types.Subnet
	clusterID         string
	taggedResources   []string
	untaggedResources []string
}

var badQueryError = errors.New("bad query")

func (t *testEC2Client) DescribeSubnets(_ context.Context, input *ec2.DescribeSubnetsInput, _ ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
	t.t.Helper()
	if len(input.Filters) != 1 {
		t.t.Errorf("query does not have correct number of filters")
		return nil, badQueryError
	}
	if aws.ToString(input.Filters[0].Name) != tagKeyFilterName {
		t.t.Errorf("unexpected filter name %s", aws.ToString(input.Filters[0].Name))
		return nil, badQueryError
	}
	if len(input.Filters[0].Values) != 1 {
		t.t.Errorf("filter name %s does not have correct value", aws.ToString(input.Filters[0].Name))
		return nil, badQueryError
	}
	if input.Filters[0].Values[0] != fmt.Sprintf(clusterOwnedTagKey, t.clusterID) {
		t.t.Errorf("unexpected filter value %s for name %s", input.Filters[0].Values[0], aws.ToString(input.Filters[0].Name))
		return nil, badQueryError
	}
	return &ec2.DescribeSubnetsOutput{Subnets: t.subnets}, nil
}

func (t *testEC2Client) CreateTags(_ context.Context, input *ec2.CreateTagsInput, _ ...func(*ec2.Options)) (*ec2.CreateTagsOutput, error) {
	t.t.Helper()
	if len(input.Tags) != 2 {
		t.t.Errorf("unexpected number of tags: %d", len(input.Tags))
		return nil, badQueryError
	}
	if !hasTag(input.Tags, publicELBTagKey) {
		t.t.Errorf("input %v does not have tag key %s", input.Tags, publicELBTagKey)
		return nil, badQueryError
	}
	if !hasTag(input.Tags, tagKeyALBOTagged) {
		t.t.Errorf("input %v does not have tag key %s", input.Tags, tagKeyALBOTagged)
		return nil, badQueryError
	}
	t.taggedResources = append(t.taggedResources, input.Resources...)
	return nil, nil
}

func (t *testEC2Client) DeleteTags(ctx context.Context, input *ec2.DeleteTagsInput, _ ...func(*ec2.Options)) (*ec2.DeleteTagsOutput, error) {
	t.t.Helper()
	if len(input.Tags) != 2 {
		t.t.Errorf("unexpected number of tags: %d", len(input.Tags))
		return nil, badQueryError
	}
	if !hasTag(input.Tags, publicELBTagKey) {
		t.t.Errorf("input %v does not have tag key %s", input.Tags, publicELBTagKey)
		return nil, badQueryError
	}
	if !hasTag(input.Tags, tagKeyALBOTagged) {
		t.t.Errorf("input %v does not have tag key %s", input.Tags, tagKeyALBOTagged)
		return nil, badQueryError
	}
	t.untaggedResources = append(t.untaggedResources, input.Resources...)
	return nil, nil
}
