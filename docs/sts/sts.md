# Using manual mode with STS

In an STS Cluster, *CredentialsRequests* are not automatically provisioned by
the `cloud-credential-operator` and requires manual intervention by the
cluster-admin. The credentials secret needs to be provisioned using the `ccoctl` binary.

The `aws-load-balancer-operator` also relies on the `cloud-credential-operator`
to provision the secret for *CredentialsRequest*. And so in an STS Cluster the
secret needs to be provisioned manually. The operator will wait until the required
secrets are created and available.

## Pre-Requisites

### [Extract and prepare the `ccoctl` binary](https://docs.openshift.com/container-platform/4.10/authentication/managing_cloud_provider_credentials/cco-mode-sts.html#cco-ccoctl-configuring_cco-mode-sts)

## Extract required `CredentialsRequests`

1. The **aws-load-balancer-operator** creates a *CredentialsRequest* named
`aws-load-balancer-controller-<cr-name>` in the *openshift-cloud-credential-operator* namespace.

    ```bash
    oc get credentialsrequest -n openshift-cloud-credential-operator  \
        aws-load-balancer-controller-<cr-name> -o yaml > <path-to-credrequests-dir>/cr.yaml
    ```

    Extract and save the required *CredentialsRequest* in a directory.

2. Use the ccoctl tool to process all *CredentialsRequest* objects in the credrequests
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

3. Apply the secrets to your cluster:

    ```bash
    ls manifests/*-credentials.yaml | xargs -I{} oc apply -f {}
    ```
