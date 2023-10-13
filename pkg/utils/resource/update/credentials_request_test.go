package update

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openshift/aws-load-balancer-operator/pkg/utils/test"
)

func Test_CredentialsRequest(t *testing.T) {
	testCases := []struct {
		name        string
		current     *cco.CredentialsRequest
		currentSpec *cco.AWSProviderSpec
		desired     *cco.CredentialsRequest
		desiredSpec *cco.AWSProviderSpec
		expected    bool
		expectedErr string
	}{
		{
			name: "secret name changed",
			current: &cco.CredentialsRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
				Spec: cco.CredentialsRequestSpec{
					SecretRef:           corev1.ObjectReference{Name: "secret-2"},
					ServiceAccountNames: []string{"sa-1"},
					CloudTokenPath:      "/path/to/token",
				},
			},
			desired: &cco.CredentialsRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
				Spec: cco.CredentialsRequestSpec{
					SecretRef:           corev1.ObjectReference{Name: "secret-1"},
					ServiceAccountNames: []string{"sa-1"},
					CloudTokenPath:      "/path/to/token",
				},
			},
			expected: true,
		},
		{
			name: "secret namespace changed",
			current: &cco.CredentialsRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
				Spec: cco.CredentialsRequestSpec{
					SecretRef:           corev1.ObjectReference{Name: "secret-1", Namespace: "ns-2"},
					ServiceAccountNames: []string{"sa-1"},
					CloudTokenPath:      "/path/to/token",
				},
			},
			desired: &cco.CredentialsRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
				Spec: cco.CredentialsRequestSpec{
					SecretRef:           corev1.ObjectReference{Name: "secret-1", Namespace: "ns"},
					ServiceAccountNames: []string{"sa-1"},
					CloudTokenPath:      "/path/to/token",
				},
			},
			expected: true,
		},
		{
			name: "service account changed",
			current: &cco.CredentialsRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
				Spec: cco.CredentialsRequestSpec{
					SecretRef:           corev1.ObjectReference{Name: "secret-1"},
					ServiceAccountNames: []string{"sa-2"},
					CloudTokenPath:      "/path/to/token",
				},
			},
			desired: &cco.CredentialsRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
				Spec: cco.CredentialsRequestSpec{
					SecretRef:           corev1.ObjectReference{Name: "secret-1"},
					ServiceAccountNames: []string{"sa-1"},
					CloudTokenPath:      "/path/to/token",
				},
			},
			expected: true,
		},
		{
			name: "cloud token path changed",
			current: &cco.CredentialsRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
				Spec: cco.CredentialsRequestSpec{
					SecretRef:           corev1.ObjectReference{Name: "secret-1"},
					ServiceAccountNames: []string{"sa-1"},
					CloudTokenPath:      "/path/to/token2",
				},
			},
			desired: &cco.CredentialsRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
				Spec: cco.CredentialsRequestSpec{
					SecretRef:           corev1.ObjectReference{Name: "secret-1"},
					ServiceAccountNames: []string{"sa-1"},
					CloudTokenPath:      "/path/to/token",
				},
			},
			expected: true,
		},
		{
			name: "iam statement changed",
			current: &cco.CredentialsRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
				Spec: cco.CredentialsRequestSpec{
					SecretRef:           corev1.ObjectReference{Name: "secret-1"},
					ServiceAccountNames: []string{"sa-1"},
					CloudTokenPath:      "/path/to/token",
				},
			},
			currentSpec: &cco.AWSProviderSpec{
				StatementEntries: []cco.StatementEntry{
					{
						Effect:          "Allow",
						Resource:        "arn:aws:ec2:*:*:subnet/*",
						PolicyCondition: cco.IAMPolicyCondition{},
						Action: []string{
							"ec2:DescribeSubnets",
						},
					},
				},
				STSIAMRoleARN: "arn1",
			},
			desired: &cco.CredentialsRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
				Spec: cco.CredentialsRequestSpec{
					SecretRef:           corev1.ObjectReference{Name: "secret-1"},
					ServiceAccountNames: []string{"sa-1"},
					CloudTokenPath:      "/path/to/token",
				},
			},
			desiredSpec: &cco.AWSProviderSpec{
				StatementEntries: []cco.StatementEntry{
					{
						Effect:          "Allow",
						Resource:        "*",
						PolicyCondition: cco.IAMPolicyCondition{},
						Action: []string{
							"ec2:DescribeSubnets",
						},
					},
				},
				STSIAMRoleARN: "arn1",
			},
			expected: true,
		},
		{
			name: "iam role arn changed",
			current: &cco.CredentialsRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
				Spec: cco.CredentialsRequestSpec{
					SecretRef:           corev1.ObjectReference{Name: "secret-1"},
					ServiceAccountNames: []string{"sa-1"},
					CloudTokenPath:      "/path/to/token",
				},
			},
			currentSpec: &cco.AWSProviderSpec{
				StatementEntries: []cco.StatementEntry{
					{
						Effect:          "Allow",
						Resource:        "*",
						PolicyCondition: cco.IAMPolicyCondition{},
						Action: []string{
							"ec2:DescribeSubnets",
						},
					},
				},
				STSIAMRoleARN: "arn2",
			},
			desired: &cco.CredentialsRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
				Spec: cco.CredentialsRequestSpec{
					SecretRef:           corev1.ObjectReference{Name: "secret-1"},
					ServiceAccountNames: []string{"sa-1"},
					CloudTokenPath:      "/path/to/token",
				},
			},
			desiredSpec: &cco.AWSProviderSpec{
				StatementEntries: []cco.StatementEntry{
					{
						Effect:          "Allow",
						Resource:        "*",
						PolicyCondition: cco.IAMPolicyCondition{},
						Action: []string{
							"ec2:DescribeSubnets",
						},
					},
				},
				STSIAMRoleARN: "arn1",
			},
			expected: true,
		},
		{
			name: "no change",
			current: &cco.CredentialsRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
				Spec: cco.CredentialsRequestSpec{
					SecretRef:           corev1.ObjectReference{Name: "secret-1"},
					ServiceAccountNames: []string{"sa-1"},
					CloudTokenPath:      "/path/to/token",
				},
			},
			currentSpec: &cco.AWSProviderSpec{
				StatementEntries: []cco.StatementEntry{
					{
						Effect:          "Allow",
						Resource:        "*",
						PolicyCondition: cco.IAMPolicyCondition{},
						Action: []string{
							"ec2:DescribeSubnets",
						},
					},
				},
				STSIAMRoleARN: "arn1",
			},
			desired: &cco.CredentialsRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
				Spec: cco.CredentialsRequestSpec{
					SecretRef:           corev1.ObjectReference{Name: "secret-1"},
					ServiceAccountNames: []string{"sa-1"},
					CloudTokenPath:      "/path/to/token",
				},
			},
			desiredSpec: &cco.AWSProviderSpec{
				StatementEntries: []cco.StatementEntry{
					{
						Effect:          "Allow",
						Resource:        "*",
						PolicyCondition: cco.IAMPolicyCondition{},
						Action: []string{
							"ec2:DescribeSubnets",
						},
					},
				},
				STSIAMRoleARN: "arn1",
			},
			expected: false,
		},
		{
			name: "name input error",
			current: &cco.CredentialsRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "wrong", Namespace: "default"},
				Spec: cco.CredentialsRequestSpec{
					SecretRef:           corev1.ObjectReference{Name: "secret-1"},
					ServiceAccountNames: []string{"sa-1"},
					CloudTokenPath:      "/path/to/token",
				},
			},
			desired: &cco.CredentialsRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
				Spec: cco.CredentialsRequestSpec{
					SecretRef:           corev1.ObjectReference{Name: "secret-1"},
					ServiceAccountNames: []string{"sa-1"},
					CloudTokenPath:      "/path/to/token",
				},
			},
			expectedErr: "current and desired name mismatch",
		},
		{
			name: "namespace input error",
			current: &cco.CredentialsRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "wrong"},
				Spec: cco.CredentialsRequestSpec{
					SecretRef:           corev1.ObjectReference{Name: "secret-1"},
					ServiceAccountNames: []string{"sa-1"},
					CloudTokenPath:      "/path/to/token",
				},
			},
			desired: &cco.CredentialsRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
				Spec: cco.CredentialsRequestSpec{
					SecretRef:           corev1.ObjectReference{Name: "secret-1"},
					ServiceAccountNames: []string{"sa-1"},
					CloudTokenPath:      "/path/to/token",
				},
			},
			expectedErr: "current and desired namespace mismatch",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cli := fake.NewClientBuilder().WithScheme(test.Scheme).WithObjects(tc.current).Build()

			currentSpecEnc, _ := cco.Codec.EncodeProviderSpec(tc.currentSpec.DeepCopyObject())
			desiredSpecEnc, _ := cco.Codec.EncodeProviderSpec(tc.desiredSpec.DeepCopyObject())
			tc.current.Spec.ProviderSpec = currentSpecEnc
			tc.desired.Spec.ProviderSpec = desiredSpecEnc

			changed, err := UpdateCredentialsRequest(context.TODO(), cli, tc.current, tc.desired)
			if err != nil {
				if tc.expectedErr == "" {
					t.Fatalf("unexpected error: %q", err)
				}
				if !strings.Contains(err.Error(), tc.expectedErr) {
					t.Fatalf("expected error %q, but got %q", tc.expectedErr, err)
				}
				return
			} else {
				if tc.expectedErr != "" {
					t.Fatalf("expected error not received: %q", tc.expectedErr)
				}
			}

			if changed != tc.expected {
				t.Fatalf("expected to report a change: %v, but got: %v", tc.expected, changed)
			}

			if changed && tc.desiredSpec != nil {
				fetched := &cco.CredentialsRequest{}
				key := types.NamespacedName{Namespace: tc.desired.Namespace, Name: tc.desired.Name}
				err = cli.Get(context.Background(), key, fetched)
				if err != nil {
					t.Fatalf("failed to fetch credentials request: %v", err)
				}

				fetchedSpec := &cco.AWSProviderSpec{}
				err = cco.Codec.DecodeProviderSpec(fetched.Spec.ProviderSpec, fetchedSpec)
				if err != nil {
					t.Fatalf("failed to decode fetched credentials request's provider spec: %v", err)
				}
				opts := []cmp.Option{
					cmpopts.EquateEmpty(),
					cmpopts.IgnoreFields(cco.AWSProviderSpec{}, "Kind", "APIVersion"),
				}
				if diff := cmp.Diff(fetchedSpec, tc.desiredSpec, opts...); diff != "" {
					t.Fatalf("fetched credentials request does not match the desired state:\n%s", diff)
				}
			}
		})
	}
}
