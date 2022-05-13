# Tagging Pre-requistes

The `aws-load-balancer-operator` requires specific tags on some of the aws
resources to function correctly. They are as follows:

## VPC

The VPC of the cluster on which the operator is running should have the tag
`kubernetes.io/cluster/${CLUSTER_ID}`. This is used by the operator to pass
the VPC ID to the controller. When the cluster is provisioned with _Installer-Provisioned Infrastructure (IPI)_,
the tag is added by the installer. But in a _User-Provisioned Infrastructure (UPI)_
cluster the user must tag the VPC as follows:

| Key                                     | Value                 |
| --------------------------------------- | --------------------- |
| `kubernetes.io/cluster/${CLUSTER_ID}`   | `owned` or `shared`   |

## Subnets

When `spec.subnetTagging` value is set to `Auto` the operator attempts to
determine the subnets which belong to the cluster and tags them appropriately.
When the cluster has been installed with _User Provisioned Infrastructure_ the subnets
do not have the tags for the controller to function correctly. In this case the user should tag
the subnets themselves and set the `spec.subnetTagging` field to `Manual`. The tags should
have the following values:

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
