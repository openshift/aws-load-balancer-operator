# Pre-Requisites

- [AWS credentials](#aws-credentials)
    - [For non-STS clusters](#for-non-sts-clusters)
    - [For STS clusters](#for-sts-clusters)
        - [Option 1. Using ccoctl](#option-1-using-ccoctl)
        - [Option 2. Using the AWS CLI](#option-2-using-the-aws-cli)
- [VPC and Subnets](#vpc-and-subnets)
    - [VPC](#vpc)
    - [Subnets](#subnets)
        - [Public subnets](#public-subnets)
        - [Private subnets](#private-subnets)

## AWS credentials
Additional AWS credentials are needed for the operator to be successfully installed. This is needed to interact with subnets and VPCs.

### For non-STS clusters

1. Create AWS credentials profile for the operator:

    ```bash
    cat << EOF > credentials
    [default]
    aws_access_key_id=${AWS_ACCESS_KEY_ID}
    aws_secret_access_key=${AWS_SECRET_ACCESS_KEY}
    EOF
    
    oc create secret generic aws-load-balancer-operator \
    -n aws-load-balancer-operator \
    --from-file=credentials=credentials
    ```
  
2. Alternatively use the `CredentialsRequest` resource in the `hack` directory:

   ```bash
   oc apply -f https://raw.githubusercontent.com/openshift/aws-load-balancer-operator/main/hack/operator-credentials-request.yaml
   ```

### For STS clusters

There are two options for creating the credentials secret:
- Using a pre-defined `CredentialsRequest`.
- Using pre-defined AWS manifests.

For handling `CredentialsRequests`, the cloud credential operator utility, [ccoctl](https://docs.openshift.com/container-platform/latest/authentication/managing_cloud_provider_credentials/cco-mode-sts.html#cco-ccoctl-configuring_cco-mode-sts), can be utilized.
If you prefer not to use `ccoctl`, or your system doesn't support it, the AWS CLI can be an alternative.

#### Option 1. Using `ccoctl`

1. [Extract and prepare the `ccoctl` binary](https://docs.openshift.com/container-platform/4.13/authentication/managing_cloud_provider_credentials/cco-mode-sts.html#cco-ccoctl-configuring_cco-mode-sts)

2. Create AWS Load Balancer Operator's namespace:

    ```bash
    oc create namespace aws-load-balancer-operator
    ```

3. Use the `ccoctl` tool to process the operator's `CredentialsRequest` objects needed to bootstrap the operator:

    ```bash
    curl --create-dirs -o <path-to-credrequests-dir>/cr.yaml https://raw.githubusercontent.com/openshift/aws-load-balancer-operator/main/hack/operator-credentials-request.yaml
    ccoctl aws create-iam-roles \
        --name <name> --region=<aws_region> \
        --credentials-requests-dir=<path-to-credrequests-dir> \
        --identity-provider-arn <oidc-arn>
    ```

    For each `CredentialsRequest` object, `ccoctl` creates an IAM role with a trust
    policy that is tied to the specified OIDC identity provider, and permissions
    policy as defined in each `CredentialsRequest` object. This also generates a set
    of secrets in a **manifests** directory that is required
    by the **aws-load-balancer-operator**.

4. Apply the secrets to your cluster:

    ```bash
    ls manifests/*-credentials.yaml | xargs -I{} oc apply -f {}
    ```

5. Verify that the operator's credentials secret is created:

    ```bash
    oc -n aws-load-balancer-operator get secret aws-load-balancer-operator -o json | jq -r '.data.credentials' | base64 -d
    [default]
    sts_regional_endpoints = regional
    role_arn = arn:aws:iam::999999999999:role/aws-load-balancer-operator-aws-load-balancer-operator
    web_identity_token_file = /var/run/secrets/openshift/serviceaccount/token
    ```

#### Option 2. Using the AWS CLI

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
    OPERATOR_ROLE_ARN=$(aws iam get-role --role-name albo-operator | \grep '^ROLE' | \grep -Po 'arn:aws:iam[0-9a-z/:\-_]+')
    echo $OPERATOR_ROLE_ARN
    ```

4. Attach the operator's permission policy to the role:

    ```bash
    curl -o albo-operator-permission-policy.json https://raw.githubusercontent.com/openshift/aws-load-balancer-operator/release-1.0/hack/operator-permission-policy.json
    aws iam put-role-policy --role-name albo-operator --policy-name perms-policy-albo-operator --policy-document file://albo-operator-permission-policy.json
    ```

5. Generate the operator's aws credentials:

    ```bash
    cat <<EOF > albo-operator-aws-credentials.cfg
    [default]
    sts_regional_endpoints = regional
    role_arn = ${OPERATOR_ROLE_ARN}
    web_identity_token_file = /var/run/secrets/openshift/serviceaccount/token
    EOF
    ```
    **Note**: mind the format of the credentials file, examples can be found in [OCP documentation](https://docs.openshift.com/container-platform/4.13/authentication/managing_cloud_provider_credentials/cco-mode-sts.html#sts-mode-about_cco-mode-sts).

6. Create the operator's credentials secret with the generated aws credentials:

    ```bash
    oc -n aws-load-balancer-operator create secret generic aws-load-balancer-operator --from-file=credentials=albo-operator-aws-credentials.cfg
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
