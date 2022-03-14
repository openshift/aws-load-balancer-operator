package aws

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type testEC2Client struct {
	SubnetClient
	t           *testing.T
	clusterName string
	output      []string
}

func (c *testEC2Client) DescribeVpcs(ctx context.Context, input *ec2.DescribeVpcsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
	c.t.Helper()
	if len(input.Filters) != 1 {
		c.t.Fatalf("unexpected input filters")
	}
	if aws.ToString(input.Filters[0].Name) != "tag-key" {
		c.t.Errorf("unexpected filter name, expected %q, got %q", "tag-key", aws.ToString(input.Filters[0].Name))
	}

	if len(input.Filters[0].Values) != 1 {
		c.t.Errorf("unexpected number of filter values")
	}

	if !strings.Contains(input.Filters[0].Values[0], c.clusterName) {
		c.t.Errorf("filter value %s does not contain %s", input.Filters[0].Values[0], c.clusterName)
	}
	output := &ec2.DescribeVpcsOutput{
		Vpcs: nil,
	}
	for _, i := range c.output {
		output.Vpcs = append(output.Vpcs, ec2types.Vpc{
			VpcId: aws.String(i),
		})
	}
	return output, nil
}

func makeTestClient(t *testing.T, clusterName string, output []string) *testEC2Client {
	return &testEC2Client{
		t:           t,
		clusterName: clusterName,
		output:      output,
	}
}

func TestGetVPCId(t *testing.T) {
	for _, tc := range []struct {
		name          string
		clusterName   string
		expectedErr   string
		vpcIDs        []string
		expectedVPCID string
	}{
		{
			name:          "success case",
			clusterName:   "test-cluster",
			vpcIDs:        []string{"test-vpc"},
			expectedVPCID: "test-vpc",
		},
		{
			name:        "no matching vpc",
			clusterName: "test-cluster",
			expectedErr: `no VPC with tag "kubernetes.io/cluster/test-cluster" found`,
		},
		{
			name:        "multiple matching vpc",
			clusterName: "test-cluster",
			expectedErr: `multiple VPCs with tag "kubernetes.io/cluster/test-cluster" found`,
			vpcIDs:      []string{"test-vpc-1", "test-vpc-2"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vpcID, err := GetVPCId(context.Background(), makeTestClient(t, tc.clusterName, tc.vpcIDs), tc.clusterName)
			if tc.expectedErr != "" {
				if err == nil {
					t.Errorf("expected error %s, got nil", tc.expectedErr)
				}
				if !strings.Contains(err.Error(), tc.expectedErr) {
					t.Errorf("expected error to contain %q, instead error is %q", tc.expectedErr, err)
				}
				return
			}
			if tc.expectedErr == "" && err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if vpcID != tc.expectedVPCID {
				t.Errorf("expected VPC Id %q, got %q", tc.expectedVPCID, vpcID)
			}
		})
	}
}
