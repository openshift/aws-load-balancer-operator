package controllers

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1"
)

var _ = Describe("AWS Load Balancer Reconciler Watch Predicates", func() {
	Context("AWS Load Balancer Controller", func() {
		It("does not match the unique name", func() {
			albc := &albo.AWSLoadBalancerController{
				ObjectMeta: metav1.ObjectMeta{
					Name: "wrongname",
				},
			}
			var gotReq ctrl.Request
			errCh := waitForRequest(reconcileCollector.Requests, 1*time.Second, &gotReq)
			Expect(k8sClient.Create(context.Background(), albc)).Should(Succeed())
			Expect(<-errCh).NotTo(BeNil())
		})
		It("matches the unique name", func() {
			albc := &albo.AWSLoadBalancerController{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
			}
			expectedReq := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name: "cluster",
				},
			}
			var gotReq ctrl.Request
			errCh := waitForRequest(reconcileCollector.Requests, 2*time.Second, &gotReq)
			Expect(k8sClient.Create(context.Background(), albc)).Should(Succeed())
			Expect(<-errCh).To(BeNil())
			Expect(gotReq).To(Equal(expectedReq))
		})
	})

	Context("Trusted CA configmap", func() {
		It("does not match the namespace passed to reconcile", func() {
			wrongNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "wrongns",
				},
			}
			Expect(k8sClient.Create(context.Background(), wrongNs)).Should(Succeed())
			trustedCAConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-trusted-ca",
					Namespace: "wrongns",
				},
			}
			var gotReq ctrl.Request
			errCh := waitForRequest(reconcileCollector.Requests, 1*time.Second, &gotReq)
			Expect(k8sClient.Create(context.Background(), trustedCAConfigMap)).Should(Succeed())
			Expect(<-errCh).NotTo(BeNil())
		})
		It("does not match the name passed to reconcile", func() {
			trustedCAConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wrongname",
					Namespace: "aws-load-balancer-operator",
				},
			}
			var gotReq ctrl.Request
			errCh := waitForRequest(reconcileCollector.Requests, 1*time.Second, &gotReq)
			Expect(k8sClient.Create(context.Background(), trustedCAConfigMap)).Should(Succeed())
			Expect(<-errCh).NotTo(BeNil())
		})
		It("matches the name passed to reconcile", func() {
			trustedCAConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-trusted-ca",
					Namespace: "aws-load-balancer-operator",
				},
			}
			expectedReq := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name: "cluster",
				},
			}
			var gotReq ctrl.Request
			errCh := waitForRequest(reconcileCollector.Requests, 1*time.Second, &gotReq)
			Expect(k8sClient.Create(context.Background(), trustedCAConfigMap)).Should(Succeed())
			Expect(<-errCh).To(BeNil())
			Expect(gotReq).To(Equal(expectedReq))
		})
	})
})

func waitForRequest(requests chan ctrl.Request, timeout time.Duration, res *ctrl.Request) <-chan error {
	errCh := make(chan error)
	go func() {
		for {
			select {
			case req := <-requests:
				*res = req
				errCh <- nil
				return
			case <-time.After(timeout):
				errCh <- fmt.Errorf("timed out waiting for request")
				return
			}
		}
	}()
	return errCh
}
