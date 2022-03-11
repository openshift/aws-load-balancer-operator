package awsloadbalancercontroller

import rbacv1 "k8s.io/api/rbac/v1"

// getControllerRules is a set of consolidated rules required by the controller.
func getControllerRules() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"endpoints"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"events"},
			Verbs:     []string{"create", "patch"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"namespaces"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"nodes"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"pods/status"},
			Verbs:     []string{"patch", "update"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"secrets"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"services"},
			Verbs:     []string{"get", "list", "patch", "update", "watch"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"services/status"},
			Verbs:     []string{"patch", "update"},
		},
		{
			APIGroups: []string{"discovery.k8s.io"},
			Resources: []string{"endpointslices"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{"elbv2.k8s.aws"},
			Resources: []string{"ingressclassparams"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{"elbv2.k8s.aws"},
			Resources: []string{"targetgroupbindings"},
			Verbs:     []string{"create", "delete", "get", "list", "patch", "update", "watch"},
		},
		{
			APIGroups: []string{"elbv2.k8s.aws"},
			Resources: []string{"targetgroupbindings/status"},
			Verbs:     []string{"patch", "update"},
		},
		{
			APIGroups: []string{"extensions"},
			Resources: []string{"ingresses"},
			Verbs:     []string{"get", "list", "patch", "update", "watch"},
		},
		{
			APIGroups: []string{"extensions"},
			Resources: []string{"ingresses/status"},
			Verbs:     []string{"patch", "update"},
		},
		{
			APIGroups: []string{"networking.k8s.io"},
			Resources: []string{"ingressclasses"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{"networking.k8s.io"},
			Resources: []string{"ingressclasses"},
			Verbs:     []string{"get", "list", "patch", "update", "watch"},
		},
		{
			APIGroups: []string{"networking.k8s.io"},
			Resources: []string{"ingressclasses/status"},
			Verbs:     []string{"patch", "update"},
		},
		{
			APIGroups: []string{"networking.k8s.io"},
			Resources: []string{"ingresses"},
			Verbs:     []string{"get", "list", "patch", "update", "watch"},
		},
		{
			APIGroups: []string{"networking.k8s.io"},
			Resources: []string{"ingresses/status"},
			Verbs:     []string{"patch", "update"},
		},
		{
			APIGroups: []string{"elbv2.k8s.aws"},
			Resources: []string{"ingressclassparams"},
			Verbs:     []string{"create", "delete", "get", "list", "patch", "update", "watch"},
		},
		{
			APIGroups: []string{"elbv2.k8s.aws"},
			Resources: []string{"ingressclassparams/status"},
			Verbs:     []string{"get"},
		},
		{
			APIGroups: []string{"elbv2.k8s.aws"},
			Resources: []string{"targetgroupbindings/status"},
			Verbs:     []string{"get"},
		},
	}
}

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
			ResourceNames: []string{"aws-load-balancer-contoller-leader"},
			Verbs:         []string{"get", "update", "patch"},
		},
	}
}
