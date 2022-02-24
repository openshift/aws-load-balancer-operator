# aws-load-balancer-operator

### Running the operator

1. Build and push the image to an image registry.
    ```bash
    export IMG=example.com/aws-load-balancer-operator:latest
    make image-build image-push
    ```
2. Create AWS credentials for the operator
    ```bash
    oc create secret generic aws-load-balancer-operator -n aws-load-balancer-operator \
    --from-literal=aws_access_key_id=$AWS_ACCESS_KEY_ID \
    --from-literal=aws_secret_access_key=$AWS_SECRET_ACCESS_KEY \
    ```
   Alternatively use the `CredentialRequest` resource in the `hack` directory
3. Deploy the operator
  ```bash
  make deploy
  ```