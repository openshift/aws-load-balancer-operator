package awsloadbalancercontroller

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
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

func TestEnsureServiceAccount(t *testing.T) {
	managedTypesList := []client.ObjectList{
		&corev1.ServiceAccountList{},
	}

	eventWaitTimeout := time.Duration(1 * time.Second)

	testcases := []struct {
		name              string
		existingObjects   []runtime.Object
		expectedEvents    []test.Event
		errExpected       bool
		hasServiceAccount bool
	}{
		{
			name:              "Initial bootstrap, creation of serviceaccount",
			existingObjects:   make([]runtime.Object, 0),
			errExpected:       false,
			hasServiceAccount: true,
			expectedEvents: []test.Event{
				{
					EventType: watch.Added,
					ObjType:   "serviceaccount",
					NamespacedName: types.NamespacedName{
						Namespace: test.OperatorNamespace,
						Name:      "aws-load-balancer-controller-cluster",
					},
				},
			},
		},
		{
			name: "Pre-existing serviceaccount",
			existingObjects: []runtime.Object{
				testServiceAccount(),
			},
			errExpected:       false,
			hasServiceAccount: true,
			expectedEvents:    []test.Event{},
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

			_, err := r.ensureControllerServiceAccount(context.TODO(), r.Namespace, &albo.AWSLoadBalancerController{ObjectMeta: v1.ObjectMeta{Name: controllerName}})
			// error check
			if err != nil && !tc.errExpected {
				t.Fatalf("got unexpected error: %v", err)
			}

			if err == nil && tc.errExpected {
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

func testServiceAccount() *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:      "aws-load-balancer-controller-cluster",
			Namespace: test.OperatorNamespace,
		},
	}
}
