# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='registry.redhat.io/albo/aws-load-balancer-rhel9-operator@sha256:772794e9a6cf910a7fe75ad43df7ee7ace6acbf60896ba46c510a74cf35567c0'
# Controller
export OPERAND_IMAGE_PULLSPEC='registry.redhat.io/albo/aws-load-balancer-controller-rhel9@sha256:992c4fb804e11ccef4a0d3ed7764f26176af02554390bdb22548b3ec4a97a9be'
# kube-rbac-proxy
# Latest version of v4.19 tag is used.
# Catalog link (health grade A): https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy-rhel9/652809a5244cb343fb4a4b66?image=6a291e91c7ee40ca259b3f3a
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy-rhel9@sha256:32540431240e12c07d35f9f390b196aae5cc2188e9db6365e41e6bbe7070d8c2'
