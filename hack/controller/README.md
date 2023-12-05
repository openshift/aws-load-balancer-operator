## Overview
This directory contains `CredentialsRequest`s for the aws-load-balancer-controller, all generated from the same [source IAM policy](../../assets/iam-policy.json). The difference lays in the size of the policies they define. 

## Limits
The Cloud Credential Operator and `ccoclt` generate two different inline policies:
- The Cloud Credential Operator generates a **user** inline policy whose size limit is **2048** characters.
- `ccoctl` generates a **role** inline policy which has the size limit of **10240** characters.

Link: [IAM and STS character limits](https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_iam-quotas.html#reference_iam-quotas-entity-length).

## controller-credentials-request.yaml

This `CrendetialsRequest` is semantically equivalent to the source IAM policy.
The Cloud Credential Operator cannot create a policy defined in this `CredentialsRequest` because it exceeds the size limit of the user inline policy.      
The recommended way to use this `CrendetialsRequest` is to submit it to `ccoctl` as described in [the post installation instructions](https://github.com/openshift/aws-load-balancer-operator/blob/b757416f27d3a84113b4660358b98cca0064731f/docs/install.md#option-1-using-ccoctl).

## controller-credentials-request-minify.yaml

This `CrendetialsRequest` is a compact ("minified") version of the source IAM policy. Its goal is to fit within the user inline policy's size limit.
This allows it to be created by both the Cloud Credential Operator and `ccoctl`.   
Currently, this `CrendetialsRequest` is used in two places:
- by the operator [to ensure `CredentialsRequest` CR](https://github.com/openshift/aws-load-balancer-operator/blob/a846cc27dc0f08adbf404714d308ded7f2cddebe/pkg/controllers/awsloadbalancercontroller/credentials_request.go#L145) during `AWSLoadBalancerController` reconciliation
- by [the aws-load-balancer pre-install CI step](https://github.com/openshift/release/blob/d797eff6740de41ee2793866f358b246e2b52ae4/ci-operator/step-registry/aws-load-balancer/pre-install/aws-load-balancer-pre-install-commands.sh#L14) to create a secret for [some e2e test cases](https://github.com/openshift/aws-load-balancer-operator/blob/a846cc27dc0f08adbf404714d308ded7f2cddebe/test/e2e/operator_test.go#L324).

**Note**: this `CredentialsRequest` has broader permissions than the source IAM policy!
