# [E2E Tests](/test/e2e)

Execution of the [e2e tests](/test/e2e) for the `aws-load-balancer-operator` are
configured at [openshift/release](https://github.com/openshift/release) and can
be found under [aws-load-balancer-install](<https://github.com/openshift/release/blob/master/ci-operator/step-registry/aws-load-balancer/installaws-load-balancer-install-workflow.yaml>)
workflow as shown below.

This workflow is a customized version of the `optional-operators-subscribe`
step, since we have a manual process of creating the needed secrets as
a prerequisite for installation of the operator.

The operator requires a secret containing AWS credentials in the required format
as mentioned in the [README.md](/README.md). The customized workflow contains
this entire process as a [pre-install](https://github.com/openshift/release/blob/master/ci-operator/step-registry/aws-load-balancer/pre-install/aws-load-balancer-pre-install-ref.yaml)
step, where the required credentials are provisioned using a `CredentialsRequest`
from [hack/operator-credentials-request.yaml](/hack/operator-credentials-request.yaml).

```yaml
workflow:
  as: aws-load-balancer-install
  steps:
    pre:
    - chain: ipi-aws-pre
    - ref: aws-load-balancer-pre-install
    - ref: optional-operators-subscribe
    post:
    - chain: ipi-aws-post
  documentation: |-
    Installs a cluster with a default configuration on AWS and runs through the pre-requistes of 
    the aws-load-balancer-operator to complete installation.
```

We are adding a custom step `aws-load-balancer-pre-install` to the existing
steps present in the `optional-operators-subscribe` step. This step executes
the [aws-load-balancer-pre-install-commands.sh](<https://github.com/openshift/release/blob/master/ci-operator/step-registry/aws-load-balancer/pre-install/aws-load-balancer-pre-install-commands.sh>)
shell script which creates the required secrets using the `CredentialsRequest`.
