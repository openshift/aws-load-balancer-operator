# Pre-Requisites

## CredentialsRequest

### For non-STS clusters

1. Additional AWS credentials are needed for the operator to be successfully
   installed. This is needed to interact with subnets and VPCs.
2. Create AWS credentials profile for the operator

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
  
3. Alternatively use the *CredentialsRequest* resource in the `hack` directory

   ```bash
   oc apply -f https://raw.githubusercontent.com/openshift/aws-load-balancer-operator/main/hack/operator-credentials-request.yaml
   ```

### For STS clusters

The same steps mentioned above can be followed for STS clusters as well.
But if instead of manually creating the secret in `step 2`, you prefer
`step 3`, then there are additional steps involved.

1. After creating the *CredentialRequests*,

    ```bash
    oc get credentialsrequest -n openshift-cloud-credential-operator  \
        aws-load-balancer-controller -o yaml > <path-to-credrequests-dir>/cr.yaml
    ```

    Extract and save the required *CredentialsRequest* in a directory.

2. Use the ccoctl tool to process all *CredentialsRequest* objects in the previously specified
directory:

    ```bash
    ccoctl aws create-iam-roles \
        --name <name> --region=<aws_region> \
        --credentials-requests-dir=<path-to-credrequests-dir> \
        --identity-provider-arn <oidc-arn>
    ```

    For each *CredentialsRequest* object, `ccoctl` creates an IAM role with a trust
    policy that is tied to the specified OIDC identity provider, and permissions
    policy as defined in each *CredentialsRequest* object. This also generates a set
    of secrets in a **manifests** directory that is required
    by the **aws-load-balancer-operator**.

    **Note**: To Extract and prepare the `ccoctl` binary, documentation can be
    found [here](https://docs.openshift.com/container-platform/4.10/authentication/managing_cloud_provider_credentials/cco-mode-sts.html#cco-ccoctl-configuring_cco-mode-sts).

3. Apply the secrets to your cluster:

    ```bash
    ls manifests/*-credentials.yaml | xargs -I{} oc apply -f {}
    ```

## VPC and Subnets

The `aws-load-balancer-operator` requires specific tags on some of the aws
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
