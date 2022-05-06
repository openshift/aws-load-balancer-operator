# Tagging Pre-requistes

The `aws-load-balancer-operator` requires specific tags on some of the aws
resources to function appropriately. These are documented as follows:

## VPC

The operator requires the VPC ID during start up and this is fetched using
`kubernetes.io/cluster/${CLUSTER_ID}` tag. This is automatically done
when the cluster is installed in an Installer-Provisioned Infrastructure (IPI),
but in a User-Provisioned Infrastructure (UPI) the user must tag the VPC as follows:

| Key                                     | Value                 |
| --------------------------------------- | --------------------- |
| `kubernetes.io/cluster/${CLUSTER_ID}`   | `owned` or `shared`   |

## Subnets

When `spec.subnetTagging` value is set to `Auto` the operator attempts to
determine the subnets which belong to the cluster and tags them appropriately.
To apply the logic mentioned in [subnettagging](/docs/tutorial.md#subnettagging)
in UPI clusters, the subnets must be tagged as follows:

### Public subnets

Public subnets are used for internet-facing load balancers. These subnets must
have the following tags:

| Key                                     | Value                 |
| --------------------------------------- | --------------------- |
| `kubernetes.io/role/elb`                | `1`  or ``            |

### Private subnets

Private subnets are used for internal load balancers. These subnets must have
the following tags:

| Key                                     | Value                 |
| --------------------------------------- | --------------------- |
|  `kubernetes.io/role/internal-elb`      |  `1`  or ``           |
