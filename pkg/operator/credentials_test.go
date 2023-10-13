package operator

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openshift/aws-load-balancer-operator/pkg/utils/test"
)

func Test_ProvisionCredentials(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		scheme  *runtime.Scheme
		// provisionedSecret simulates CCO job
		// which is supposed to create a secret from CredentialsRequest
		provisionedSecret   *corev1.Secret
		existingCredReq     *cco.CredentialsRequest
		expectedCredReqName types.NamespacedName
		// compareCredReq compares only the moving parts of
		// the created CredentialsRequest. The moving parts
		// are those which can change depending on the input.
		// That is supposed to spare us from the unnecessary repetitions
		// of the static parts of the CredentialsRequest.
		compareCredReq   func(*cco.CredentialsRequest, *cco.AWSProviderSpec) error
		expectedContents string
		errExpected      bool
	}{
		{
			name: "nominal sts",
			envVars: map[string]string{
				"ROLEARN": "arn:aws:iam::123456789012:role/foo",
			},
			scheme: test.Scheme,
			provisionedSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws-load-balancer-operator",
					Namespace: "aws-load-balancer-operator",
				},
				Data: map[string][]byte{
					"credentials": []byte("oksts"),
				},
			},
			expectedCredReqName: types.NamespacedName{
				Namespace: "openshift-cloud-credential-operator",
				Name:      "aws-load-balancer-operator",
			},
			compareCredReq: func(credReq *cco.CredentialsRequest, providerSpec *cco.AWSProviderSpec) error {
				if providerSpec.STSIAMRoleARN != "arn:aws:iam::123456789012:role/foo" {
					return fmt.Errorf("got unexpected role arn: %q", providerSpec.STSIAMRoleARN)
				}
				if credReq.Spec.CloudTokenPath != "/var/run/secrets/openshift/serviceaccount/token" {
					return fmt.Errorf("got unexpected token path: %q", credReq.Spec.CloudTokenPath)
				}
				return nil
			},
			expectedContents: "oksts",
		},
		{
			name:   "nominal non sts",
			scheme: test.Scheme,
			provisionedSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws-load-balancer-operator",
					Namespace: "aws-load-balancer-operator",
				},
				Data: map[string][]byte{
					"credentials": []byte("oknonsts"),
				},
			},
			expectedCredReqName: types.NamespacedName{
				Namespace: "openshift-cloud-credential-operator",
				Name:      "aws-load-balancer-operator",
			},
			compareCredReq: func(credReq *cco.CredentialsRequest, providerSpec *cco.AWSProviderSpec) error {
				if providerSpec.STSIAMRoleARN != "" {
					return fmt.Errorf("expected role arn to be unset but got %q", providerSpec.STSIAMRoleARN)
				}
				if credReq.Spec.CloudTokenPath != "" {
					return fmt.Errorf("expected token path to be unset but got %q", credReq.Spec.CloudTokenPath)
				}
				return nil
			},
			expectedContents: "oknonsts",
		},
		{
			name:   "invalid role arn",
			scheme: test.Scheme,
			envVars: map[string]string{
				"ROLEARN": "arn:aws:iam:role/foo",
			},
			provisionedSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws-load-balancer-operator",
					Namespace: "aws-load-balancer-operator",
				},
				Data: map[string][]byte{
					"credentials": []byte("ok"),
				},
			},
			errExpected: true,
		},
		{
			name:   "udpate",
			scheme: test.Scheme,
			provisionedSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws-load-balancer-operator",
					Namespace: "aws-load-balancer-operator",
				},
				Data: map[string][]byte{
					"credentials": []byte("ok"),
				},
			},
			existingCredReq: &cco.CredentialsRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
				Spec: cco.CredentialsRequestSpec{
					SecretRef:           corev1.ObjectReference{Name: "secret-1"},
					ServiceAccountNames: []string{"sa-1"},
				},
			},
			expectedCredReqName: types.NamespacedName{
				Namespace: "openshift-cloud-credential-operator",
				Name:      "aws-load-balancer-operator",
			},
			compareCredReq: func(credReq *cco.CredentialsRequest, providerSpec *cco.AWSProviderSpec) error {
				if credReq.Spec.SecretRef.Namespace != "aws-load-balancer-operator" || credReq.Spec.SecretRef.Name != "aws-load-balancer-operator" {
					return fmt.Errorf("expected secret reference to be reset back to desired but got %v", credReq.Spec.SecretRef)
				}
				if len(credReq.Spec.ServiceAccountNames) != 1 || credReq.Spec.ServiceAccountNames[0] != "aws-load-balancer-operator-controller-manager" {
					return fmt.Errorf("expected service account to be reset back to desired but got %v", credReq.Spec.ServiceAccountNames)
				}
				return nil
			},
			expectedContents: "ok",
		},
		{
			name:   "credentialsrequest creation failed",
			scheme: test.BasicScheme,
			provisionedSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws-load-balancer-operator",
					Namespace: "aws-load-balancer-operator",
				},
				Data: map[string][]byte{
					"credentials": []byte("oknonsts"),
				},
			},
			errExpected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.envVars {
				err := os.Setenv(k, v)
				if err != nil {
					t.Fatalf("failed to set %q environment variable: %v", k, err)
				}
				defer os.Unsetenv(k)
			}

			bld := fake.NewClientBuilder().WithScheme(tc.scheme)
			if tc.provisionedSecret != nil {
				bld.WithObjects(tc.provisionedSecret)
			}
			if tc.existingCredReq != nil {
				bld.WithObjects(tc.existingCredReq)
			}
			cli := bld.Build()

			gotFilename, err := ProvisionCredentials(context.Background(), cli, tc.provisionedSecret.Namespace)
			if err != nil {
				if !tc.errExpected {
					t.Fatalf("got unexpected error: %v", err)
				}
				return
			} else if tc.errExpected {
				t.Fatalf("error expected but not received")
			}

			// skip CredentialsRequest check if the scheme doesn't have a CRD for it.
			if tc.scheme != test.BasicScheme {
				credReq := &cco.CredentialsRequest{}
				err = cli.Get(context.Background(), tc.expectedCredReqName, credReq)
				if err != nil {
					t.Fatalf("failed to get credentials request %v: %v", tc.expectedCredReqName, err)
				}

				providerSpec := &cco.AWSProviderSpec{}
				err = cco.Codec.DecodeProviderSpec(credReq.Spec.ProviderSpec, providerSpec)
				if err != nil {
					t.Fatalf("failed to decode credentials request's aws provider spec: %v", err)
				}

				if err := tc.compareCredReq(credReq, providerSpec); err != nil {
					t.Fatalf("credentials request comparison failed: %v", err)
				}
			}

			gotContents, err := os.ReadFile(gotFilename)
			if err != nil {
				t.Fatalf("failed to read generated file: %v", err)
			}
			if string(gotContents) != tc.expectedContents {
				t.Fatalf("expected contents %q but got %q", tc.expectedContents, string(gotContents))
			}
		})
	}
}

func Test_WaitForSecret(t *testing.T) {
	secretNsName := types.NamespacedName{
		Namespace: "test",
		Name:      "test",
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: secretNsName.Namespace,
			Name:      secretNsName.Name,
		},
	}

	// secret exists
	cli := fake.NewClientBuilder().WithScheme(test.Scheme).WithObjects(secret).Build()
	resCh, errCh := make(chan *corev1.Secret), make(chan error)
	go func(resCh chan *corev1.Secret, errCh chan error) {
		s, err := waitForSecret(context.Background(), cli, secretNsName, 10*time.Second, 10*time.Millisecond)
		resCh <- s
		errCh <- err
	}(resCh, errCh)
wait:
	for {
		select {
		case gotSecret := <-resCh:
			sameNs := gotSecret.Namespace == secret.Namespace
			sameName := gotSecret.Name == secret.Name
			if !sameNs || !sameName {
				t.Fatalf("got unexpected secret: %v", gotSecret)
			}
			break wait
		case err := <-errCh:
			t.Fatalf("got unexpected error: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out")
		}
	}

	// secret doesn't exist - timeout
	emptyCli := fake.NewClientBuilder().WithScheme(test.Scheme).WithObjects().Build()
	go func(resCh chan *corev1.Secret, errCh chan error) {
		s, err := waitForSecret(context.Background(), emptyCli, secretNsName, 1*time.Second, 10*time.Millisecond)
		resCh <- s
		errCh <- err
	}(resCh, errCh)
waitTimeout:
	for {
		select {
		case gotSecret := <-resCh:
			t.Fatalf("got unexpected secret: %v", gotSecret)
		case <-errCh:
			break waitTimeout
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out")
		}
	}
}

func Test_CredentialsFileFromSecret(t *testing.T) {
	tests := []struct {
		name             string
		secret           *corev1.Secret
		pattern          string
		expectedPrefix   string
		expectedContents string
		errExpected      bool
	}{
		{
			name: "nominal",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
				},
				Data: map[string][]byte{
					"credentials": []byte("ok"),
				},
			},
			pattern:          "test-",
			expectedPrefix:   "/tmp/test-",
			expectedContents: "ok",
		},
		{
			name: "wrong data key",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
				},
				Data: map[string][]byte{
					"wrongkey": []byte("ok"),
				},
			},
			pattern:     "test-",
			errExpected: true,
		},
		{
			name: "invalid pattern",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
				},
				Data: map[string][]byte{
					"credentials": []byte("ok"),
				},
			},
			pattern:     "test-//",
			errExpected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := credentialsFileFromSecret(tc.secret, tc.pattern)
			if err != nil {
				if !tc.errExpected {
					t.Fatalf("got unexpected error: %v", err)
				}
				return
			} else if tc.errExpected {
				t.Fatalf("error expected but not received")
			}
			if !strings.HasPrefix(got, tc.expectedPrefix) {
				t.Fatalf("expected %q to have %q prefix", got, tc.expectedPrefix)
			}
			gotContents, err := os.ReadFile(got)
			if err != nil {
				t.Fatalf("error while reading generated file: %v", err)
			}
			if string(gotContents) != tc.expectedContents {
				t.Fatalf("expected contents %q but got %q", tc.expectedContents, string(gotContents))
			}
		})
	}
}
