package awsloadbalancercontroller

import (
	"context"
	"fmt"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	"github.com/google/go-cmp/cmp"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
)

const (
	appLabelName    = "app.kubernetes.io/name"
	appName         = "aws-load-balancer-operator"
	appInstanceName = "app.kubernetes.io/instance"
)

func (r *AWSLoadBalancerControllerReconciler) ensureDeployment(ctx context.Context, namespace string, image string, sa *corev1.ServiceAccount, controller *albo.AWSLoadBalancerController) (*appsv1.Deployment, error) {
	deploymentName := fmt.Sprintf("%s-%s", controllerResourcePrefix, controller.Name)

	exists, current, err := r.currentDeployment(ctx, deploymentName, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get existing deployment %s: %w", deploymentName, err)
	}

	desired := desiredDeployment(deploymentName, namespace, image, r.VPCID, r.ClusterName, controller, sa)
	err = controllerutil.SetOwnerReference(controller, desired, r.Scheme)
	if err != nil {
		return nil, fmt.Errorf("failed to set owner reference on deployment %s: %w", deploymentName, err)
	}

	if !exists {
		err = r.createDeployment(ctx, desired)
		if err != nil {
			return nil, fmt.Errorf("failed to create deployment %s: %w", deploymentName, err)
		}
		_, current, err = r.currentDeployment(ctx, deploymentName, namespace)
		if err != nil {
			return nil, fmt.Errorf("failed to get new deployment %s: %w", deploymentName, err)
		}
		return current, nil
	}
	updated, err := r.updateDeployment(ctx, current, desired)
	if err != nil {
		return nil, fmt.Errorf("failed to update existing deployment: %w", err)
	}
	if updated {
		_, current, err = r.currentDeployment(ctx, deploymentName, namespace)
		if err != nil {
			return nil, fmt.Errorf("failed to get existing deployment: %w", err)
		}
	}
	return current, nil
}

func desiredDeployment(name, namespace, image string, vpcID, clusterName string, controller *albo.AWSLoadBalancerController, sa *corev1.ServiceAccount) *appsv1.Deployment {
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					appLabelName:    appName,
					appInstanceName: controller.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						appLabelName:    appName,
						appInstanceName: controller.Name,
					},
				},
				Spec: corev1.PodSpec{

					Containers: []corev1.Container{
						{
							Name:  "controller",
							Image: image,
							Args:  desiredContainerArgs(controller, clusterName, vpcID),
						},
					},
					ServiceAccountName: sa.Name,
				},
			},
		},
	}
	if controller.Spec.Config != nil && controller.Spec.Config.Replicas != 0 {
		d.Spec.Replicas = pointer.Int32(controller.Spec.Config.Replicas)
	}
	return d
}

func desiredContainerArgs(controller *albo.AWSLoadBalancerController, clusterName, vpcID string) []string {
	var args []string
	args = append(args, fmt.Sprintf("--aws-vpc-id=%s", vpcID))
	args = append(args, fmt.Sprintf("--cluster-name=%s", clusterName))

	// if additional keys are present then sort them and append it to the arguments
	if controller.Spec.AdditionalResourceTags != nil {
		var tags []string
		for k, v := range controller.Spec.AdditionalResourceTags {
			tags = append(tags, fmt.Sprintf("%s=%s", k, v))
		}
		sort.Strings(tags)
		args = append(args, fmt.Sprintf(`--default-tags=%s`, strings.Join(tags, ",")))
	}
	args = append(args, "--disable-ingress-class-annotation")
	args = append(args, "--disable-ingress-group-name-annotation")
	if controller.Spec.Config != nil && controller.Spec.Config.Replicas > 1 {
		args = append(args, "--enable-leader-election")
	}
	enabledAddons := make(map[albo.AWSAddon]struct{})
	for _, a := range controller.Spec.EnabledAddons {
		enabledAddons[a] = struct{}{}
	}
	if _, ok := enabledAddons[albo.AWSAddonShield]; ok {
		args = append(args, "--enable-shield=true")
	} else {
		args = append(args, "--enable-shield=false")
	}
	if _, ok := enabledAddons[albo.AWSAddonWAFv1]; ok {
		args = append(args, "--enable-waf=true")
	} else {
		args = append(args, "--enable-waf=false")
	}
	if _, ok := enabledAddons[albo.AWSAddonWAFv2]; ok {
		args = append(args, "--enable-wafv2=true")
	} else {
		args = append(args, "--enable-wafv2=false")
	}
	args = append(args, fmt.Sprintf("--ingress-class=%s", controller.Spec.IngressClass))
	sort.Strings(args)
	return args
}

func (r *AWSLoadBalancerControllerReconciler) currentDeployment(ctx context.Context, name string, namespace string) (bool, *appsv1.Deployment, error) {
	var deployment appsv1.Deployment
	err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &deployment)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}
	return true, &deployment, nil
}

func (r *AWSLoadBalancerControllerReconciler) createDeployment(ctx context.Context, deployment *appsv1.Deployment) error {
	return r.Create(ctx, deployment)
}

// updateDeployment updates the deployment if required and indicates if an update actually occurred
func (r *AWSLoadBalancerControllerReconciler) updateDeployment(ctx context.Context, current, desired *appsv1.Deployment) (bool, error) {
	updated := current.DeepCopy()

	var outdated bool

	// if replicas don't match then update
	if desired.Spec.Replicas != nil {
		if updated.Spec.Replicas == nil {
			updated.Spec.Replicas = pointer.Int32(*desired.Spec.Replicas)
			outdated = true
		}
		if *desired.Spec.Replicas != *updated.Spec.Replicas {
			updated.Spec.Replicas = desired.Spec.Replicas
			outdated = true
		}
	}

	// if the desired and current deployment container are not the same then just update
	if len(desired.Spec.Template.Spec.Containers) != len(updated.Spec.Template.Spec.Containers) {
		updated.Spec.Template.Spec.Containers = desired.Spec.Template.Spec.Containers
		outdated = true
	} else {
		// for each of the container in the desired deployment ensure the corresponding container in the current matches
		for _, desiredContainer := range desired.Spec.Template.Spec.Containers {
			foundIndex := -1
			for i, currentContainer := range updated.Spec.Template.Spec.Containers {
				if currentContainer.Name == desiredContainer.Name {
					foundIndex = i
					break
				}

			}
			if foundIndex < 0 {
				return false, fmt.Errorf("deployment %s does not have a container with the name %s", current.Name, desiredContainer.Name)
			}

			if container, changed := hasContainerChanged(updated.Spec.Template.Spec.Containers[foundIndex], desiredContainer); changed {
				updated.Spec.Template.Spec.Containers[foundIndex] = container
				outdated = true
			}
		}
	}

	if outdated {
		err := r.Update(ctx, updated)
		if err != nil {
			return false, fmt.Errorf("failed to update existing deployment %s: %w", updated.Name, err)
		}
		return true, nil
	}
	return false, nil
}

func hasContainerChanged(current, desired corev1.Container) (corev1.Container, bool) {
	var updated bool
	if current.Image != desired.Image {
		updated = true
	}
	if !cmp.Equal(current.Args, desired.Args) {
		updated = true
	}
	return desired, updated
}
