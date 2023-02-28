# Tutorial

## Post Installation

After the operator is installed, create an instance of
`AWSLoadBalancerController`. Note that since only a single instance of the
aws-load-balancer-controller can be installed in a cluster only the resource
with the name `cluster` will be reconciled by the operator.

## AWSLoadBalancerController resource

```yaml
apiVersion: networking.olm.openshift.io/v1alpha1
kind: AWSLoadBalancerController
metadata:
  name: cluster
spec:
  subnetTagging: Auto
  additionalResourceTags:
    example.org/cost-center: 5113232
    example.org/security-scope: staging
  ingressClass: alb
  config:
    replicas: 2
  enabledAddons:
    - AWSShield
    - AWSWAFv2
```

The spec of `AWSLoadBalancerController` resource has fields which are used to
configure the instance of `aws-load-balancer-controller`.

### subnetTagging

This field can take two values:

* Auto
* Manual

When the value is set to `Auto` the operator attempts to determine the subnets
which belong to the cluster and tags them appropriately. It uses the following
logic.

1. Fetch all the subnets that are tagged with the
   key `kubernetes.io/cluster/$CLUSTER_ID`.
2. If the subnet has the tag `kubernetes.io/role/internal-elb` then it's an
   internal subnet.
3. Any subnets without the internal subnet tag are automatically classified as
   public subnets.
4. The tag `kubernetes.io/role/elb` is added to the public subnets.

__Note:__

* The operator cannot determine the role correctly if the internal
subnet tags are not present on internal subnet. So if your cluster is installed
on User-Provisioned Infrastructure then you should manually tag the subnets with
the appropriate role tags and set the subnet tagging policy to `Manual`

* Additional information for subnet tagging if your cluster is installed
on User-Provisioned Infrastructure can be found in [tagging.md](/docs/tagging/tagging.md).

### additionalResourceTags

These tags will be used by the controller when it provisions AWS resources. They
are added to the resource in addition to the cluster tag.

### ingressClass

The default value for this field is `alb`. The operator will provision an
[IngressClass](https://kubernetes.io/docs/concepts/services-networking/ingress/#ingress-class)
with the same name if it does not exist. The controller however is not
restricted to only this Ingress Class. Any _IngressClass_ which has the
`spec.controller` set to `ingress.k8s.aws/alb` will be reconciled by the
controller instance.

### config.replicas

This field can be used to specify the number of replicas of the controller. It
is advised to have at least 2 instances of the controller to ensure availability
of the during updates, relocations, etc. Leader election is automatically
enabled on the controller when more than one replica is specified.

### enabledAddons

This field is used to specify addons for Ingress resources, which will be
specified through annotations. Including the addons has the following effects:

1. `AWSShield` addon enables the
   annotation `alb.ingress.kubernetes.io/shield-advanced-protection`
2. `AWSWAFv1` enables the annotation `alb.ingress.kubernetes.io/waf-acl-id`
3. `AWSWAFv2` enables the annotation `alb.ingress.kubernetes.io/wafv2-acl-arn`

Enabling the addons does not immediately enable the feature on Ingress
resources. Instead, it allows for configuration of the feature through the
annotations.

More information in
the [controller docs](https://kubernetes-sigs.github.io/aws-load-balancer-controller/v2.4/guide/ingress/annotations/#addons)
.

### credentials.name
This field is used to specify the secret name containing AWS credentials to be used by the controller.
The secret specified must be created in the namespace where the operator was installed (by default `aws-load-balancer-operator`).
`credentials` is an optional field. If it's not set, the controller's credentials will be requested using the Cloud Credentials API;
see [Cloud Credentials Operator](https://docs.openshift.com/container-platform/4.11/authentication/managing_cloud_provider_credentials/about-cloud-credential-operator.html).   
The IAM policy required for the controller can be found in [`assets/iam-policy.json`](../assets/iam-policy.json) in this repository.

## Creating an Ingress

Once the controller is running an ALB backed Ingress can be created. The
following example creates an HTTP echo server which responds with the request
body payload.

Create a namespace with the following manifest:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: echoserver
```

Create a deployment with the following manifest:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: echoserver
  namespace: echoserver
spec:
  selector:
    matchLabels:
      app: echoserver
  replicas: 3
  template:
    metadata:
      labels:
        app: echoserver
    spec:
      containers:
        - image: openshift/origin-node
          command:
           - "/bin/socat"
          args:
            - TCP4-LISTEN:8080,reuseaddr,fork
            - EXEC:'/bin/bash -c \"printf \\\"HTTP/1.0 200 OK\r\n\r\n\\\"; sed -e \\\"/^\r/q\\\"\"'
          imagePullPolicy: Always
          name: echoserver
          ports:
            - containerPort: 8080
```

Then create a service which targets the pods:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: echoserver
  namespace: echoserver
spec:
  ports:
    - port: 80
      targetPort: 8080
      protocol: TCP
  type: NodePort
  selector:
    app: echoserver
```

Finally, deploy the ALB backed ingress which targets these pods:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: echoserver
  namespace: echoserver
  annotations:
    alb.ingress.kubernetes.io/scheme: internet-facing
    alb.ingress.kubernetes.io/target-type: instance
spec:
  ingressClassName: alb
  rules:
    - http:
        paths:
          - path: /
            pathType: Exact
            backend:
              service:
                name: echoserver
                port:
                  number: 80

```

Wait for the status of the Ingress to show the host of the provisioned ALB.

```bash
HOST=$(oc get ingress -n echoserver echoserver -o json | jq -r '.status.loadBalancer.ingress[0].hostname')
curl $HOST
```
