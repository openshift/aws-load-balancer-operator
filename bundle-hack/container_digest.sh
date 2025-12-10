# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='quay.io/redhat-user-workloads/aws-load-balancer-operator-tenant/aws-lb-optr-1-3-rhel-9/aws-load-balancer-operator-container-aws-lb-optr-1-3-rhel-9@sha256:56c65884dab411f16c6f7c407a8f83be502fd4777712f12c12d4267792cd5412'
# Controller
export OPERAND_IMAGE_PULLSPEC='quay.io/redhat-user-workloads/aws-load-balancer-operator-tenant/aws-lb-optr-1-3-rhel-9/aws-load-balancer-controller-container-aws-lb-optr-1-3-rhel-9@sha256:577016038b4324e3ebc7fc5a9756594206c58d81b7f630884c0f20b565cc7ebb'
# kube-rbac-proxy
# Latest version of v4.14 tag is used.
# Catalog link (health grade A): https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy/5cdb2634dd19c778293b4d98?image=691eb72e6d4c48dbffa76548
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy@sha256:ba9ff4c933739f1774bc8277d636053c5306863221a8c7b7b9ddc4470eb7feff'
