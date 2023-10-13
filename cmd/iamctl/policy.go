package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/template"
)

type iamPolicy struct {
	Version   string            `json:"Version,omitempty"`
	Statement []policyStatement `json:"Statement,omitempty"`
}
type policyStatement struct {
	Effect    string              `json:"Effect,omitempty"`
	Action    AWSValue            `json:"Action,omitempty"`
	Resource  AWSValue            `json:"Resource,omitempty"`
	Condition *iamPolicyCondition `json:"Condition,omitempty"`
}

type AWSValue []string

func (v *AWSValue) UnmarshalJSON(input []byte) error {
	var raw interface{}
	_ = json.Unmarshal(input, &raw)
	var elements []string

	switch item := raw.(type) {
	case string:
		elements = []string{item}
	case []interface{}:
		elements = make([]string, len(item))
		for i, it := range item {
			elements[i] = fmt.Sprintf("%s", it)
		}
	default:
		return fmt.Errorf("unsupported type %t in list", item)
	}
	*v = elements
	return nil
}

type iamPolicyCondition map[string]iamPolicyConditionKeyValue

type iamPolicyConditionKeyValue map[string]string

// compressionPrefixes defines a list of action prefixes in the policy
// that are going to be compressed using wildcards.
var compressionPrefixes = map[string]string{
	"ec2:Describe":                  "ec2:Describe*",
	"elasticloadbalancing:Describe": "elasticloadbalancing:Describe*",
}

func generateIAMPolicy(inputFile, output, outputCR, pkg string) {
	generateIAMPolicyFromTemplate(filetemplate, inputFile, output, pkg)
	if outputCR != "" {
		generateIAMPolicyFromTemplate(credentialsRequestTemplate, inputFile, outputCR, pkg)
	}
}

func generateIAMPolicyFromTemplate(filetemplate string, inputFile, output, pkg string) {
	tmpl, err := template.New("").Parse(filetemplate)
	if err != nil {
		panic(err)
	}

	jsFs, err := os.ReadFile(inputFile)
	if err != nil {
		panic(fmt.Errorf("failed to read input file %v", err))
	}

	policy := iamPolicy{}

	err = json.Unmarshal([]byte(jsFs), &policy)
	if err != nil {
		panic(fmt.Errorf("failed to parse policy JSON %v", err))
	}

	if !skipMinify {
		// Minifying here as a workaround for current limitations
		// in credential requests length (2048 max bytes).
		policy = minify(policy)
	}

	opFs, err := os.Create(output)
	if err != nil {
		panic(err)
	}

	var in bytes.Buffer
	err = tmpl.Execute(&in, struct {
		Package   string
		Statement []policyStatement
	}{Package: pkg, Statement: policy.Statement})
	if err != nil {
		panic(err)
	}

	_, err = in.WriteTo(opFs)
	if err != nil {
		panic(err)
	}
}

// Minify replaces strict actions allowed across all Amazon Resource Names (ARNs)
// with much weaker actions using wildcards(*). It also removes any resource and
// condition limits applied in the policy. This reduces policy granularity and relies on
// the operator for adhering to resource access. NOTE: Potential security concern.
func minify(policy iamPolicy) iamPolicy {
	var miniPolicy iamPolicy

	// removing duplicates if present and compressing actions.
	arns := make(map[string]bool)
	for _, statement := range policy.Statement {
		for _, action := range statement.Action {
			compressed := false
			for k, v := range compressionPrefixes {
				if strings.HasPrefix(action, k) {
					arns[v] = true
					compressed = true
					break
				}
			}
			if !compressed {
				arns[action] = true
			}

		}
	}

	actions := make([]string, 0, len(arns))
	for action := range arns {
		actions = append(actions, action)
	}

	sort.Strings(actions)

	miniPolicy.Version = policy.Version
	miniPolicy.Statement = []policyStatement{
		{
			Effect:   "Allow",
			Action:   actions,
			Resource: AWSValue{"*"},
		},
	}
	return miniPolicy
}
