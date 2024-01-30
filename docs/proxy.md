# Configuring egress proxy for AWS Load Balancer Operator

If a cluster wide egress proxy is configured on the OpenShift cluster, OLM automatically updates all the operators' deployments with `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY` environment variables.
Those variables are then propagated down to the managed controller by the AWS Load Balancer Operator.

## Trusted Certificate Authority

AWS Load Balancer Operator will make use of the OpenShift cluster global trust bundle. Should you need to trust a
custom Certificate Authority (CA), follow the OpenShift documentation to add an global trusted CA.
