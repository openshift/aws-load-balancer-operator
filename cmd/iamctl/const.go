package main

const (
	filetemplate = `
package {{.Package}}

import cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"

type IAMPolicy struct {
	Version   string
	Statement []cco.StatementEntry
}

func GetIAMPolicy() IAMPolicy {
	return IAMPolicy{
		Statement: []cco.StatementEntry{
			{
				Effect: {{.Statement.Effect|printf "%q"}},
				Resource: {{range .Statement.Resource}}{{printf "%q" .}}{{end}},
				PolicyCondition: cco.IAMPolicyCondition{},
				Action: []string{
					{{range $index, $element := .Statement.Action -}}
					{{.|printf "%q"}},{{printf "\n"}}
					{{- end}}
				},
			},
		},
	}
}	
`
	credentialsRequestTemplate = `apiVersion: cloudcredential.openshift.io/v1
kind: CredentialsRequest
metadata:
  name: aws-load-balancer-controller
  namespace: openshift-cloud-credential-operator
spec:
  providerSpec:
    apiVersion: cloudcredential.openshift.io/v1
    kind: AWSProviderSpec
    statementEntries:
    - action:
      {{range $index, $element := .Statement.Action}}- {{$element}}
      {{end -}}

      effect: {{.Statement.Effect}}
      resource: {{range .Statement.Resource}}{{printf "%q" .}}{{end}}
  secretRef:
    name: aws-load-balancer-controller-manual-cluster
    namespace: aws-load-balancer-operator
  serviceAccountNames:
  - aws-load-balancer-controller-cluster
`
)
