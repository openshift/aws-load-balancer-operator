# Configuring egress proxy for AWS Load Balancer Operator

If a cluster wide egress proxy is configured on the OpenShift cluster, OLM automatically updates all the operators' deployments with `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY` environment variables.

## Trusted Certificate Authority

### Running operator
Follow the instructions below to let AWS Load Balancer Operator trust a custom Certificate Authority (CA). The operator's OLM subscription has to have been created first.
The operator's deployment doesn't have to be ready though.

1. Create the configmap containing the CA bundle in `aws-load-balancer-operator` namespace.
    ```bash
    oc -n aws-load-balancer-operator create configmap trusted-ca
    oc -n aws-load-balancer-operator label cm trusted-ca config.openshift.io/inject-trusted-cabundle=true
    ```

2. Consume the created configmap in AWS Load Balancer Operator's deployment by updating its subscription:

    ```bash
    oc -n aws-load-balancer-operator patch subscription aws-load-balancer-operator --type='merge' -p '{"spec":{"config":{"volumes":[{"name":"trusted-ca","configMap":{"name":"trusted-ca"}}],"volumeMounts":[{"name":"trusted-ca","mountPath":"/etc/pki/tls/certs/albo-tls-ca-bundle.crt","subPath":"ca-bundle.crt"}]}}}'
    ```

3. Wait for the operator deployment to finish the rollout and verify that CA bundle is added:

    ```bash
    oc -n aws-load-balancer-operator exec deploy/aws-load-balancer-operator-controller-manager -c manager -- ls -l /etc/pki/tls/certs/albo-tls-ca-bundle.crt

    -rw-r--r--. 1 root 1000690000 5875 Jan 11 12:25 /etc/pki/tls/certs/albo-tls-ca-bundle.crt
    ```

4. _Optional_: make sure the operator is restarted every time the configmap contents change:

    ```bash
    oc -n aws-load-balancer-operator rollout restart deployment/aws-load-balancer-operator-controller-manager
    ```
