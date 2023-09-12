# Pre-Requisites

In certain scenarios, the operator requires extra steps to be executed before it can be installed.

- [IAM Role for STS clusters](#iam-role-for-sts-clusters)
    - [Option 1. Using ccoctl](#option-1-using-ccoctl)
    - [Option 2. Using the AWS CLI](#option-2-using-the-aws-cli)
- [VPC and Subnets](#vpc-and-subnets)
    - [VPC](#vpc)
    - [Subnets](#subnets)
        - [Public subnets](#public-subnets)
        - [Private subnets](#private-subnets)

## IAM Role for STS clusters
An additional IAM Role is needed for the operator to be successfully installed in STS clusters. This is needed to interact with subnets and VPCs.
The operator will generate a `CredentialsRequest` with this role to self bootstrap with AWS credentials.

There are two options for creating the IAM role:
- Using a pre-defined `CredentialsRequest`.
- Using pre-defined AWS manifests.

For handling `CredentialsRequests`, the cloud credential operator utility, [`ccoctl`](https://docs.openshift.com/container-platform/latest/authentication/managing_cloud_provider_credentials/cco-mode-sts.html#cco-ccoctl-configuring_cco-mode-sts), can be utilized.
If you prefer not to use `ccoctl`, or your system doesn't support it, the AWS CLI can be an alternative.

### Option 1. Using `ccoctl`

1. [Extract and prepare the `ccoctl` binary](https://docs.openshift.com/container-platform/4.13/authentication/managing_cloud_provider_credentials/cco-mode-sts.html#cco-ccoctl-configuring_cco-mode-sts)

2. Use the `ccoctl` tool to create a IAM role from the operator's `CredentialsRequest`:

    ```bash
   $ curl --create-dirs -o <path-to-credrequests-dir>/cr.yaml https://raw.githubusercontent.com/openshift/aws-load-balancer-operator/main/hack/operator-credentials-request.yaml
   $ CCOCTL_OUTPUT=$(mktemp)
   $ ccoctl aws create-iam-roles \
        --name <name> --region=<aws_region> \
        --credentials-requests-dir=<path-to-credrequests-dir> \
        --identity-provider-arn <oidc-arn> 2>&1 | tee "${CCOCTL_OUTPUT}"

    2023/09/12 11:38:57 Role arn:aws:iam::777777777777:role/<name>-aws-load-balancer-operator-aws-load-balancer-operator created
    2023/09/12 11:38:57 Saved credentials configuration to: /home/user/<credrequests-dir>/manifests/aws-load-balancer-operator-aws-load-balancer-operator-credentials.yaml
    2023/09/12 11:38:58 Updated Role policy for Role <name>-aws-load-balancer-operator-aws-load-balancer-operator created
    ```

    For each `CredentialsRequest` object, `ccoctl` creates an IAM role with a trust
    policy that is tied to the specified OIDC identity provider, and permissions
    policy as defined in each `CredentialsRequest` object. This also generates a set
    of secrets in a `manifests` directory, which are not needed by the operator.

3. Extract and verify the operator's role ARN from the output of `ccoctl` command:

    ```bash
    $ ROLEARN=$(grep -Po 'arn:aws:iam[0-9a-z/:\-_]+' "${CCOCTL_OUTPUT}")
    $ echo "${ROLEARN}"
    arn:aws:iam::777777777777:role/<name>-aws-load-balancer-operator-aws-load-balancer-operator
    ```

4. Install the operator using the OpenShift OperatorHub web UI or by running the following commands:

    ```bash
    $ oc create namespace aws-load-balancer-operator

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

### Option 2. Using the AWS CLI

1. Create AWS Load Balancer Operator's namespace:

    ```bash
    oc create namespace aws-load-balancer-operator
    ```

2. Generate a trusted policy file using your identity provider (e.g. OpenID Connect):

    ```bash
    IDP="<my-oidc-provider-name>"
    IDP_ARN="arn:aws:iam::<my-aws-account>:oidc-provider/${IDP}"
    cat <<EOF > albo-operator-trusted-policy.json
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
                        "${IDP}:sub": "system:serviceaccount:aws-load-balancer-operator:aws-load-balancer-operator-controller-manager"
                    }
                }
            }
        ]
    }
    EOF
    ```

3. Create and verify the role with the generated trusted policy:

    ```bash
    aws iam create-role --role-name albo-operator --assume-role-policy-document file://albo-operator-trusted-policy.json
    ROLEARN=$(aws iam get-role --role-name albo-operator | \grep '^ROLE' | \grep -Po 'arn:aws:iam[0-9a-z/:\-_]+')
    echo $ROLEARN
    ```

4. Attach the operator's permission policy to the role:

    ```bash
    curl -o albo-operator-permission-policy.json https://raw.githubusercontent.com/openshift/aws-load-balancer-operator/main/hack/operator-permission-policy.json
    aws iam put-role-policy --role-name albo-operator --policy-name perms-policy-albo-operator --policy-document file://albo-operator-permission-policy.json
    ```

4. Install the operator using the OpenShift OperatorHub web UI or by running the following commands:

    ```bash
    $ oc create namespace aws-load-balancer-operator

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
## VPC and Subnets

The **aws-load-balancer-operator** requires specific tags on some AWS
resources to function correctly. They are as follows:

### VPC

The VPC of the cluster on which the operator is running should have the tag
`kubernetes.io/cluster/${CLUSTER_ID}`. This is used by the operator to pass
the VPC ID to the controller. When the cluster is provisioned with *Installer-Provisioned Infrastructure (IPI)*,
the tag is added by the installer. But in a *User-Provisioned Infrastructure (UPI)*
cluster the user must tag the VPC as follows:

| Key                                     | Value                 |
| --------------------------------------- | --------------------- |
| `kubernetes.io/cluster/${CLUSTER_ID}`   | `owned` or `shared`   |

### Subnets

When `spec.subnetTagging` value is set to `Auto` the operator attempts to
determine the subnets which belong to the cluster and tags them appropriately.
When the cluster has been installed with *User Provisioned Infrastructure* the subnets
do not have the tags for the controller to function correctly. In this case the user should tag
the subnets themselves and set the `spec.subnetTagging` field to `Manual`. The tags should
have the following values:

#### Public subnets

Public subnets are used for internet-facing load balancers. These subnets must
have the following tags:

| Key                                     | Value                 |
| --------------------------------------- | --------------------- |
| `kubernetes.io/role/elb`                | `1`  or ``            |

#### Private subnets

Private subnets are used for internal load balancers. These subnets must have
the following tags:

| Key                                     | Value                 |
| --------------------------------------- | --------------------- |
|  `kubernetes.io/role/internal-elb`      |  `1`  or ``           |
