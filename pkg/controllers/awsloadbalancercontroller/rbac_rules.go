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
		{
			APIGroups: []string{"coordination.k8s.io"},
			Resources: []string{"leases"},
			Verbs:     []string{"create"},
		},
		{
			APIGroups:     []string{"coordination.k8s.io"},
			Resources:     []string{"leases"},
			ResourceNames: []string{"aws-load-balancer-controller-leader"},
			Verbs:         []string{"get", "update", "patch"},
		},
	}
}
