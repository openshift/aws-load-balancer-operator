# Installation

This document provides the necessary installation and post-installation steps to ensure the operator can function correctly.

- [Non-STS Clusters](#non-sts-clusters)
    - [Operator installation](#operator-installation)
- [STS Clusters](#sts-clusters)
    - [Operator installation on STS cluster](#operator-installation-on-sts-cluster)
    - [Post operator installation on STS cluster](#post-operator-installation-on-sts-cluster)
        - [Option 1. Using ccoctl](#option-1-using-ccoctl)
        - [Option 2. Using the AWS CLI](#option-2-using-the-aws-cli)

## Non-STS clusters

### Operator installation

The operator can be installed either through [the OperatorHub web UI](https://docs.openshift.com/container-platform/latest/operators/understanding/olm-understanding-operatorhub.html) or using the OpenShift CLI:

```bash
$ oc new-project aws-load-balancer-operator

$ cat <<EOF | oc apply -f -
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: aws-load-balancer-operator
  namespace: aws-load-balancer-operator
spec:
  targetNamespaces: []
EOF

$ cat <<EOF | oc apply -f -
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: aws-load-balancer-operator
  namespace: aws-load-balancer-operator
spec:
  channel: stable-v1
  name: aws-load-balancer-operator
  source: redhat-operators
  sourceNamespace: openshift-marketplace
EOF
```

## STS clusters

The **aws-load-balancer-operator** relies on the **cloud-credential-operator** to provision the secrets for itself and for the operated controller. For this `CredentialsRequest` instances are created by **aws-load-balancer-operator**.

### Operator installation on STS cluster

In an STS cluster, the operator's `CredentialsRequest` needs to be set with the IAM role which needs to be [provisioned manually](prerequisites.md#iam-role-for-sts-clusters). The role's ARN needs to be passed to the operator as an environment variable.    
This can be achieved either through the dedicated input box in [the OperatorHub web UI](https://docs.openshift.com/container-platform/latest/operators/understanding/olm-understanding-operatorhub.html) or by specifying it in the `Subscription` resource when installing the operator via the OpenShift CLI:

```bash
$ oc new-project aws-load-balancer-operator

$ cat <<EOF | oc apply -f -
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: aws-load-balancer-operator
  namespace: aws-load-balancer-operator
spec:
  targetNamespaces: []
EOF

$ cat <<EOF | oc apply -f -
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: aws-load-balancer-operator
  namespace: aws-load-balancer-operator
spec:
  channel: stable-v1
  name: aws-load-balancer-operator
  source: redhat-operators
  sourceNamespace: openshift-marketplace
  config:
    env:
    - name: ROLEARN
      value: "${ROLEARN}"
EOF
```

The **aws-load-balancer-operator** will wait until the required secret is created before moving to the available state.

### Post operator installation on STS cluster
In an STS cluster, the controller's `CredentialsRequest` needs to be set with the IAM role which needs to be provisioned manually.

There are two options for creating the controller's IAM role:
1. Using [`ccoctl`](https://docs.openshift.com/container-platform/latest/authentication/managing_cloud_provider_credentials/cco-mode-sts.html#cco-ccoctl-configuring_cco-mode-sts) and a pre-defined `CredentialsRequest`.
2. Using AWS CLI and pre-defined AWS manifests.

If your system doesn't support `ccoctl`, the second option is the only available choice.

#### Option 1. Using `ccoctl`
The controller's `CredentialsRequest` is maintained in [hack/controller/controller-credentials-request.yaml](../hack/controller/controller-credentials-request.yaml) file of this repository.
Its contents are identical to the ones requested by **aws-load-balancer-operator** from the **cloud-credential-operator**.

1. [Extract and prepare the `ccoctl` binary](https://docs.openshift.com/container-platform/4.13/authentication/managing_cloud_provider_credentials/cco-mode-sts.html#cco-ccoctl-configuring_cco-mode-sts)

2. Use the `ccoctl` tool to create a IAM role from the pre-defined controller's `CredentialsRequest`:

    ```bash
   $ curl --create-dirs -o <credrequests-dir>/controller.yaml https://raw.githubusercontent.com/openshift/aws-load-balancer-operator/main/hack/controller/controller-credentials-request.yaml
   $ CCOCTL_OUTPUT=$(mktemp)
   $ ROLENAME=<name>
   $ ccoctl aws create-iam-roles --name ${ROLENAME:0:12} --region=<aws_region> --credentials-requests-dir=<credrequests-dir> --identity-provider-arn <oidc-arn> 2>&1 | tee "${CCOCTL_OUTPUT}"

    2023/09/12 11:38:57 Role arn:aws:iam::777777777777:role/<name>-aws-load-balancer-operator-aws-load-balancer-controller created
    2023/09/12 11:38:57 Saved credentials configuration to: /home/user/<credrequests-dir>/manifests/aws-load-balancer-operator-aws-load-balancer-controller-credentials.yaml
    2023/09/12 11:38:58 Updated Role policy for Role <name>-aws-load-balancer-operator-aws-load-balancer-controller created
    ```

    For each `CredentialsRequest` object, `ccoctl` creates an IAM role with a trust
    policy that is tied to the specified OIDC identity provider, and permissions
    policy as defined in each `CredentialsRequest` object. This also generates a set
    of secrets in a `manifests` directory, which are not needed by the controller.

3. Extract and verify the controller's role ARN from the output of `ccoctl` command:

    ```bash
    $ CONTROLLER_ROLEARN=$(grep -Po 'arn:aws:iam[0-9a-z/:\-_]+' "${CCOCTL_OUTPUT}")
    $ echo "${CONTROLLER_ROLEARN}"
    arn:aws:iam::777777777777:role/<name>-aws-load-balancer-operator-aws-load-balancer-controller
    ```

4. Create a controller instance with the role IAM set in the [credentialsRequestConfig.stsIAMRoleARN](./tutorial.md#credentialsrequestconfigstsiamrolearn) field.

#### Option 2. Using the AWS CLI

1. Generate a trusted policy file using your identity provider (e.g. OpenID Connect):

    ```bash
    IDP="<my-oidc-provider-name>"
    IDP_ARN="arn:aws:iam::<my-aws-account>:oidc-provider/${IDP}"
    cat <<EOF > albo-controller-trusted-policy.json
    {
        "Version": "2012-10-17",
        "Statement": [
            {
                "Effect": "Allow",
                "Principal": {
                    "Federated": "${IDP_ARN}"
                },
                "Action": "sts:AssumeRoleWithWebIdentity",
                "Condition": {
                    "StringEquals": {
                        "${IDP}:sub": "system:serviceaccount:aws-load-balancer-operator:aws-load-balancer-controller-cluster"
                    }
                }
            }
        ]
    }
    EOF
    ```

2. Create and verify the role with the generated trusted policy:

    ```bash
    aws iam create-role --role-name albo-controller --assume-role-policy-document file://albo-controller-trusted-policy.json
    CONTROLLER_ROLEARN=$(aws iam get-role --role-name albo-controller | grep '^ROLE' | grep -Po 'arn:aws:iam[0-9a-z/:\-_]+')
    echo $CONTROLLER_ROLEARN
    ```

3. Attach the controller's permission policy to the role:

    ```bash
    curl -o albo-controller-permission-policy.json https://raw.githubusercontent.com/openshift/aws-load-balancer-operator/main/assets/iam-policy.json
    aws iam put-role-policy --role-name albo-controller --policy-name perms-policy-albo-controller --policy-document file://albo-controller-permission-policy.json
    ```

4. Create a controller instance with the role IAM set in the [credentialsRequestConfig.stsIAMRoleARN](./tutorial.md#credentialsrequestconfigstsiamrolearn) field.
