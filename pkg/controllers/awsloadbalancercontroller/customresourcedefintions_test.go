package awsloadbalancercontroller

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	apiextensionv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
)

func makeTestCRD(name, singular, plural, kind, listKind string, fields ...string) *apiextensionv1.CustomResourceDefinition {
	crd := &apiextensionv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: apiextensionv1.CustomResourceDefinitionSpec{
			Names: apiextensionv1.CustomResourceDefinitionNames{
				Plural:   plural,
				Singular: singular,
				Kind:     kind,
				ListKind: listKind,
			},
			Versions: []apiextensionv1.CustomResourceDefinitionVersion{
				{
					Name: "v1alpha1",
					Schema: &apiextensionv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionv1.JSONSchemaProps{
							Properties: map[string]apiextensionv1.JSONSchemaProps{},
						},
					},
				},
			},
		},
	}
	for _, f := range fields {
		crd.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties[f] = apiextensionv1.JSONSchemaProps{
			Type: "string",
		}
	}
	return crd
}

func makeObjects(crds []*apiextensionv1.CustomResourceDefinition) []client.Object {
	objects := make([]client.Object, len(crds))
	for i, c := range crds {
		objects[i] = c
	}
	return objects
}

func TestEnsureCustomResourceDefinitions(t *testing.T) {
	for _, tc := range []struct {
		name           string
		controllerCRDs []*apiextensionv1.CustomResourceDefinition
		currentCRDs    []*apiextensionv1.CustomResourceDefinition
		finalCRDs      []*apiextensionv1.CustomResourceDefinition
	}{
		{
			name: "no previous CRD",
			controllerCRDs: []*apiextensionv1.CustomResourceDefinition{
				makeTestCRD("test.group.io", "test", "tests", "Test", "TestList", "field1", "field2"),
			},
			finalCRDs: []*apiextensionv1.CustomResourceDefinition{
				makeTestCRD("test.group.io", "test", "tests", "Test", "TestList", "field1", "field2"),
			},
		},
		{
			name: "existing CRDs",
			currentCRDs: []*apiextensionv1.CustomResourceDefinition{
				makeTestCRD("test.group.io", "test", "tests", "Test", "TestList", "field1", "field1"),
			},
			controllerCRDs: []*apiextensionv1.CustomResourceDefinition{
				makeTestCRD("test.group.io", "test", "tests", "Test", "TestList", "field1", "field2"),
			},
			finalCRDs: []*apiextensionv1.CustomResourceDefinition{
				makeTestCRD("test.group.io", "test", "tests", "Test", "TestList", "field1", "field2"),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &AWSLoadBalancerControllerReconciler{
				Client:                    fake.NewClientBuilder().WithScheme(testScheme).WithObjects(makeObjects(tc.currentCRDs)...).Build(),
				Scheme:                    testScheme,
				CustomResourceDefinitions: tc.controllerCRDs,
			}
			err := r.ensureCustomResourceDefinitions(context.Background(), &albo.AWSLoadBalancerController{ObjectMeta: metav1.ObjectMeta{Name: allowedResourceName}})
			if err != nil {
				t.Errorf("unexpected error ensuring CRD: %v", err)
				return
			}
			var controllerCRDs apiextensionv1.CustomResourceDefinitionList
			err = r.List(context.Background(), &controllerCRDs)
			if err != nil {
				t.Errorf("unexpected error listing controller CRDs: %v", err)
				return
			}

			for _, fc := range tc.finalCRDs {
				var currentCRD apiextensionv1.CustomResourceDefinition
				err = r.Get(context.Background(), types.NamespacedName{Name: fc.Name}, &currentCRD)
				if err != nil {
					t.Errorf("failed to get CRD %s: %v", fc.Name, err)
					continue
				}

				if !equality.Semantic.DeepEqual(fc.Spec, currentCRD.Spec) {
					t.Errorf("CRD spec differs. Diff:\n%s", cmp.Diff(fc.Spec, currentCRD.Spec))
				}
			}
		})
	}
}
