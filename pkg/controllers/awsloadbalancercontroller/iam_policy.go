package awsloadbalancercontroller

import cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"

type IAMPolicy struct {
	Version   string
	Statement []cco.StatementEntry
}

func GetIAMPolicy() IAMPolicy {
	return IAMPolicy{Version: "2012-10-17", Statement: []cco.StatementEntry{{Effect: "Allow", Action: []string{"acm:*", "cognito-idp:*", "ec2:*", "elasticloadbalancing:*", "iam:*", "shield:*", "waf-regional:*", "wafv2:*"}, Resource: "*", PolicyCondition: nil}}}
}
