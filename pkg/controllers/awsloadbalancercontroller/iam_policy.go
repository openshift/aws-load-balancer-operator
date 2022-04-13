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
				Effect:          "Allow",
				Resource:        "*",
				PolicyCondition: cco.IAMPolicyCondition{},
				Action: []string{
					"acm:DescribeCertificate",
					"acm:ListCertificates",
					"cognito-idp:DescribeUserPoolClient",
					"ec2:AuthorizeSecurityGroupIngress",
					"ec2:CreateSecurityGroup",
					"ec2:CreateTags",
					"ec2:DeleteSecurityGroup",
					"ec2:DeleteTags",
					"ec2:Describe*",
					"ec2:GetCoipPoolUsage",
					"ec2:RevokeSecurityGroupIngress",
					"elasticloadbalancing:AddListenerCertificates",
					"elasticloadbalancing:AddTags",
					"elasticloadbalancing:CreateListener",
					"elasticloadbalancing:CreateLoadBalancer",
					"elasticloadbalancing:CreateRule",
					"elasticloadbalancing:CreateTargetGroup",
					"elasticloadbalancing:DeleteListener",
					"elasticloadbalancing:DeleteLoadBalancer",
					"elasticloadbalancing:DeleteRule",
					"elasticloadbalancing:DeleteTargetGroup",
					"elasticloadbalancing:DeregisterTargets",
					"elasticloadbalancing:Describe*",
					"elasticloadbalancing:ModifyListener",
					"elasticloadbalancing:ModifyLoadBalancerAttributes",
					"elasticloadbalancing:ModifyRule",
					"elasticloadbalancing:ModifyTargetGroup",
					"elasticloadbalancing:ModifyTargetGroupAttributes",
					"elasticloadbalancing:RegisterTargets",
					"elasticloadbalancing:RemoveListenerCertificates",
					"elasticloadbalancing:RemoveTags",
					"elasticloadbalancing:SetIpAddressType",
					"elasticloadbalancing:SetSecurityGroups",
					"elasticloadbalancing:SetSubnets",
					"elasticloadbalancing:SetWebAcl",
					"iam:CreateServiceLinkedRole",
					"iam:GetServerCertificate",
					"iam:ListServerCertificates",
					"shield:CreateProtection",
					"shield:DeleteProtection",
					"shield:DescribeProtection",
					"shield:GetSubscriptionState",
					"waf-regional:AssociateWebACL",
					"waf-regional:DisassociateWebACL",
					"waf-regional:GetWebACL",
					"waf-regional:GetWebACLForResource",
					"wafv2:AssociateWebACL",
					"wafv2:DisassociateWebACL",
					"wafv2:GetWebACL",
					"wafv2:GetWebACLForResource",
				},
			},
		},
	}
}
