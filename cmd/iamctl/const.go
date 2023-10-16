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
		{{- range .Statement }}
			{
				Effect: "{{ .Effect }}",
				Resource: {{ range  .Resource }}"{{ . }}"{{ end }},
				PolicyCondition: cco.IAMPolicyCondition{
 				{{- with .Condition }}
				{{- range $key, $value := . }}
					"{{ $key }}": cco.IAMPolicyConditionKeyValue{
					{{- range $innerKey, $innerValue := $value }}
						"{{ $innerKey }}": {{ stringOrSlice $innerValue false }},
					{{- end }}
					},
				{{- end }}
				{{- end }}
				},
				Action: []string{
				{{- range .Action }}
					"{{ . }}",
				{{- end }}
				},
			},
		{{- end }}
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
    {{- range .Statement }}
    - action:
      {{- range .Action }}
      - {{ . }}
      {{- end }}
      effect: {{ .Effect }}
      resource: {{range .Resource}}{{printf "%q" .}}{{end}}
      {{- with .Condition }}
      policyCondition:
        {{- range $key, $value := . }}
          "{{ $key }}":
            {{- range $innerKey, $innerValue := $value }}
              "{{ $innerKey }}": {{ stringOrSlice $innerValue true }}
            {{- end }}
        {{- end }}
      {{- end }}
    {{- end }}
  secretRef:
    name: aws-load-balancer-controller-cluster
    namespace: aws-load-balancer-operator
  serviceAccountNames:
  - aws-load-balancer-controller-cluster
`
)
