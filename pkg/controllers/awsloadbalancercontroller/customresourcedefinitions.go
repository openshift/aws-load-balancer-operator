package awsloadbalancercontroller

import (
	"context"
	"embed"
	_ "embed"
	"fmt"
	"path/filepath"

	apiextensionv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
)

//go:embed customresourcedefinitions
var embeddedCRDStore embed.FS

func LoadDefaultCRDs() ([]*apiextensionv1.CustomResourceDefinition, error) {
	crdFiles, err := embeddedCRDStore.ReadDir("customresourcedefinitions")
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded CRDs directory: %w\n", err)
	}
	var embeddedCRDs []*apiextensionv1.CustomResourceDefinition
	for _, f := range crdFiles {
		fPath := filepath.Join("customresourcedefinitions", f.Name())
		crdBytes, err := embeddedCRDStore.ReadFile(fPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read embedded file %s: %w", fPath, err)
		}
		var crd apiextensionv1.CustomResourceDefinition
		err = yaml.Unmarshal(crdBytes, &crd)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshall embedded file %s: %w", fPath, err)
		}
		embeddedCRDs = append(embeddedCRDs, &crd)
	}
	return embeddedCRDs, nil
}

func (r *AWSLoadBalancerControllerReconciler) ensureCustomResourceDefinitions(ctx context.Context, awsloadbalancercontroller *albo.AWSLoadBalancerController) error {
	// TODO: list previously created CRDs and delete any of them which are not in the current CRD list
	for _, crd := range r.CustomResourceDefinitions {
		desired := crd

		if err := controllerutil.SetOwnerReference(awsloadbalancercontroller, desired, r.Scheme); err != nil {
			return fmt.Errorf("failed to set the controller reference for custom resource definition %s: %w", desired.Name, err)
		}

		exist, current, err := r.currentCustomResourceDefinition(ctx, desired.Name)
		if err != nil {
			return err
		}

		if !exist {
			if err := r.createCustomResourceDefinition(ctx, desired); err != nil {
				return err
			}
			continue
		}

		if err := r.updateCustomResourceDefinition(ctx, current, desired); err != nil {
			return err
		}
	}
	return nil
}

func (r *AWSLoadBalancerControllerReconciler) currentCustomResourceDefinition(ctx context.Context, name string) (bool, *apiextensionv1.CustomResourceDefinition, error) {
	var crd apiextensionv1.CustomResourceDefinition
	err := r.Get(ctx, types.NamespacedName{Name: name}, &crd)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, fmt.Errorf("failed to get CustomResourceDefinition %s: %w", name, err)
	}
	return true, &crd, nil
}

func (r *AWSLoadBalancerControllerReconciler) createCustomResourceDefinition(ctx context.Context, crd *apiextensionv1.CustomResourceDefinition) error {
	return r.Create(ctx, crd)
}

func (r *AWSLoadBalancerControllerReconciler) updateCustomResourceDefinition(ctx context.Context, current *apiextensionv1.CustomResourceDefinition, desired *apiextensionv1.CustomResourceDefinition) error {
	updatedCRD := current.DeepCopy()
	var updated bool

	if !equality.Semantic.DeepEqual(updatedCRD.Spec, desired.Spec) {
		updated = true
		updatedCRD.Spec = desired.Spec
	}

	if updated {
		err := r.Update(ctx, updatedCRD)
		if err != nil {
			return fmt.Errorf("failed to update CustomResourceDefinition %s: %w", updatedCRD.Name, err)
		}
	}

	return nil
}
