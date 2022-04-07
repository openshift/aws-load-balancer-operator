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
	"sigs.k8s.io/controller-runtime/pkg/log"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
)

const (
	appLabelName    = "app.kubernetes.io/name"
	appName         = "aws-load-balancer-operator"
	appInstanceName = "app.kubernetes.io/instance"
	// awsRegionEnvVarName is the name of the environment variable which hold the AWS region for the controller
	awsRegionEnvVarName = "AWS_DEFAULT_REGION"
	// awsCredentialsEnvVarName is the name of the environment varible whose value points to the AWS credentials file
	awsCredentialEnvVarName = "AWS_SHARED_CREDENTIALS_FILE"
	// awsCredentialsDir is the directory with the credentials profile file
	awsCredentialsDir = "/aws"
	// awsCredentialsPath is the default AWS credentials path
	awsCredentialsPath = "/aws/credentials"
	// awsCredentialsVolumeName is the name of the volume with AWS credentials from the CredentialsRequest
	awsCredentialsVolumeName = "aws-credentials"
	// the directory for the webhook certificate and key.
	webhookTLSDir = "/tls"
	// the name of the volume mount with the webhook tls config
	webhookTLSVolumeName = "tls"
)

func (r *AWSLoadBalancerControllerReconciler) ensureDeployment(ctx context.Context, namespace, image string, sa *corev1.ServiceAccount, crSecretName, servingSecretName string, controller *albo.AWSLoadBalancerController) (*appsv1.Deployment, error) {
	deploymentName := fmt.Sprintf("%s-%s", controllerResourcePrefix, controller.Name)

	reqLogger := log.FromContext(ctx).WithValues("deployment", deploymentName)
	reqLogger.Info("ensuring deployment for aws-load-balancer-controller instance")

	exists, current, err := r.currentDeployment(ctx, deploymentName, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get existing deployment %s: %w", deploymentName, err)
	}

	desired := desiredDeployment(deploymentName, namespace, image, r.VPCID, r.ClusterName, r.AWSRegion, crSecretName, servingSecretName, controller, sa)
	err = controllerutil.SetControllerReference(controller, desired, r.Scheme)
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

func desiredDeployment(name, namespace, image, vpcID, clusterName, awsRegion, credentialsRequestSecretName, servingSecret string, controller *albo.AWSLoadBalancerController, sa *corev1.ServiceAccount) *appsv1.Deployment {
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
							Env: []corev1.EnvVar{
								{
									Name:  awsRegionEnvVarName,
									Value: awsRegion,
								},
								{
									Name:  awsCredentialEnvVarName,
									Value: awsCredentialsPath,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      awsCredentialsVolumeName,
									MountPath: awsCredentialsDir,
								},
								{
									Name:      webhookTLSVolumeName,
									MountPath: webhookTLSDir,
								},
							},
						},
					},
					ServiceAccountName: sa.Name,
					Volumes: []corev1.Volume{
						{
							Name: awsCredentialsVolumeName,
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: credentialsRequestSecretName,
								},
							},
						},
						{
							Name: webhookTLSVolumeName,
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: servingSecret,
								},
							},
						},
					},
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
	args = append(args, fmt.Sprintf("--webhook-cert-dir=%s", webhookTLSDir))
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

	if haveVolumesChanged(updated.Spec.Template.Spec.Volumes, desired.Spec.Template.Spec.Volumes) {
		updated.Spec.Template.Spec.Volumes = desired.Spec.Template.Spec.Volumes
		outdated = true
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

// haveVolumesChanged if the current volumes differs from the desired volumes
func haveVolumesChanged(current []corev1.Volume, desired []corev1.Volume) bool {
	if len(current) != len(desired) {
		return true
	}
	for i := 0; i < len(current); i++ {
		cv := current[i]
		dv := desired[i]
		if cv.Name != dv.Name {
			return true
		}
		if dv.Secret != nil {
			if cv.Secret == nil {
				return true
			}
			if cv.Secret.SecretName != dv.Secret.SecretName {
				return true
			}
		}
	}
	return false
}

func hasContainerChanged(current, desired corev1.Container) (corev1.Container, bool) {
	var updated bool
	if current.Image != desired.Image {
		updated = true
	}
	if !cmp.Equal(current.Args, desired.Args) {
		updated = true
	}

	if len(current.Env) == len(desired.Env) {
		currentEnvs := indexedContainerEnv(current.Env)
		for _, e := range desired.Env {
			if ce, ok := currentEnvs[e.Name]; !ok {
				updated = true
				break
			} else if !cmp.Equal(ce, e) {
				updated = true
				break
			}
		}
	} else {
		updated = true
	}

	if len(current.VolumeMounts) != len(desired.VolumeMounts) {
		updated = true
	} else {
		for i := 0; i < len(current.VolumeMounts); i++ {
			cvm := current.VolumeMounts[i]
			dvm := desired.VolumeMounts[i]
			if cvm.Name != dvm.Name || cvm.MountPath != dvm.MountPath {
				updated = true
				break
			}
		}
	}

	return desired, updated
}

func indexedContainerEnv(envs []corev1.EnvVar) map[string]corev1.EnvVar {
	indexed := make(map[string]corev1.EnvVar)
	for _, e := range envs {
		indexed[e.Name] = e
	}
	return indexed
}
