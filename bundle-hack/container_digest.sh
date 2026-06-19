# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='registry.redhat.io/albo/aws-load-balancer-rhel9-operator@sha256:b59ee634581b767eb547327926de12d0c01bd2c348bed9d9d293e1b96e3c197c'
# Controller
export OPERAND_IMAGE_PULLSPEC='registry.redhat.io/albo/aws-load-balancer-controller-rhel9@sha256:fcb63bc601772fe5ef4740c46c0c2f158e5ab1228b4f01170feb1fe40c694b61'
# kube-rbac-proxy
# Latest version of v4.19 tag is used.
# Catalog link (health grade A): https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy-rhel9/652809a5244cb343fb4a4b66?image=6a291e91c7ee40ca259b3f3a
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy-rhel9@sha256:32540431240e12c07d35f9f390b196aae5cc2188e9db6365e41e6bbe7070d8c2'
