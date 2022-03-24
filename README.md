# aws-load-balancer-operator

This operator is used to install, manage and configure an instance of
[aws-load-balancer-controller](https://github.com/kubernetes-sigs/aws-load-balancer-controller/)
in a OpenShift cluster.

### Running the operator

1. Build and push the image to an image registry.
    ```bash
    export IMG=example.com/aws-load-balancer-operator:latest
    make image-build image-push
    ```
2. Create AWS credentials profile for the operator
    ```bash
    cat << EOF > credentials
    [default]
    aws_access_key_id=${AWS_ACCESS_KEY_ID}
    aws_secret_access_key=${AWS_SECRET_ACCESS_KEY}
    EOF
    
    oc create secret generic aws-load-balancer-operator -n aws-load-balancer-operator \
    --from-file=credentials=credentials
    ```
   Alternatively use the `CredentialRequest` resource in the `hack` directory
3. Deploy the operator
    ```bash
    make deploy
    ```

TEST
