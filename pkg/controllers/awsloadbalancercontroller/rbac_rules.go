package awsloadbalancercontroller

import rbacv1 "k8s.io/api/rbac/v1"

// getLeaderElectionRules is a set of rules required for leader election by the controller.
func getLeaderElectionRules() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"configmaps"},
			Verbs:     []string{"create"},
		},
		{
			APIGroups:     []string{""},
			Resources:     []string{"configmaps"},
			ResourceNames: []string{"aws-load-balancer-controller-leader"},
			Verbs:         []string{"get", "update", "patch"},
		},
	}
}
