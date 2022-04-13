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
)
