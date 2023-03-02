package awsloadbalancercontroller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/operator-framework/operator-lib/proxy"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
)

const (
	appLabelName    = "app.kubernetes.io/name"
	appName         = "aws-load-balancer-operator"
	appInstanceName = "app.kubernetes.io/instance"
	// trustedCAAnnotation is the annotation which contains the hash of the trusted CA configmap's contents.
	// It's added to the template pod spec of the controller deployment. Therefore a new rollout is triggered
	// if the hash (annotation value) changes. The hash is calculated at each reconciliation of the controller resource.
	// The rollout is necessary because 1) the trusted configmap is consumed as a subPath which forbids the updates,
	// 2) the controller doesn't have a means (fsnotify or similar) to detect the updates anyway.
	trustedCAAnnotation = "networking.olm.openshift.io/trusted-ca-configmap-hash"
	// awsLoadBalancerControllerContainerName is the name of the AWS load balancer controller's container.
	awsLoadBalancerControllerContainerName = "controller"
	// awsSDKLoadConfigName is the name of the environment variable which enables shared configs.
	// Without which certain fields in the config aren't set. Eg: role_arn.
	awsSDKLoadConfigName = "AWS_SDK_LOAD_CONFIG"
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
	// boundSATokenVolumeName is the volume name for the sa token
	boundSATokenVolumeName = "bound-sa-token"
	// boundSATokenDir is the sa token directory
	boundSATokenDir = "/var/run/secrets/openshift/serviceaccount"
	// trustedCAVolumeName is the name of the volume with the CA bundle to be trusted by the controller.
	trustedCAVolumeName = "trusted-ca"
	// trustedCAPath is the mounting path for the trusted CA bundle.
	// Default certificate path is taken from the golang source:
	// https://cs.opensource.google/go/go/+/refs/tags/go1.19.5:src/crypto/x509/root_linux.go;drc=82f09b75ca181a6be0e594e1917e4d3d91934b27;l=20
	trustedCAPath = "/etc/pki/tls/certs/albo-tls-ca-bundle.crt"
	// defaultCABundleKey is the default name for the data key of the configmap injected with the trusted CA.
	defaultCABundleKey = "ca-bundle.crt"
	// all capabilities in the pod security context
	allCapabilities = "ALL"
)

func (r *AWSLoadBalancerControllerReconciler) ensureDeployment(ctx context.Context, sa *corev1.ServiceAccount, crSecretName, servingSecretName string, controller *albo.AWSLoadBalancerController, trustCAConfigMap *corev1.ConfigMap) (*appsv1.Deployment, error) {
	deploymentName := fmt.Sprintf("%s-%s", controllerResourcePrefix, controller.Name)

	reqLogger := log.FromContext(ctx).WithValues("deployment", deploymentName)
	reqLogger.Info("ensuring deployment for aws-load-balancer-controller instance")

	exists, current, err := r.currentDeployment(ctx, deploymentName, r.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get existing deployment %s: %w", deploymentName, err)
	}

	trustCAConfigMapName, trustCAConfigMapHash := "", ""
	if trustCAConfigMap != nil {
		trustCAConfigMapName = trustCAConfigMap.Name
		configMapHash, err := buildMapHash(trustCAConfigMap.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to build the trusted CA configmap's hash: %w", err)
		}
		trustCAConfigMapHash = configMapHash
	}

	desired := r.desiredDeployment(deploymentName, crSecretName, servingSecretName, controller, sa, trustCAConfigMapName, trustCAConfigMapHash)
	err = controllerutil.SetControllerReference(controller, desired, r.Scheme)
	if err != nil {
		return nil, fmt.Errorf("failed to set owner reference on deployment %s: %w", deploymentName, err)
	}

	if !exists {
		err = r.createDeployment(ctx, desired)
		if err != nil {
			return nil, fmt.Errorf("failed to create deployment %s: %w", deploymentName, err)
		}
		_, current, err = r.currentDeployment(ctx, deploymentName, r.Namespace)
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
		_, current, err = r.currentDeployment(ctx, deploymentName, r.Namespace)
		if err != nil {
			return nil, fmt.Errorf("failed to get existing deployment: %w", err)
		}
	}
	return current, nil
}

func (r *AWSLoadBalancerControllerReconciler) desiredDeployment(name, credentialsRequestSecretName, servingSecret string, controller *albo.AWSLoadBalancerController, sa *corev1.ServiceAccount, trustedCAConfigMapName, trustedCAConfigMapHash string) *appsv1.Deployment {
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: r.Namespace,
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
							Name:  awsLoadBalancerControllerContainerName,
							Image: r.Image,
							Args:  desiredContainerArgs(controller, r.ClusterName, r.VPCID),
							Env: append([]corev1.EnvVar{
								{
									Name:  awsRegionEnvVarName,
									Value: r.AWSRegion,
								},
								{
									Name:  awsCredentialEnvVarName,
									Value: awsCredentialsPath,
								},
								{
									Name:  awsSDKLoadConfigName,
									Value: "1",
								},
								// OLM adds the http proxy environment variables to all the operators it manages
								// if the cluster wide egress proxy is set up on the cluster.
								// We propagate these environment variables down to the controller.
								// Ref: https://sdk.operatorframework.io/docs/building-operators/golang/references/proxy-vars/#m-docsbuilding-operatorsgolangreferencesproxy-vars
							}, proxy.ReadProxyVarsFromEnv()...),
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      awsCredentialsVolumeName,
									MountPath: awsCredentialsDir,
								},
								{
									Name:      webhookTLSVolumeName,
									MountPath: webhookTLSDir,
								},
								{
									Name:      boundSATokenVolumeName,
									MountPath: boundSATokenDir,
									ReadOnly:  true,
								},
							},
							SecurityContext: &corev1.SecurityContext{
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{allCapabilities},
								},
								Privileged:               pointer.Bool(false),
								RunAsNonRoot:             pointer.Bool(true),
								AllowPrivilegeEscalation: pointer.Bool(false),
								SeccompProfile: &corev1.SeccompProfile{
									Type: corev1.SeccompProfileTypeRuntimeDefault,
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
						{
							Name: boundSATokenVolumeName,
							VolumeSource: corev1.VolumeSource{
								Projected: &corev1.ProjectedVolumeSource{
									DefaultMode: pointer.Int32(420),
									Sources: []corev1.VolumeProjection{{
										ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
											Audience:          "openshift",
											ExpirationSeconds: pointer.Int64(3600),
											Path:              "token",
										},
									}},
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
	if trustedCAConfigMapName != "" {
		if trustedCAConfigMapHash != "" {
			if d.Spec.Template.Annotations == nil {
				d.Spec.Template.Annotations = map[string]string{}
			}
			d.Spec.Template.Annotations[trustedCAAnnotation] = trustedCAConfigMapHash
		}
		d.Spec.Template.Spec.Volumes = append(d.Spec.Template.Spec.Volumes, corev1.Volume{
			Name: trustedCAVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: trustedCAConfigMapName,
					},
				},
			}})
		for i := range d.Spec.Template.Spec.Containers {
			d.Spec.Template.Spec.Containers[i].VolumeMounts = append(d.Spec.Template.Spec.Containers[i].VolumeMounts, corev1.VolumeMount{
				Name:      trustedCAVolumeName,
				MountPath: trustedCAPath,
				SubPath:   defaultCABundleKey,
			})
		}
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
		for _, t := range controller.Spec.AdditionalResourceTags {
			tags = append(tags, fmt.Sprintf("%s=%s", t.Key, t.Value))
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
	args = append(args, "--feature-gates=EnableIPTargetType=false")
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

	// add desired template annotations if missing
	if len(desired.Spec.Template.Annotations) != 0 {
		if updated.Spec.Template.Annotations == nil {
			updated.Spec.Template.Annotations = map[string]string{}
		}
		for desKey, desVal := range desired.Spec.Template.Annotations {
			if currVal, currExists := current.Spec.Template.Annotations[desKey]; !currExists || currVal != desVal {
				updated.Spec.Template.Annotations[desKey] = desVal
				outdated = true
			}
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

			if changed := hasContainerChanged(updated.Spec.Template.Spec.Containers[foundIndex], desiredContainer); changed {
				updated.Spec.Template.Spec.Containers[foundIndex] = desiredContainer
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

func hasContainerChanged(current, desired corev1.Container) bool {
	if current.Image != desired.Image {
		return true
	}
	if !cmp.Equal(current.Args, desired.Args) {
		return true
	}

	if hasSecurityContextChanged(current.SecurityContext, desired.SecurityContext) {
		return true
	}

	if len(current.Env) != len(desired.Env) {
		return true
	}
	currentEnvs := indexedContainerEnv(current.Env)
	for _, e := range desired.Env {
		if ce, ok := currentEnvs[e.Name]; !ok {
			return true
		} else if !cmp.Equal(ce, e) {
			return true
		}
	}

	if len(current.VolumeMounts) != len(desired.VolumeMounts) {
		return true
	}

	for i := 0; i < len(current.VolumeMounts); i++ {
		cvm := current.VolumeMounts[i]
		dvm := desired.VolumeMounts[i]
		if cvm.Name != dvm.Name || cvm.MountPath != dvm.MountPath {
			return true
		}
	}

	return false
}

func hasSecurityContextChanged(current, desired *corev1.SecurityContext) bool {
	if desired == nil {
		return false
	}

	if current == nil {
		return true
	}

	if desired.Capabilities != nil {
		if current.Capabilities == nil {
			return true
		}
		cmpCapabilities := cmpopts.SortSlices(func(a, b corev1.Capability) bool { return a < b })
		if !cmp.Equal(desired.Capabilities.Add, current.Capabilities.Add, cmpCapabilities) {
			return true
		}

		if !cmp.Equal(desired.Capabilities.Drop, current.Capabilities.Drop, cmpCapabilities) {
			return true
		}
	}

	if !equalBoolPtr(current.RunAsNonRoot, desired.RunAsNonRoot) {
		return true
	}

	if !equalBoolPtr(current.Privileged, desired.Privileged) {
		return true
	}
	if !equalBoolPtr(current.AllowPrivilegeEscalation, desired.AllowPrivilegeEscalation) {
		return true
	}

	if desired.SeccompProfile != nil {
		if current.SeccompProfile == nil {
			return true
		}
		if desired.SeccompProfile.Type != "" && desired.SeccompProfile.Type != current.SeccompProfile.Type {
			return true
		}
	}
	return false
}

func indexedContainerEnv(envs []corev1.EnvVar) map[string]corev1.EnvVar {
	indexed := make(map[string]corev1.EnvVar)
	for _, e := range envs {
		indexed[e.Name] = e
	}
	return indexed
}

func equalBoolPtr(current, desired *bool) bool {
	if desired == nil {
		return true
	}

	if current == nil {
		return false
	}

	if *current != *desired {
		return false
	}
	return true
}

// buildMapHash is a utility function to get a checksum of a data map.
func buildMapHash(data map[string]string) (string, error) {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	hash := sha256.New()
	for _, k := range keys {
		_, err := hash.Write([]byte(k))
		if err != nil {
			return "", err
		}
		_, err = hash.Write([]byte(data[k]))
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
