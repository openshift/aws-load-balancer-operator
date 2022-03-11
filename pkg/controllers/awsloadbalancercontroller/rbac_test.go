package awsloadbalancercontroller

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/google/go-cmp/cmp"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
	"github.com/openshift/aws-load-balancer-operator/pkg/controllers/utils/test"
)

func TestEnsureClusterRolesAndBinding(t *testing.T) {
	managedTypesList := []client.ObjectList{
		&rbacv1.ClusterRoleList{},
		&rbacv1.RoleList{},
		&rbacv1.ClusterRoleBindingList{},
		&rbacv1.RoleBindingList{},
	}

	eventWaitTimeout := time.Duration(1 * time.Second)

	testcases := []struct {
		name            string
		existingObjects []runtime.Object
		expectedEvents  []test.Event
		errExpected     bool
	}{
		{
			name:            "Initial bootstrap, creation of all roles/clusterroles and their bindings",
			existingObjects: make([]runtime.Object, 0),
			errExpected:     false,
			expectedEvents: []test.Event{
				{
					EventType: watch.Added,
					ObjType:   "clusterrole",
					NamespacedName: types.NamespacedName{
						Name: commonResourceName,
					},
				},
				{
					EventType: watch.Added,
					ObjType:   "clusterrolebinding",
					NamespacedName: types.NamespacedName{
						Name: commonResourceName,
					},
				},
				{
					EventType: watch.Added,
					ObjType:   "role",
					NamespacedName: types.NamespacedName{
						Name:      commonResourceName,
						Namespace: test.OperatorNamespace,
					},
				},
				{
					EventType: watch.Added,
					ObjType:   "rolebinding",
					NamespacedName: types.NamespacedName{
						Name:      commonResourceName,
						Namespace: test.OperatorNamespace,
					},
				},
			},
		},
		{
			name: "Some clusterroles pre-exist",
			existingObjects: []runtime.Object{
				testPreExistingClusterRole(),
			},
			errExpected: false,
			expectedEvents: []test.Event{
				{
					EventType: watch.Added,
					ObjType:   "clusterrolebinding",
					NamespacedName: types.NamespacedName{
						Name: commonResourceName,
					},
				},
				{
					EventType: watch.Added,
					ObjType:   "role",
					NamespacedName: types.NamespacedName{
						Name:      commonResourceName,
						Namespace: test.OperatorNamespace,
					},
				},
				{
					EventType: watch.Added,
					ObjType:   "rolebinding",
					NamespacedName: types.NamespacedName{
						Name:      commonResourceName,
						Namespace: test.OperatorNamespace,
					},
				},
			},
		},
		{
			name: "Some roles pre-exist",
			existingObjects: []runtime.Object{
				testPreExistingRole(),
			},
			errExpected: false,
			expectedEvents: []test.Event{
				{
					EventType: watch.Added,
					ObjType:   "clusterrole",
					NamespacedName: types.NamespacedName{
						Name: commonResourceName,
					},
				},
				{
					EventType: watch.Added,
					ObjType:   "clusterrolebinding",
					NamespacedName: types.NamespacedName{
						Name: commonResourceName,
					},
				},
				{
					EventType: watch.Added,
					ObjType:   "rolebinding",
					NamespacedName: types.NamespacedName{
						Name:      commonResourceName,
						Namespace: test.OperatorNamespace,
					},
				},
			},
		},
		{
			name: "Some clusterroles pre-exist but contain old policies",
			existingObjects: []runtime.Object{
				testOutDatedPreExistingClusterRole(),
			},
			errExpected: false,
			expectedEvents: []test.Event{
				{
					EventType: watch.Modified,
					ObjType:   "clusterrole",
					NamespacedName: types.NamespacedName{
						Name: commonResourceName,
					},
				},
				{
					EventType: watch.Added,
					ObjType:   "clusterrolebinding",
					NamespacedName: types.NamespacedName{
						Name: commonResourceName,
					},
				},
				{
					EventType: watch.Added,
					ObjType:   "role",
					NamespacedName: types.NamespacedName{
						Name:      commonResourceName,
						Namespace: test.OperatorNamespace,
					},
				},
				{
					EventType: watch.Added,
					ObjType:   "rolebinding",
					NamespacedName: types.NamespacedName{
						Name:      commonResourceName,
						Namespace: test.OperatorNamespace,
					},
				},
			},
		},
		{
			name: "Some roles pre-exist but contain old policies",
			existingObjects: []runtime.Object{
				testOutDatedPreExistingRole(),
			},
			errExpected: false,
			expectedEvents: []test.Event{
				{
					EventType: watch.Added,
					ObjType:   "clusterrole",
					NamespacedName: types.NamespacedName{
						Name: commonResourceName,
					},
				},
				{
					EventType: watch.Added,
					ObjType:   "clusterrolebinding",
					NamespacedName: types.NamespacedName{
						Name: commonResourceName,
					},
				},
				{
					EventType: watch.Modified,
					ObjType:   "role",
					NamespacedName: types.NamespacedName{
						Name:      commonResourceName,
						Namespace: test.OperatorNamespace,
					},
				},
				{
					EventType: watch.Added,
					ObjType:   "rolebinding",
					NamespacedName: types.NamespacedName{
						Name:      commonResourceName,
						Namespace: test.OperatorNamespace,
					},
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			cl := fake.NewClientBuilder().WithScheme(test.Scheme).
				WithRuntimeObjects(tc.existingObjects...).
				Build()

			r := &AWSLoadBalancerControllerReconciler{
				Client:    cl,
				Namespace: test.OperatorNamespace,
				Image:     test.OperandImage,
				Scheme:    test.Scheme,
			}

			c := test.NewEventCollector(t, cl, managedTypesList, len(tc.expectedEvents))

			// get watch interfaces from all the type managed by the operator
			c.Start(context.TODO())
			defer c.Stop()

			err := r.ensureClusterRoleAndBinding(context.TODO(), &corev1.ServiceAccount{
				ObjectMeta: v1.ObjectMeta{
					Name:      controllerName,
					Namespace: test.OperatorNamespace,
				},
			}, &albo.AWSLoadBalancerController{ObjectMeta: v1.ObjectMeta{Name: controllerName}})
			if err != nil {
				if !tc.errExpected {
					t.Fatalf("got unexpected error: %v", err)
				}
			} else if tc.errExpected {
				t.Fatalf("error expected but not received")
			}

			// collect the events received from Reconcile()
			collectedEvents := c.Collect(len(tc.expectedEvents), eventWaitTimeout)

			// compare collected and expected events
			idxExpectedEvents := test.IndexEvents(tc.expectedEvents)
			idxCollectedEvents := test.IndexEvents(collectedEvents)
			if diff := cmp.Diff(idxExpectedEvents, idxCollectedEvents); diff != "" {
				t.Fatalf("found diff between expected and collected events: %s", diff)
			}
		})
	}
}

func testPreExistingClusterRole() *rbacv1.ClusterRole {
	return buildClusterRole(commonResourceName, getControllerRules())
}

func testOutDatedPreExistingClusterRole() *rbacv1.ClusterRole {
	return buildClusterRole(commonResourceName, []rbacv1.PolicyRule{})
}

func testPreExistingRole() *rbacv1.Role {
	return buildRole(commonResourceName, test.OperatorNamespace, getLeaderElectionRules())
}

func testOutDatedPreExistingRole() *rbacv1.Role {
	return buildRole(commonResourceName, test.OperatorNamespace, []rbacv1.PolicyRule{})
}
