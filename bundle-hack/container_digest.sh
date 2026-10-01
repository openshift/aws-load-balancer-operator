# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='registry.redhat.io/albo/aws-load-balancer-rhel8-operator@sha256:bcb52bdd0465dea60130dad11d804dc5348a16cfe89aab315e95fd7a94d05410'
# Controller
export OPERAND_IMAGE_PULLSPEC='registry.redhat.io/albo/aws-load-balancer-controller-rhel8@sha256:2228f275574716d79703244b7d5da3d408bc844014fc92803896991ce9218cfe'
# kube-rbac-proxy
# Latest version of v4.14 tag is used.
# Catalog link (health grade A): https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy/5cdb2634dd19c778293b4d98?image=691eb72e6d4c48dbffa76548
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy@sha256:ba9ff4c933739f1774bc8277d636053c5306863221a8c7b7b9ddc4470eb7feff'
