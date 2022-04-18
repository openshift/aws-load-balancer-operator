package awsloadbalancercontroller

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
	"github.com/openshift/aws-load-balancer-operator/pkg/controllers/utils/test"
)

func TestUpdateIngressClassStatus(t *testing.T) {
	for _, tc := range []struct {
		name              string
		controller        *albo.AWSLoadBalancerController
		inputIngressClass string
	}{
		{
			name: "new class",
			controller: &albo.AWSLoadBalancerController{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
			},
			inputIngressClass: "alb",
		},
		{
			name: "updated class",
			controller: &albo.AWSLoadBalancerController{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Status:     albo.AWSLoadBalancerControllerStatus{IngressClass: "alb"},
			},
			inputIngressClass: "alb2",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &AWSLoadBalancerControllerReconciler{
				Client: fake.NewClientBuilder().WithScheme(test.Scheme).WithObjects(tc.controller).Build(),
			}
			err := r.updateStatusIngressClass(context.Background(), tc.controller, tc.inputIngressClass)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			controller, _, err := r.getAWSLoadBalancerController(context.Background(), tc.controller.Name)
			if err != nil {
				t.Fatalf("failed to get controller %q: %v", tc.controller.Name, err)
			}
			if controller.Status.IngressClass != tc.inputIngressClass {
				t.Errorf("unexpected ingress class in status, expected %q, got %q", tc.inputIngressClass, controller.Status.IngressClass)
			}
		})
	}
}

func TestUpdateSubnets(t *testing.T) {
	for _, tc := range []struct {
		name                               string
		controller                         *albo.AWSLoadBalancerController
		internal, public, untagged, tagged []string
		taggingPolicy                      albo.SubnetTaggingPolicy
	}{
		{
			name: "no previous values",
			controller: &albo.AWSLoadBalancerController{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
			},
			internal:      []string{"internal-1", "internal-2"},
			public:        []string{"public-1", "public-2"},
			tagged:        []string{"internal-1"},
			untagged:      []string{"unknown-1"},
			taggingPolicy: albo.ManualSubnetTaggingPolicy,
		},
		{
			name: "existing values",
			controller: &albo.AWSLoadBalancerController{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Status: albo.AWSLoadBalancerControllerStatus{
					Subnets: &albo.AWSLoadBalancerControllerStatusSubnets{
						SubnetTagging: albo.AutoSubnetTaggingPolicy,
						Internal:      []string{"internal-1"},
						Public:        []string{},
						Tagged:        []string{},
						Untagged:      []string{"unknown-1"},
					},
				},
			},
			internal:      []string{"internal-1", "internal-2"},
			public:        []string{"public-1", "public-2"},
			tagged:        []string{"internal-1"},
			untagged:      []string{"unknown-1"},
			taggingPolicy: albo.ManualSubnetTaggingPolicy,
		},
		{
			name: "nil input values",
			controller: &albo.AWSLoadBalancerController{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Status: albo.AWSLoadBalancerControllerStatus{
					Subnets: &albo.AWSLoadBalancerControllerStatusSubnets{
						SubnetTagging: albo.AutoSubnetTaggingPolicy,
						Internal:      []string{"internal-1"},
						Public:        []string{},
						Tagged:        []string{},
						Untagged:      []string{"unknown-1"},
					},
				},
			},
			internal:      []string{"internal-1", "internal-2"},
			public:        []string{"public-1", "public-2"},
			tagged:        []string{"internal-1", "public-2"},
			taggingPolicy: albo.ManualSubnetTaggingPolicy,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &AWSLoadBalancerControllerReconciler{
				Client: fake.NewClientBuilder().WithScheme(test.Scheme).WithObjects(tc.controller).Build(),
			}
			err := r.updateStatusSubnets(context.Background(), tc.controller, tc.internal, tc.public, tc.untagged, tc.tagged, tc.taggingPolicy)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			controller, _, err := r.getAWSLoadBalancerController(context.Background(), tc.controller.Name)
			if err != nil {
				t.Fatalf("failed to get controller %q: %v", tc.controller.Name, err)
			}
			if !equalStringSlices(tc.internal, controller.Status.Subnets.Internal) {
				t.Errorf("unexpected internal subnets, expected %v, got %v", tc.internal, controller.Status.Subnets.Internal)
			}
			if !equalStringSlices(tc.public, controller.Status.Subnets.Public) {
				t.Errorf("unexpected public subnets, expected %v, got %v", tc.internal, controller.Status.Subnets.Public)
			}
			if !equalStringSlices(tc.tagged, controller.Status.Subnets.Tagged) {
				t.Errorf("unexpected tagged subnets, expected %v, got %v", tc.tagged, controller.Status.Subnets.Tagged)
			}
			if !equalStringSlices(tc.untagged, controller.Status.Subnets.Untagged) {
				t.Errorf("unexpected untagged subnets, expected %v, got %v", tc.untagged, controller.Status.Subnets.Untagged)
			}
			if tc.taggingPolicy != controller.Status.Subnets.SubnetTagging {
				t.Errorf("unexpected tagging policy, expected %q, got %q", tc.taggingPolicy, controller.Status.Subnets.SubnetTagging)
			}
		})
	}
}

func equalStringSlices(a, b []string) bool {
	options := cmp.Options{
		cmpopts.SortSlices(func(a, b string) bool { return strings.Compare(a, b) < 0 }),
	}
	return cmp.Equal(a, b, options)
}

func TestUpdateStatus(t *testing.T) {
	for _, tc := range []struct {
		name               string
		controller         *albo.AWSLoadBalancerController
		deployment         *appsv1.Deployment
		credentialsRequest *cco.CredentialsRequest
		secretProvisioned  bool
		conditions         []metav1.Condition
	}{
		{
			name: "deployment and credentialsrequest available and up-to-date",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec:       appsv1.DeploymentSpec{Replicas: pointer.Int32(1)},
				Status:     appsv1.DeploymentStatus{AvailableReplicas: 1, UpdatedReplicas: 1},
			},
			credentialsRequest: &cco.CredentialsRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Generation: 1},
				Status:     cco.CredentialsRequestStatus{LastSyncGeneration: 1, Provisioned: true},
			},
			secretProvisioned: true,
			controller: &albo.AWSLoadBalancerController{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Generation: 5},
			},
			conditions: []metav1.Condition{
				{
					Type:               CredentialsRequestAvailable,
					Status:             metav1.ConditionTrue,
					Reason:             "CredentialsRequestProvisioned",
					Message:            `CredentialsRequest "test" has been provisioned`,
					ObservedGeneration: 5,
				},
				{
					Type:               DeploymentAvailableCondition,
					Reason:             "AllDeploymentReplicasAvailable",
					Message:            `Number of desired and available replicas of deployment "test" are equal`,
					ObservedGeneration: 5,
					Status:             metav1.ConditionTrue,
				},
				{
					Type:               DeploymentUpgradingCondition,
					Reason:             "AllDeploymentReplicasUpdated",
					Message:            `Number of desired and updated replicas of deployment "test" are equal`,
					ObservedGeneration: 5,
					Status:             metav1.ConditionFalse,
				},
			},
		},
		{
			name:       "deployment upgrading",
			controller: &albo.AWSLoadBalancerController{ObjectMeta: metav1.ObjectMeta{Name: "test", Generation: 5}},
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec:       appsv1.DeploymentSpec{Replicas: pointer.Int32Ptr(2)},
				Status:     appsv1.DeploymentStatus{AvailableReplicas: 2, UpdatedReplicas: 1},
			},
			credentialsRequest: &cco.CredentialsRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Generation: 3},
				Status: cco.CredentialsRequestStatus{
					Provisioned: true, LastSyncGeneration: 3,
				},
			},
			secretProvisioned: true,
			conditions: []metav1.Condition{
				{
					Type:               CredentialsRequestAvailable,
					Status:             metav1.ConditionTrue,
					Reason:             "CredentialsRequestProvisioned",
					Message:            `CredentialsRequest "test" has been provisioned`,
					ObservedGeneration: 5,
				},
				{
					Type:               DeploymentAvailableCondition,
					Reason:             "AllDeploymentReplicasAvailable",
					Message:            `Number of desired and available replicas of deployment "test" are equal`,
					ObservedGeneration: 5,
					Status:             metav1.ConditionTrue,
				},
				{
					Type:               DeploymentUpgradingCondition,
					Reason:             "AllDeploymentReplicasNotUpdated",
					Message:            `Number of desired and updated replicas of deployment "test" are not equal`,
					ObservedGeneration: 5,
					Status:             metav1.ConditionTrue,
				},
			},
		},
		{
			name: "deployment is not available",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec:       appsv1.DeploymentSpec{Replicas: pointer.Int32(2)},
				Status:     appsv1.DeploymentStatus{AvailableReplicas: 1, UpdatedReplicas: 2},
			},
			credentialsRequest: &cco.CredentialsRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Generation: 1},
				Status:     cco.CredentialsRequestStatus{LastSyncGeneration: 1, Provisioned: true},
			},
			secretProvisioned: true,
			controller: &albo.AWSLoadBalancerController{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Generation: 5},
			},
			conditions: []metav1.Condition{
				{
					Type:               CredentialsRequestAvailable,
					Status:             metav1.ConditionTrue,
					Reason:             "CredentialsRequestProvisioned",
					Message:            `CredentialsRequest "test" has been provisioned`,
					ObservedGeneration: 5,
				},
				{
					Type:               DeploymentAvailableCondition,
					Reason:             "AllDeploymentReplicasNotAvailable",
					Message:            `Number of desired and available replicas of deployment "test" are not equal`,
					ObservedGeneration: 5,
					Status:             metav1.ConditionFalse,
				},
				{
					Type:               DeploymentUpgradingCondition,
					Reason:             "AllDeploymentReplicasUpdated",
					Message:            `Number of desired and updated replicas of deployment "test" are equal`,
					ObservedGeneration: 5,
					Status:             metav1.ConditionFalse,
				},
			},
		},
		{
			name: "credential request is not available",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec:       appsv1.DeploymentSpec{Replicas: pointer.Int32(1)},
				Status:     appsv1.DeploymentStatus{AvailableReplicas: 1, UpdatedReplicas: 1},
			},
			credentialsRequest: &cco.CredentialsRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Generation: 1},
				Status:     cco.CredentialsRequestStatus{LastSyncGeneration: 1, Provisioned: false},
			},
			secretProvisioned: false,
			controller: &albo.AWSLoadBalancerController{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Generation: 5},
			},
			conditions: []metav1.Condition{
				{
					Type:               CredentialsRequestAvailable,
					Status:             metav1.ConditionFalse,
					Reason:             "CredentialsRequestNotProvisioned",
					Message:            `CredentialsRequest "test" has not yet been provisioned`,
					ObservedGeneration: 5,
				},
				{
					Type:               DeploymentAvailableCondition,
					Reason:             "AllDeploymentReplicasAvailable",
					Message:            `Number of desired and available replicas of deployment "test" are equal`,
					ObservedGeneration: 5,
					Status:             metav1.ConditionTrue,
				},
				{
					Type:               DeploymentUpgradingCondition,
					Reason:             "AllDeploymentReplicasUpdated",
					Message:            `Number of desired and updated replicas of deployment "test" are equal`,
					ObservedGeneration: 5,
					Status:             metav1.ConditionFalse,
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := AWSLoadBalancerControllerReconciler{
				Client: fake.NewClientBuilder().WithScheme(test.Scheme).WithObjects(tc.controller).Build(),
			}
			err := r.updateControllerStatus(context.Background(), tc.controller, tc.deployment, tc.credentialsRequest, tc.secretProvisioned)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			controller, _, err := r.getAWSLoadBalancerController(context.Background(), tc.controller.Name)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			stompTime := cmp.Transformer("", func(metav1.Time) metav1.Time {
				return metav1.Time{}
			})
			if diff := cmp.Diff(tc.conditions, controller.Status.Conditions, stompTime); diff != "" {
				t.Errorf("expected controller status conditions are different:\n%s", diff)
			}
		})
	}
}
