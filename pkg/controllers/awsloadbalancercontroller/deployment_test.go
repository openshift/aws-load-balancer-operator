package awsloadbalancercontroller

import (
	"context"
	"fmt"
	"sort"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/pointer"

	"github.com/google/go-cmp/cmp"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
	"github.com/openshift/aws-load-balancer-operator/pkg/controllers/utils/test"
)

const (
	testAWSRegion = "us-east-1"
)

func TestDesiredArgs(t *testing.T) {
	for _, tc := range []struct {
		name         string
		controller   *albo.AWSLoadBalancerController
		expectedArgs sets.String
	}{
		{
			name: "non-default ingress class",
			controller: &albo.AWSLoadBalancerController{
				Spec: albo.AWSLoadBalancerControllerSpec{
					IngressClass: "special-ingress-class",
				},
			},
			expectedArgs: sets.NewString(
				"--enable-shield=false",
				"--enable-waf=false",
				"--enable-wafv2=false",
				"--ingress-class=special-ingress-class",
			),
		},
		{
			name: "multiple replicas",
			controller: &albo.AWSLoadBalancerController{
				Spec: albo.AWSLoadBalancerControllerSpec{
					Config: &albo.AWSLoadBalancerDeploymentConfig{Replicas: 2},
				},
			},
			expectedArgs: sets.NewString(
				"--enable-shield=false",
				"--enable-waf=false",
				"--enable-wafv2=false",
				"--ingress-class=alb",
				"--enable-leader-election",
			),
		},
		{
			name: "wafv1 addon enabled",
			controller: &albo.AWSLoadBalancerController{
				Spec: albo.AWSLoadBalancerControllerSpec{
					EnabledAddons: []albo.AWSAddon{
						albo.AWSAddonWAFv1,
					},
				},
			},
			expectedArgs: sets.NewString(
				"--enable-shield=false",
				"--enable-waf=true",
				"--enable-wafv2=false",
				"--ingress-class=alb",
			),
		},
		{
			name: "wafv2 addon enabled",
			controller: &albo.AWSLoadBalancerController{
				Spec: albo.AWSLoadBalancerControllerSpec{
					EnabledAddons: []albo.AWSAddon{
						albo.AWSAddonWAFv2,
					},
				},
			},
			expectedArgs: sets.NewString(
				"--enable-shield=false",
				"--enable-waf=false",
				"--enable-wafv2=true",
				"--ingress-class=alb",
			),
		},
		{
			name: "shield addon enabled",
			controller: &albo.AWSLoadBalancerController{
				Spec: albo.AWSLoadBalancerControllerSpec{
					EnabledAddons: []albo.AWSAddon{
						albo.AWSAddonShield,
					},
				},
			},
			expectedArgs: sets.NewString(
				"--enable-shield=true",
				"--enable-waf=false",
				"--enable-wafv2=false",
				"--ingress-class=alb",
			),
		},
		{
			name: "resource tags specified",
			controller: &albo.AWSLoadBalancerController{
				Spec: albo.AWSLoadBalancerControllerSpec{
					AdditionalResourceTags: map[string]string{
						"test-key1": "test-value1",
						"test-key2": "test-value2",
						"test-key3": "test-value3",
					},
				},
			},
			expectedArgs: sets.NewString(
				"--enable-shield=false",
				"--enable-waf=false",
				"--enable-wafv2=false",
				"--ingress-class=alb",
				"--default-tags=test-key1=test-value1,test-key2=test-value2,test-key3=test-value3",
			),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defaultArgs := sets.NewString(
				"--aws-vpc-id=test-vpc",
				"--cluster-name=test-cluster",
				"--disable-ingress-class-annotation",
				"--disable-ingress-group-name-annotation",
				"--webhook-cert-dir=/tls",
			)
			expectedArgs := defaultArgs.Union(tc.expectedArgs)
			if tc.controller.Spec.IngressClass == "" {
				tc.controller.Spec.IngressClass = "alb"
			}
			args := desiredContainerArgs(tc.controller, "test-cluster", "test-vpc")

			expected := expectedArgs.List()
			sort.Strings(expected)
			if diff := cmp.Diff(expected, args); diff != "" {
				t.Fatalf("unexpected arguments\n%s", diff)
			}
		})
	}
}

type testDeploymentBuilder struct {
	name           string
	namespace      string
	serviceAccount string
	replicas       *int32
	version        string
	containers     []corev1.Container
	ownerReference []metav1.OwnerReference
	volumes        []corev1.Volume
	certsSecret    string
}

func testDeployment(name, namespace, serviceAccount string, certsSecret string) *testDeploymentBuilder {
	return &testDeploymentBuilder{name: name, namespace: namespace, serviceAccount: serviceAccount, certsSecret: certsSecret}
}

func (b *testDeploymentBuilder) withReplicas(replicas int32) *testDeploymentBuilder {
	b.replicas = pointer.Int32(replicas)
	return b
}

func (b *testDeploymentBuilder) withResourceVersion(version string) *testDeploymentBuilder {
	b.version = version
	return b
}

func (b *testDeploymentBuilder) withContainers(containers ...corev1.Container) *testDeploymentBuilder {
	b.containers = containers
	return b
}

func (b *testDeploymentBuilder) withOwnerReference(name string) *testDeploymentBuilder {
	b.ownerReference = []metav1.OwnerReference{
		{
			APIVersion: albo.GroupVersion.Identifier(),
			Kind:       "AWSLoadBalancerController",
			Name:       name,
		},
	}
	return b
}

func (b *testDeploymentBuilder) withVolumes(volumes ...corev1.Volume) *testDeploymentBuilder {
	b.volumes = volumes
	return b
}

func (b *testDeploymentBuilder) build() *appsv1.Deployment {
	d := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            fmt.Sprintf("%s-%s", controllerResourcePrefix, b.name),
			Namespace:       "test-namespace",
			OwnerReferences: b.ownerReference,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					appInstanceName: b.name,
					appLabelName:    appName,
				},
			},
			Replicas: b.replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						appInstanceName: b.name,
						appLabelName:    appName,
					},
				},
				Spec: corev1.PodSpec{
					Containers:         b.containers,
					ServiceAccountName: b.serviceAccount,
					Volumes:            b.volumes,
				},
			},
		},
	}
	if b.version != "" {
		d.ResourceVersion = b.version
	} else {
		d.ResourceVersion = "1"
	}
	return d
}

type testContainerBuilder struct {
	name         string
	image        string
	args         []string
	env          []corev1.EnvVar
	volumeMounts []corev1.VolumeMount
}

func testContainer(name, image string) *testContainerBuilder {
	return &testContainerBuilder{
		name:  name,
		image: image,
	}
}

func (b *testContainerBuilder) withEnvs(envs ...corev1.EnvVar) *testContainerBuilder {
	b.env = envs
	return b
}

func (b *testContainerBuilder) withArgs(args ...string) *testContainerBuilder {
	b.args = args
	return b
}

func (b *testContainerBuilder) withDefaultEnvs() *testContainerBuilder {
	b.env = append(b.env, corev1.EnvVar{Name: awsRegionEnvVarName, Value: testAWSRegion}, corev1.EnvVar{Name: awsCredentialEnvVarName, Value: awsCredentialsPath})
	return b
}

func (b *testContainerBuilder) withVolumeMounts(mounts ...corev1.VolumeMount) *testContainerBuilder {
	b.volumeMounts = mounts
	return b
}

func (b *testContainerBuilder) build() corev1.Container {
	return corev1.Container{
		Name:         b.name,
		Image:        b.image,
		Args:         b.args,
		Env:          b.env,
		VolumeMounts: b.volumeMounts,
	}
}

func TestUpdateDeployment(t *testing.T) {
	for _, tc := range []struct {
		name               string
		existingDeployment *appsv1.Deployment
		desiredDeployment  *appsv1.Deployment
		expectedDeployment *appsv1.Deployment
		expectUpdate       bool
	}{
		{
			name: "image changed",
			existingDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v1").build(),
			).build(),
			desiredDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v2").build(),
			).build(),
			expectedDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v2").build(),
			).build(),
			expectUpdate: true,
		},
		{
			name: "replicas changed from value",
			existingDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v1").build(),
			).withReplicas(1).build(),
			desiredDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v1").build(),
			).withReplicas(2).build(),
			expectedDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v1").build(),
			).withReplicas(2).build(),
			expectUpdate: true,
		},
		{
			name: "replicas changed from nil",
			existingDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v1").build(),
			).build(),
			desiredDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v1").build(),
			).withReplicas(1).build(),
			expectedDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v1").build(),
			).withReplicas(1).build(),
			expectUpdate: true,
		},
		{
			name: "container args changed",
			existingDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v1").withArgs("--arg1", "--arg2").build(),
			).build(),
			desiredDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v1").withArgs("--arg2", "--arg3").build(),
			).build(),
			expectedDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v1").withArgs("--arg2", "--arg3").build(),
			).build(),
			expectUpdate: true,
		},
		{
			name: "container environment variables changed",
			existingDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v1").withEnvs(
					corev1.EnvVar{Name: "test-1", Value: "value-1"},
				).build(),
			).build(),
			desiredDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v1").withEnvs(
					corev1.EnvVar{
						Name: "test-1",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								Key: "test-secret",
							},
						},
					},
					corev1.EnvVar{Name: "test-2", Value: "value-2"},
				).build(),
			).build(),
			expectedDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v1").withEnvs(
					corev1.EnvVar{
						Name: "test-1",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								Key: "test-secret",
							},
						},
					},
					corev1.EnvVar{Name: "test-2", Value: "value-2"},
				).build(),
			).build(),
			expectUpdate: true,
		},
		{
			name: "container injected into current deployment",
			existingDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v1").build(),
				testContainer("sidecar", "sidecar:v1").build(),
			).build(),
			desiredDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v1").build(),
			).build(),
			expectedDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v1").build(),
			).build(),
			expectUpdate: true,
		},
		{
			name: "desired container removed from deployment",
			existingDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("sidecar", "sidecar:v1").build(),
			).build(),
			desiredDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v1").build(),
				testContainer("sidecar", "sidecar:v1").build(),
			).build(),
			expectedDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v1").build(),
				testContainer("sidecar", "sidecar:v1").build(),
			).build(),
			expectUpdate: true,
		},
		{
			name: "no change in deployment",
			existingDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v1").withArgs("--arg1", "--arg2").withEnvs(
					corev1.EnvVar{Name: "test-1", Value: "test-1"},
					corev1.EnvVar{Name: "test-2", Value: "test-2"},
				).build(),
			).build(),
			desiredDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v1").withArgs("--arg1", "--arg2").withEnvs(
					corev1.EnvVar{Name: "test-1", Value: "test-1"},
					corev1.EnvVar{Name: "test-2", Value: "test-2"},
				).build(),
			).build(),
			expectedDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v1").withArgs("--arg1", "--arg2").withEnvs(
					corev1.EnvVar{Name: "test-1", Value: "test-1"},
					corev1.EnvVar{Name: "test-2", Value: "test-2"},
				).build(),
			).build(),
		},
		{
			name: "volume added",
			existingDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v1").build(),
			).build(),
			desiredDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v1").build(),
			).withVolumes(
				corev1.Volume{Name: "test-mount"},
			).build(),
			expectedDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v1").build(),
			).withVolumes(
				corev1.Volume{Name: "test-mount"},
			).build(),
			expectUpdate: true,
		},
		{
			name: "volume changed",
			existingDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v1").build(),
			).withVolumes(
				corev1.Volume{Name: "test-mount-1"},
			).build(),
			desiredDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v1").build(),
			).withVolumes(
				corev1.Volume{Name: "test-mount-2"},
			).build(),
			expectedDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v1").build(),
			).withVolumes(
				corev1.Volume{Name: "test-mount-2"},
			).build(),
			expectUpdate: true,
		},
		{
			name: "volume mount added",
			existingDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v1").withVolumeMounts(
					corev1.VolumeMount{Name: "config", MountPath: "/opt/config"},
				).build(),
			).build(),
			desiredDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v1").withVolumeMounts(
					corev1.VolumeMount{Name: "credentials", MountPath: "/opt/credentials"},
					corev1.VolumeMount{Name: "config", MountPath: "/opt/config"},
				).build(),
			).build(),
			expectedDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v1").withVolumeMounts(
					corev1.VolumeMount{Name: "credentials", MountPath: "/opt/credentials"},
					corev1.VolumeMount{Name: "config", MountPath: "/opt/config"},
				).build(),
			).build(),
			expectUpdate: true,
		},
		{
			name: "volume mount changed",
			existingDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v1").withVolumeMounts(
					corev1.VolumeMount{Name: "credentials", MountPath: "/opt/credentials"},
					corev1.VolumeMount{Name: "config", MountPath: "/opt/config"},
				).build(),
			).build(),
			desiredDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v1").withVolumeMounts(
					corev1.VolumeMount{Name: "credentials", MountPath: "/opt/credentials"},
					corev1.VolumeMount{Name: "config", MountPath: "/var/config"},
				).build(),
			).build(),
			expectedDeployment: testDeployment("operator", "test-namespace", "test-sa", "test-serving").withContainers(
				testContainer("controller", "controller:v1").withVolumeMounts(
					corev1.VolumeMount{Name: "credentials", MountPath: "/opt/credentials"},
					corev1.VolumeMount{Name: "config", MountPath: "/var/config"},
				).build(),
			).build(),
			expectUpdate: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			client := fake.NewClientBuilder().WithObjects(tc.existingDeployment).Build()
			r := &AWSLoadBalancerControllerReconciler{
				Client: client,
			}
			updated, err := r.updateDeployment(ctx, tc.existingDeployment, tc.desiredDeployment)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.expectUpdate != updated {
				t.Errorf("expected update to be %t, instead was %t", tc.expectUpdate, updated)
			}
			currentDeployment := &appsv1.Deployment{}
			err = r.Get(ctx, types.NamespacedName{Namespace: tc.expectedDeployment.Namespace, Name: tc.expectedDeployment.Name}, currentDeployment)
			if err != nil {
				t.Fatalf("failed to get existing deployment: %v", err)
			}

			if diff := cmp.Diff(currentDeployment.Spec, tc.expectedDeployment.Spec); diff != "" {
				t.Fatalf("deployment spec mismatch:\n%s", diff)
			}
		})
	}
}

func TestEnsureDeployment(t *testing.T) {
	for _, tc := range []struct {
		name               string
		existingObjects    []runtime.Object
		serviceAccount     *corev1.ServiceAccount
		controller         *albo.AWSLoadBalancerController
		expectedDeployment *appsv1.Deployment
		clusterName        string
		vpcID              string
	}{
		{
			name:           "new controller",
			serviceAccount: &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "test-sa"}},
			controller: &albo.AWSLoadBalancerController{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec:       albo.AWSLoadBalancerControllerSpec{},
			},
			expectedDeployment: testDeployment(
				"cluster",
				"test-namespace",
				"test-sa", "test-serving").withContainers(
				testContainer("controller", "test-image").withDefaultEnvs().withVolumeMounts(
					corev1.VolumeMount{Name: "aws-credentials", MountPath: "/aws"},
					corev1.VolumeMount{Name: "tls", MountPath: "/tls"},
				).build(),
			).withOwnerReference("cluster").withVolumes(
				corev1.Volume{Name: "aws-credentials", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "test-credentials"}}},
				corev1.Volume{Name: "tls", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "test-serving"}}},
			).build(),
		},
		{
			name:           "existing controller",
			serviceAccount: &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "test-sa"}},
			controller: &albo.AWSLoadBalancerController{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec:       albo.AWSLoadBalancerControllerSpec{},
			},
			existingObjects: []runtime.Object{
				testDeployment(
					"cluster",
					"test-namespace",
					"test-sa",
					"test-serving").withContainers(
					testContainer("controller", "controller:v0.1").build(),
				).build(),
			},
			expectedDeployment: testDeployment(
				"cluster",
				"test-namespace",
				"test-sa",
				"test-serving",
			).withContainers(
				testContainer("controller", "test-image").withDefaultEnvs().withVolumeMounts(
					corev1.VolumeMount{Name: "aws-credentials", MountPath: "/aws"},
					corev1.VolumeMount{Name: "tls", MountPath: "/tls"},
				).build(),
			).withResourceVersion("2").withVolumes(
				corev1.Volume{Name: "aws-credentials", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "test-credentials"}}},
				corev1.Volume{Name: "tls", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "test-serving"}}},
			).build(),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewClientBuilder().WithScheme(test.Scheme).WithRuntimeObjects(tc.existingObjects...).Build()
			r := &AWSLoadBalancerControllerReconciler{
				Client:      client,
				Scheme:      test.Scheme,
				ClusterName: "test-cluster",
				VPCID:       "test-vpc",
				AWSRegion:   testAWSRegion,
			}
			_, err := r.ensureDeployment(context.Background(), "test-namespace", "test-image", tc.serviceAccount, "test-credentials", "test-serving", tc.controller)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tc.expectedDeployment.Spec.Template.Spec.Containers[0].Args = desiredContainerArgs(tc.controller, "test-cluster", "test-vpc")
			var deployment appsv1.Deployment
			err = client.Get(context.Background(), types.NamespacedName{Namespace: "test-namespace", Name: fmt.Sprintf("%s-%s", controllerResourcePrefix, tc.controller.Name)}, &deployment)
			if err != nil {
				t.Fatalf("failed to get deployment: %v", err)
			}
			if diff := cmp.Diff(&deployment, tc.expectedDeployment); diff != "" {
				t.Fatalf("resource mismatch:\n%s", diff)
			}
		})
	}
}
