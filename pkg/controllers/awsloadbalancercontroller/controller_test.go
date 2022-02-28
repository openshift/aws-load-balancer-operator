package awsloadbalancercontroller

import (
	apiextensionv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	configv1 "github.com/openshift/api/config/v1"
	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
)

var (
	// testScheme is as the runtime.Scheme for all tests
	testScheme *runtime.Scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(testScheme))

	utilruntime.Must(albo.AddToScheme(testScheme))
	//+kubebuilder:scaffold:scheme

	utilruntime.Must(configv1.Install(testScheme))
	utilruntime.Must(cco.Install(testScheme))
	utilruntime.Must(apiextensionv1.AddToScheme(testScheme))
}
