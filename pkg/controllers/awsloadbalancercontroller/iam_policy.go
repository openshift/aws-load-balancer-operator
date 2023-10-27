package awsloadbalancercontroller

import cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"

type IAMPolicy struct {
	Version   string
	Statement []cco.StatementEntry
}

func GetIAMPolicy() IAMPolicy {
	return IAMPolicy{
		Statement: []cco.StatementEntry{
			{
				Effect:   "Allow",
				Resource: "*",
				PolicyCondition: cco.IAMPolicyCondition{
					"StringEquals": cco.IAMPolicyConditionKeyValue{
						"iam:AWSServiceName": "elasticloadbalancing.amazonaws.com",
					},
				},
				Action: []string{
					"iam:CreateServiceLinkedRole",
				},
			},
			{
				Effect:          "Allow",
				Resource:        "*",
				PolicyCondition: cco.IAMPolicyCondition{},
				Action: []string{
					"ec2:DescribeAccountAttributes",
					"ec2:DescribeAddresses",
					"ec2:DescribeAvailabilityZones",
					"ec2:DescribeInternetGateways",
					"ec2:DescribeVpcs",
					"ec2:DescribeVpcPeeringConnections",
					"ec2:DescribeSubnets",
					"ec2:DescribeSecurityGroups",
					"ec2:DescribeInstances",
					"ec2:DescribeNetworkInterfaces",
					"ec2:DescribeTags",
					"ec2:GetCoipPoolUsage",
					"ec2:DescribeCoipPools",
					"elasticloadbalancing:DescribeLoadBalancers",
					"elasticloadbalancing:DescribeLoadBalancerAttributes",
					"elasticloadbalancing:DescribeListeners",
					"elasticloadbalancing:DescribeListenerCertificates",
					"elasticloadbalancing:DescribeSSLPolicies",
					"elasticloadbalancing:DescribeRules",
					"elasticloadbalancing:DescribeTargetGroups",
					"elasticloadbalancing:DescribeTargetGroupAttributes",
					"elasticloadbalancing:DescribeTargetHealth",
					"elasticloadbalancing:DescribeTags",
				},
			},
			{
				Effect:          "Allow",
				Resource:        "*",
				PolicyCondition: cco.IAMPolicyCondition{},
				Action: []string{
					"cognito-idp:DescribeUserPoolClient",
					"acm:ListCertificates",
					"acm:DescribeCertificate",
					"iam:ListServerCertificates",
					"iam:GetServerCertificate",
					"waf-regional:GetWebACL",
					"waf-regional:GetWebACLForResource",
					"waf-regional:AssociateWebACL",
					"waf-regional:DisassociateWebACL",
					"wafv2:GetWebACL",
					"wafv2:GetWebACLForResource",
					"wafv2:AssociateWebACL",
					"wafv2:DisassociateWebACL",
					"shield:GetSubscriptionState",
					"shield:DescribeProtection",
					"shield:CreateProtection",
					"shield:DeleteProtection",
				},
			},
			{
				Effect:          "Allow",
				Resource:        "*",
				PolicyCondition: cco.IAMPolicyCondition{},
				Action: []string{
					"ec2:AuthorizeSecurityGroupIngress",
					"ec2:RevokeSecurityGroupIngress",
				},
			},
			{
				Effect:          "Allow",
				Resource:        "*",
				PolicyCondition: cco.IAMPolicyCondition{},
				Action: []string{
					"ec2:CreateSecurityGroup",
				},
			},
			{
				Effect:   "Allow",
				Resource: "arn:aws:ec2:*:*:security-group/*",
				PolicyCondition: cco.IAMPolicyCondition{
					"Null": cco.IAMPolicyConditionKeyValue{
						"aws:RequestTag/elbv2.k8s.aws/cluster": "false",
					},
					"StringEquals": cco.IAMPolicyConditionKeyValue{
						"ec2:CreateAction": "CreateSecurityGroup",
					},
				},
				Action: []string{
					"ec2:CreateTags",
				},
			},
			{
				Effect:   "Allow",
				Resource: "arn:aws:ec2:*:*:security-group/*",
				PolicyCondition: cco.IAMPolicyCondition{
					"Null": cco.IAMPolicyConditionKeyValue{
						"aws:RequestTag/elbv2.k8s.aws/cluster":  "true",
						"aws:ResourceTag/elbv2.k8s.aws/cluster": "false",
					},
				},
				Action: []string{
					"ec2:CreateTags",
					"ec2:DeleteTags",
				},
			},
			{
				Effect:   "Allow",
				Resource: "*",
				PolicyCondition: cco.IAMPolicyCondition{
					"Null": cco.IAMPolicyConditionKeyValue{
						"aws:ResourceTag/elbv2.k8s.aws/cluster": "false",
					},
				},
				Action: []string{
					"ec2:AuthorizeSecurityGroupIngress",
					"ec2:RevokeSecurityGroupIngress",
					"ec2:DeleteSecurityGroup",
				},
			},
			{
				Effect:   "Allow",
				Resource: "*",
				PolicyCondition: cco.IAMPolicyCondition{
					"Null": cco.IAMPolicyConditionKeyValue{
						"aws:RequestTag/elbv2.k8s.aws/cluster": "false",
					},
				},
				Action: []string{
					"elasticloadbalancing:CreateLoadBalancer",
					"elasticloadbalancing:CreateTargetGroup",
				},
			},
			{
				Effect:          "Allow",
				Resource:        "*",
				PolicyCondition: cco.IAMPolicyCondition{},
				Action: []string{
					"elasticloadbalancing:CreateListener",
					"elasticloadbalancing:DeleteListener",
					"elasticloadbalancing:CreateRule",
					"elasticloadbalancing:DeleteRule",
				},
			},
			{
				Effect:   "Allow",
				Resource: "arn:aws:elasticloadbalancing:*:*:targetgroup/*/*",
				PolicyCondition: cco.IAMPolicyCondition{
					"Null": cco.IAMPolicyConditionKeyValue{
						"aws:RequestTag/elbv2.k8s.aws/cluster":  "true",
						"aws:ResourceTag/elbv2.k8s.aws/cluster": "false",
					},
				},
				Action: []string{
					"elasticloadbalancing:AddTags",
					"elasticloadbalancing:RemoveTags",
				},
			},
			{
				Effect:   "Allow",
				Resource: "arn:aws:elasticloadbalancing:*:*:loadbalancer/net/*/*",
				PolicyCondition: cco.IAMPolicyCondition{
					"Null": cco.IAMPolicyConditionKeyValue{
						"aws:RequestTag/elbv2.k8s.aws/cluster":  "true",
						"aws:ResourceTag/elbv2.k8s.aws/cluster": "false",
					},
				},
				Action: []string{
					"elasticloadbalancing:AddTags",
					"elasticloadbalancing:RemoveTags",
				},
			},
			{
				Effect:   "Allow",
				Resource: "arn:aws:elasticloadbalancing:*:*:loadbalancer/app/*/*",
				PolicyCondition: cco.IAMPolicyCondition{
					"Null": cco.IAMPolicyConditionKeyValue{
						"aws:RequestTag/elbv2.k8s.aws/cluster":  "true",
						"aws:ResourceTag/elbv2.k8s.aws/cluster": "false",
					},
				},
				Action: []string{
					"elasticloadbalancing:AddTags",
					"elasticloadbalancing:RemoveTags",
				},
			},
			{
				Effect:          "Allow",
				Resource:        "arn:aws:elasticloadbalancing:*:*:listener/net/*/*/*",
				PolicyCondition: cco.IAMPolicyCondition{},
				Action: []string{
					"elasticloadbalancing:AddTags",
					"elasticloadbalancing:RemoveTags",
				},
			},
			{
				Effect:          "Allow",
				Resource:        "arn:aws:elasticloadbalancing:*:*:listener/app/*/*/*",
				PolicyCondition: cco.IAMPolicyCondition{},
				Action: []string{
					"elasticloadbalancing:AddTags",
					"elasticloadbalancing:RemoveTags",
				},
			},
			{
				Effect:          "Allow",
				Resource:        "arn:aws:elasticloadbalancing:*:*:listener-rule/net/*/*/*",
				PolicyCondition: cco.IAMPolicyCondition{},
				Action: []string{
					"elasticloadbalancing:AddTags",
					"elasticloadbalancing:RemoveTags",
				},
			},
			{
				Effect:          "Allow",
				Resource:        "arn:aws:elasticloadbalancing:*:*:listener-rule/app/*/*/*",
				PolicyCondition: cco.IAMPolicyCondition{},
				Action: []string{
					"elasticloadbalancing:AddTags",
					"elasticloadbalancing:RemoveTags",
				},
			},
			{
				Effect:   "Allow",
				Resource: "arn:aws:elasticloadbalancing:*:*:targetgroup/*/*",
				PolicyCondition: cco.IAMPolicyCondition{
					"Null": cco.IAMPolicyConditionKeyValue{
						"aws:RequestTag/elbv2.k8s.aws/cluster": "false",
					},
					"StringEquals": cco.IAMPolicyConditionKeyValue{
						"elasticloadbalancing:CreateAction": []string{"CreateTargetGroup", "CreateLoadBalancer"},
					},
				},
				Action: []string{
					"elasticloadbalancing:AddTags",
				},
			},
			{
				Effect:   "Allow",
				Resource: "arn:aws:elasticloadbalancing:*:*:loadbalancer/net/*/*",
				PolicyCondition: cco.IAMPolicyCondition{
					"Null": cco.IAMPolicyConditionKeyValue{
						"aws:RequestTag/elbv2.k8s.aws/cluster": "false",
					},
					"StringEquals": cco.IAMPolicyConditionKeyValue{
						"elasticloadbalancing:CreateAction": []string{"CreateTargetGroup", "CreateLoadBalancer"},
					},
				},
				Action: []string{
					"elasticloadbalancing:AddTags",
				},
			},
			{
				Effect:   "Allow",
				Resource: "arn:aws:elasticloadbalancing:*:*:loadbalancer/app/*/*",
				PolicyCondition: cco.IAMPolicyCondition{
					"Null": cco.IAMPolicyConditionKeyValue{
						"aws:RequestTag/elbv2.k8s.aws/cluster": "false",
					},
					"StringEquals": cco.IAMPolicyConditionKeyValue{
						"elasticloadbalancing:CreateAction": []string{"CreateTargetGroup", "CreateLoadBalancer"},
					},
				},
				Action: []string{
					"elasticloadbalancing:AddTags",
				},
			},
			{
				Effect:   "Allow",
				Resource: "*",
				PolicyCondition: cco.IAMPolicyCondition{
					"Null": cco.IAMPolicyConditionKeyValue{
						"aws:ResourceTag/elbv2.k8s.aws/cluster": "false",
					},
				},
				Action: []string{
					"elasticloadbalancing:ModifyLoadBalancerAttributes",
					"elasticloadbalancing:SetIpAddressType",
					"elasticloadbalancing:SetSecurityGroups",
					"elasticloadbalancing:SetSubnets",
					"elasticloadbalancing:DeleteLoadBalancer",
					"elasticloadbalancing:ModifyTargetGroup",
					"elasticloadbalancing:ModifyTargetGroupAttributes",
					"elasticloadbalancing:DeleteTargetGroup",
				},
			},
			{
				Effect:          "Allow",
				Resource:        "arn:aws:elasticloadbalancing:*:*:targetgroup/*/*",
				PolicyCondition: cco.IAMPolicyCondition{},
				Action: []string{
					"elasticloadbalancing:RegisterTargets",
					"elasticloadbalancing:DeregisterTargets",
				},
			},
			{
				Effect:          "Allow",
				Resource:        "*",
				PolicyCondition: cco.IAMPolicyCondition{},
				Action: []string{
					"elasticloadbalancing:SetWebAcl",
					"elasticloadbalancing:ModifyListener",
					"elasticloadbalancing:AddListenerCertificates",
					"elasticloadbalancing:RemoveListenerCertificates",
					"elasticloadbalancing:ModifyRule",
				},
			},
		},
	}
}
