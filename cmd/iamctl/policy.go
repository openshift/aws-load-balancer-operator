package main

import (
	"encoding/json"
	"fmt"
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
	json.Unmarshal(input, &raw)
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
