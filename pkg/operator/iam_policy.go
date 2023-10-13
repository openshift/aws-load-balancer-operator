package operator

import cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"

type IAMPolicy struct {
	Version   string
	Statement []cco.StatementEntry
}

func GetIAMPolicy() IAMPolicy {
	return IAMPolicy{
		Statement: []cco.StatementEntry{
			{
				Effect:          "Allow",
				Resource:        "*",
				PolicyCondition: cco.IAMPolicyCondition{},
				Action: []string{
					"ec2:DescribeSubnets",
				},
			},
			{
				Effect:          "Allow",
				Resource:        "arn:aws:ec2:*:*:subnet/*",
				PolicyCondition: cco.IAMPolicyCondition{},
				Action: []string{
					"ec2:CreateTags",
					"ec2:DeleteTags",
				},
			},
			{
				Effect:          "Allow",
				Resource:        "*",
				PolicyCondition: cco.IAMPolicyCondition{},
				Action: []string{
					"ec2:DescribeVpcs",
				},
			},
		},
	}
}
