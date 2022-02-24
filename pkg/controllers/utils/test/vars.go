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
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"

	albv1aplha1 "github.com/openshift/aws-load-balancer-operator/api/v1alpha1"
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
	if err := clientgoscheme.AddToScheme(Scheme); err != nil {
		panic(err)
	}
	if err := albv1aplha1.AddToScheme(Scheme); err != nil {
		panic(err)
	}
	if err := cco.AddToScheme(Scheme); err != nil {
		panic(err)
	}
}
