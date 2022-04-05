## [E2E Tests](/test/e2e)

Eecution of the [e2e tests](/test/e2e) for the `aws-load-balancer-operator` are configured at [openshift/release](https://github.com/openshift/release) and can be found under [aws-load-balancer-install](https://github.com/openshift/release/blob/master/ci-operator/step-registry/aws-load-balancer/install/aws-load-balancer-install-workflow.yaml) workflow. 

This workflow is a customized version of the `optional-operators-subscribe` workflow since we have a manual process of creating the needed secrets as pre-requisite for installation of the operator. 

The operator requires a secret containing aws credentials in the required format as mentioned in the [README.md](/README.md). The customized workflow contains this entire process as a [pre-install](https://github.com/openshift/release/blob/master/ci-operator/step-registry/aws-load-balancer/pre-install/aws-load-balancer-pre-install-ref.yaml) step, where the required credentials are provisioned using a `CredentialsRequest` from [hack/operator-credentials-request.yaml](/hack/operator-credentials-request.yaml).
