package main

const (
	policycondition = "PolicyCondition"
	resource        = "Resource"
	effect          = "Effect"
	action          = "Action"
	filetemplate    = `
package {{.}}

import cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"

type IAMPolicy struct {
	Version   string
	Statement []cco.StatementEntry
}

func GetIAMPolicy() IAMPolicy {
    return IAMPolicy{}
}
`
)
