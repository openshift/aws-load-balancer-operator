/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package test

import (
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	configv1 "github.com/openshift/api/config/v1"
	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	albo "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
)

const (
	OperandImage      = "quay.io/test/aws-load-balancer:latest"
	OperatorNamespace = "aws-load-balancer-operator"
)

var (
	TrueVar = true
	Scheme  = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(Scheme))

	utilruntime.Must(albo.AddToScheme(Scheme))
	//+kubebuilder:scaffold:scheme

	utilruntime.Must(configv1.Install(Scheme))
	utilruntime.Must(cco.Install(Scheme))
	utilruntime.Must(rbacv1.AddToScheme(Scheme))
}
