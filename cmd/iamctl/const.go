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
            {{range .Statement -}}
            {
				Effect: {{.Effect|printf "%q"}},
				Resource: {{range .Resource}}{{printf "%q" .}}{{end}},
				PolicyCondition: cco.IAMPolicyCondition{},
				Action: []string{
					{{range $index, $element := .Action -}}
					{{.|printf "%q"}},{{printf "\n"}}
					{{- end}}
				},
			},
            {{end}}
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
    {{range .Statement -}}
    - action:
      {{range $index, $element := .Action}}- {{$element}}
      {{end -}}

      effect: {{.Effect}}
      resource: {{range .Resource}}{{printf "%q" .}}{{end}}
    {{- end}}
  secretRef:
    name: aws-load-balancer-controller-manual-cluster
    namespace: aws-load-balancer-operator
  serviceAccountNames:
  - aws-load-balancer-controller-cluster
`
)
