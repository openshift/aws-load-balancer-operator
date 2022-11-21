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
