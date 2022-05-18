# AWS Load Balancer Operator

This operator is used to install, manage and configure an instance of
[aws-load-balancer-controller](https://github.com/kubernetes-sigs/aws-load-balancer-controller/)
in a OpenShift cluster.

This [document](https://github.com/openshift/enhancements/blob/master/enhancements/ingress/aws-load-balancer-operator.md)
describes the design and implementation of the operator in more detail.

## Table of contents

1. [Prerequisites](/docs/prerequisites.md)
   1. [CredentialsRequest](/docs/prerequisites.md#credentialsrequest)
   2. [VPC and Subnets](/docs/prerequisites.md#vpc-and-subnets)
2. [Installation](/docs/install.md)
   1. [STS Clusters](/docs/install.md#sts-clusters)
3. [Tutorial](/docs/tutorial.md)
4. [Local Development](#local-development)

## Local Development

### Build the operand image

The operand image must be built first. Our fork of the operand
is [here](https://github.com/openshift/aws-load-balancer-controller/). Clone 
the repository and build the image and push it to a registry which is 
accessible from the test cluster.

```bash
git clone https://github.com/openshift/aws-load-balancer-controller.git
IMG=quay.io/$USER/aws-load-balancer-controller
podman build -t $IMG -f Dockerfile.openshift
podman push $IMG
```

### Running the operator

1. Replace the operand image in the file `config/manager/manager.yaml` in 
   the environment variable `RELATED_IMAGE_CONTROLLER` with the image 
   created in the previous step.
2. Build and push the image to an image registry.
    ```bash
    export IMG=quay.io/$USER/aws-load-balancer-operator:latest
    make image-build image-push
    ```
3. Create the namespace where the operator will be deployed.
   ```bash
   oc create ns aws-load-balancer-operator
   ```
4. Create AWS credentials profile for the operator
    ```bash
    cat << EOF > credentials
    [default]
    aws_access_key_id=${AWS_ACCESS_KEY_ID}
    aws_secret_access_key=${AWS_SECRET_ACCESS_KEY}
    EOF
    
    oc create secret generic aws-load-balancer-operator -n aws-load-balancer-operator \
    --from-file=credentials=credentials
    ```
   Alternatively use the `CredentialsRequest` resource in the `hack` directory
   ```bash
   oc apply -f hack/operator-credentials-request.yaml
   ```
5. Deploy the operator
    ```bash
    make deploy
    ```
   
### Running the end-to-end tests

After the operator has been deployed as described previously you can run the e2e
tests with the following command:

```bash
make test-e2e
```
