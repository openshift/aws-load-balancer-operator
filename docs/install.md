# Installation

This documents any required information either during installation or
post installation to ensure the operator can function correctly.

## STS Clusters

### Post operator installation

In an STS Cluster, `CredentialsRequest`s are not automatically provisioned by
the **cloud-credential-operator** and the manual intervention done by the
cluster-admin is required. IAM role and policies as well as the credentials secret need to be provisioned manually for the further consumption by the pods.
`ccoctl` binary can be used to facilitate this task.

Normally, the **aws-load-balancer-operator** relies on the **cloud-credential-operator**
to provision the secret for the operated controller using `CredentialsRequest`. And so in an STS cluster this
secret needs to be provisioned manually. The **aws-load-balancer-operator** will wait until the required
secret is created and available before spawning the **aws-load-balancer-controller** pod.

#### Pre-Requisites

#### [Extract and prepare the `ccoctl` binary](https://docs.openshift.com/container-platform/4.11/authentication/managing_cloud_provider_credentials/cco-mode-sts.html#cco-ccoctl-configuring_cco-mode-sts)

#### Extract required `CredentialsRequests`

1. For `AWSLoadBalancerController` CR the **aws-load-balancer-operator** creates a `CredentialsRequest` named `aws-load-balancer-controller-cluster` in the `openshift-cloud-credential-operator` namespace. Extract and save the created `CredentialsRequest` in a directory:

    ```bash
    oc get credentialsrequest -n openshift-cloud-credential-operator  \
        aws-load-balancer-controller-cluster -o yaml > <path-to-credrequests-dir>/cr.yaml
    ```
    Note: currently `AWSLoadBalancerController` CR can only be named `cluster`

2. Use the `ccoctl` tool to process all `CredentialsRequest` objects from the credrequests directory:

    ```bash
    ccoctl aws create-iam-roles \
        --name <name> --region=<aws_region> \
        --credentials-requests-dir=<path-to-credrequests-dir> \
        --identity-provider-arn <oidc-arn>
    ```

    For each `CredentialsRequest` object, `ccoctl` creates an IAM role with a trust
    policy that is tied to the specified OIDC identity provider, and permissions
    policy as defined in each `CredentialsRequest` object. This also generates a set
    of secrets in a **manifests** directory that is required
    by the **aws-load-balancer-controller**.

3. Apply the secrets to your cluster:

    ```bash
    ls manifests/*-credentials.yaml | xargs -I{} oc apply -f {}
    ```

4. Verify that the corresponding **aws-load-balancer-controller** pod was created:

    ```bash
    oc -n aws-load-balancer-operator get pods
    NAME                                                            READY   STATUS    RESTARTS   AGE
    aws-load-balancer-controller-cluster-9b766d6-gg82c              1/1     Running   0          137m
    aws-load-balancer-operator-controller-manager-b55ff68cc-85jzg   2/2     Running   0          3h26m
    ```

### Use predefined `CredentialsRequest`
In case the provisioning of the credentials secret should not be done by the **cloud-credential-operator**, the secret needs to be explicitly referenced in `AWSLoadBalancerController` CR, see [credentials.name field description](./tutorial.md#credentialsname).    
However, this credentials secret needs to reference a role with all the policies needed by the controller. For this purpose a dedicated controller's `CredentialsRequest` is maintained in [hack/controller](../hack/controller/) directory of this repository.
Its contents are identical to the ones requested from the **cloud-credential-operator**.

1. Use the `ccoctl` tool to process the controller's `CredentialsRequest` object:

    ```bash
    ccoctl aws create-iam-roles \
        --name <name> --region=<aws_region> \
        --credentials-requests-dir=hack/controller \
        --identity-provider-arn <oidc-arn>
    ```

    For each `CredentialsRequest` object, `ccoctl` creates an IAM role with a trust
    policy that is tied to the specified OIDC identity provider, and permissions
    policy as defined in each `CredentialsRequest` object. This also generates a set
    of secrets in a **manifests** directory that is required
    by the **aws-load-balancer-controller**.

2. Apply the secrets to your cluster:

    ```bash
    ls manifests/*-credentials.yaml | xargs -I{} oc apply -f {}
    ```

3. Verify that the controller's credentials secret is created:

    ```bash
    oc -n aws-load-balancer-operator get secret aws-load-balancer-controller-manual-cluster -o json | jq -r '.data.credentials' | base64 -d
    [default]
    sts_regional_endpoints = regional
    role_arn = arn:aws:iam::999999999999:role/aws-load-balancer-operator-aws-load-balancer-controller
    web_identity_token_file = /var/run/secrets/openshift/serviceaccount/token
    ```
