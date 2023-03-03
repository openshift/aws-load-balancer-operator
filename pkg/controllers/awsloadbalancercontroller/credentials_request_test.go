package awsloadbalancercontroller

import (
	"context"
	"fmt"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"

	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"

	"github.com/google/go-cmp/cmp"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1"
	"github.com/openshift/aws-load-balancer-operator/pkg/controllers/utils/test"
)

const (
	testCredentialsRequestNamespace = credentialRequestNamespace
)

func TestEnsureCredentialsRequest(t *testing.T) {
	var (
		managedTypesList = []client.ObjectList{
			&cco.CredentialsRequestList{},
		}
		eventWaitTimeout = time.Duration(1 * time.Second)
		addEvent         = test.Event{
			EventType: watch.Added,
			ObjType:   "credentialsrequest",
			NamespacedName: types.NamespacedName{
				Namespace: testCredentialsRequestNamespace,
				Name:      "aws-load-balancer-controller-cluster",
			},
		}
		modifyEvent = test.Event{
			EventType: watch.Modified,
			ObjType:   "credentialsrequest",
			NamespacedName: types.NamespacedName{
				Namespace: testCredentialsRequestNamespace,
				Name:      "aws-load-balancer-controller-cluster",
			},
		}
	)

	testcases := []struct {
		name            string
		existingObjects []runtime.Object
		expectedEvents  []test.Event
		errExpected     bool
	}{
		{
			name:            "Initial bootstrap",
			existingObjects: make([]runtime.Object, 0),
			expectedEvents:  []test.Event{addEvent},
			errExpected:     false,
		},
		{
			name: "Change in Credential Request. ProviderSpec",
			existingObjects: []runtime.Object{
				testCredentialsRequestProviderSpecDiff(),
			},
			expectedEvents: []test.Event{modifyEvent},
			errExpected:    false,
		},
		{
			name: "Change in Credential Request. SecretName",
			existingObjects: []runtime.Object{
				testCredentialsRequestSecretNameDiff(),
			},
			expectedEvents: []test.Event{modifyEvent},
			errExpected:    false,
		},
		{
			name: "Change in Credential Request. SecretNamespace",
			existingObjects: []runtime.Object{
				testCredentialsRequestSecretNsDiff(),
			},
			expectedEvents: []test.Event{modifyEvent},
			errExpected:    false,
		},
		{
			name: "Change in Credential Request. ServiceAccounts",
			existingObjects: []runtime.Object{
				testCredentialsRequestSADiff(),
			},
			expectedEvents: []test.Event{modifyEvent},
			errExpected:    false,
		},
		{
			name: "No change in Credential Request",
			existingObjects: []runtime.Object{
				testCompleteCredentialsRequest(),
			},
			expectedEvents: []test.Event{},
			errExpected:    false,
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

			cr, err := r.ensureCredentialsRequest(context.TODO(), r.Namespace, &albo.AWSLoadBalancerController{ObjectMeta: metav1.ObjectMeta{Name: controllerName}})
			// error check
			if err != nil {
				if !tc.errExpected {
					t.Fatalf("got unexpected error: %v", err)
				}
			} else if tc.errExpected {
				t.Fatalf("error expected but not received")
			}

			expectedSecretName := fmt.Sprintf("%s-credentialsrequest-%s", controllerResourcePrefix, "cluster")
			if cr.Spec.SecretRef.Name != expectedSecretName {
				t.Errorf("unexpected CredentialsRequest secret name, expected %q, got %q", expectedSecretName, cr.Spec.SecretRef.Name)
			}
			if cr.Spec.SecretRef.Namespace != test.OperatorNamespace {
				t.Errorf("unexpected CredentialsRequest secret namespace, expected %q, got %q", test.OperatorNamespace, cr.Spec.SecretRef.Namespace)
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

func testCompleteCredentialsRequest() *cco.CredentialsRequest {
	codec, _ := cco.NewCodec()
	cfg, _ := createProviderConfig(codec)
	return &cco.CredentialsRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "aws-load-balancer-controller-cluster",
			Namespace: testCredentialsRequestNamespace,
		},
		Spec: cco.CredentialsRequestSpec{
			ProviderSpec:        cfg,
			SecretRef:           createCredentialsSecretRef("aws-load-balancer-controller-credentialsrequest-cluster", test.OperatorNamespace),
			ServiceAccountNames: []string{"aws-load-balancer-controller-cluster"},
		},
	}
}

func testCredentialsRequestProviderSpecDiff() *cco.CredentialsRequest {
	cr := testCompleteCredentialsRequest()
	cr.Spec.ProviderSpec = testAWSProviderSpec()
	return cr
}

func testCredentialsRequestSecretNameDiff() *cco.CredentialsRequest {
	cr := testCompleteCredentialsRequest()
	cr.Spec.SecretRef.Name = "wrongsecret"
	return cr
}

func testCredentialsRequestSecretNsDiff() *cco.CredentialsRequest {
	cr := testCompleteCredentialsRequest()
	cr.Spec.SecretRef.Namespace = "wrongns"
	return cr
}

func testCredentialsRequestSADiff() *cco.CredentialsRequest {
	cr := testCompleteCredentialsRequest()
	cr.Spec.ServiceAccountNames = []string{"wrongsa"}
	return cr
}

func testAWSProviderSpec() *runtime.RawExtension {
	codec, _ := cco.NewCodec()
	providerSpec, _ := codec.EncodeProviderSpec(&cco.AWSProviderSpec{})
	return providerSpec
}
