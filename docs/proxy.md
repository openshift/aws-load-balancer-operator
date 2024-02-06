# Configuring egress proxy for AWS Load Balancer Operator

If a cluster wide egress proxy is configured on the OpenShift cluster, OLM automatically updates all the operators' deployments with `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY` environment variables.
Those variables are then propagated down to the managed controller by the AWS Load Balancer Operator.

## Trusted Certificate Authority

AWS Load Balancer Operator will make use of the OpenShift cluster-wide trusted CA bundle. Should you need to trust a custom Certificate Authority (CA), follow [the OpenShift documentation to configure a custom PKI](https://docs.openshift.com/container-platform/latest/networking/configuring-a-custom-pki.html).

In order for changes to the cluster-wide trusted CA bundle to take affect, the operator needs to be restarted:

```bash
oc -n aws-load-balancer-operator rollout restart deployment/aws-load-balancer-operator-controller-manager
```
