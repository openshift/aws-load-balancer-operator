package main

import (
	"fmt"
	"sort"
	"strings"
)

// Minify replaces strict actions allowed across all Amazon Resource Names (ARNs)
// with much weaker actions using wildcards(*). It also removes any resource and
// condition limits applied in the policy. This reduces policy granularity and relies on
// the operator for adhering to resource access. NOTE: Potential security concern.
func minify(policy iamPolicy) iamPolicy {
	var miniPolicy iamPolicy

	arns := make(map[string]bool)
	for _, statement := range policy.Statement {

		for _, action := range statement.Action {
			arns[strings.Split(action, ":")[0]] = true
		}
	}

	actions := make([]string, 0, len(arns))

	for k := range arns {
		actions = append(actions, fmt.Sprintf("%s:*", k))
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
