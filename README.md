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
    1. [Build the operand image](#build-the-operand-image)
    2. [Running the operator](#running-the-operator)
    3. [Running the end-to-end tests](#running-the-end-to-end-tests)
    4. [Running the end-to-end tests on ROSA STS cluster](#running-the-end-to-end-tests-on-rosa-sts-cluster)
5. [Proxy support](#proxy-support)

## Local Development

### Build the operand image

**Note**: only needed for unmerged changes, all merged changes get published in a public quay.io repository

The operand image must be built first. Clone [the OpenShift fork of the operand](https://github.com/openshift/aws-load-balancer-controller),
build the image and push it to a registry which is accessible from the test cluster.

```bash
git clone https://github.com/openshift/aws-load-balancer-controller.git
cd aws-load-balancer-controller
IMG=quay.io/$USER/aws-load-balancer-controller
podman build -t $IMG -f Dockerfile.openshift
podman push $IMG
```

### Running the operator

1. Replace the operand image in the file `config/manager/manager.yaml` in 
   the environment variable `RELATED_IMAGE_CONTROLLER` with the image 
   created in the previous step.
2. Build and push the operator image to an image registry.
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
6. The previous step deploys the conversion webhook, which requires TLS verification on the webhook client side. The
   manifests deployed through the `make deploy` command do not contain a valid certificate and key. You must provision a valid certificate and key through other tools.     
   If you run on OpenShift, you can use a convenience script, `hack/add-serving-cert.sh`, to enable [the service serving certificate feature](https://docs.openshift.com/container-platform/4.12/security/certificates/service-serving-certificate.html). 
   Run the `hack/add-serving-cert.sh` script with the following inputs:
   ```bash
   hack/add-serving-cert.sh --namespace aws-load-balancer-operator --service aws-load-balancer-operator-webhook-service --secret webhook-server-cert --crd awsloadbalancercontrollers.networking.olm.openshift.io
   ```
   *Note*: You may need to wait for the retry of the volume mount in the operator's pod.

### Running the end-to-end tests

After the operator has been deployed as described previously you can run the e2e
tests with the following command:

```bash
make test-e2e
```

### Running the end-to-end tests on ROSA STS cluster

**Prerequisistes**:
- The operator has to be deployed with [the prerequisites for the STS cluster](./docs/prerequisites.md#for-sts-clusters).
- The controller's secret needs to be created as described in [the installation instructions for the STS cluster](./docs/install.md#post-operator-installation).
- The test WAFv2 and WAF regional WebACLs need to be created. You can use the following commands:
```bash
aws wafv2 create-web-acl --name "echoserver-acl" --scope REGIONAL --default-action '{"Block":{}}'  --visibility-config '{"MetricName":"echoserver","CloudWatchMetricsEnabled": false,"SampledRequestsEnabled":false}'
aws waf-regional create-web-acl --name "echoserverclassicacl" --metric-name "echoserverclassicacl" --default-action '{"Type":"BLOCK"}' --change-token "$(aws waf-regional get-change-token)"
```
**Note**: note the ARN and ID of the created ACLs from the output of the commands

Now you can run the e2e test with the following commands:
```bash
export ALBO_E2E_PLATFORM=ROSA
export ALBO_E2E_WAFV2_WEBACL_ARN=<wafv2-webacl-arn>
export ALBO_E2E_WAF_WEBACL_ID=<wafregional-webacl-id>
make test-e2e
```

## Proxy support

[Configuring egress proxy for AWS Load Balancer Operator](./docs/proxy.md)
