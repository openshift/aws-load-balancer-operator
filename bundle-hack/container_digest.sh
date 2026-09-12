# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='registry.redhat.io/albo/aws-load-balancer-rhel9-operator@sha256:300192ffc1bf1b7012dd1d034b70febab17844d885e8bc833fb2903b995f2020'
# Controller
export OPERAND_IMAGE_PULLSPEC='registry.redhat.io/albo/aws-load-balancer-controller-rhel9@sha256:e416e2d857b3b2481f102611b70f5e2df71463fe7ea03bf3cc57cc9985c41552'
# kube-rbac-proxy
# Latest version of v4.19 tag is used.
# Catalog link (health grade A): https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy-rhel9/652809a5244cb343fb4a4b66?image=6a291e91c7ee40ca259b3f3a
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy-rhel9@sha256:32540431240e12c07d35f9f390b196aae5cc2188e9db6365e41e6bbe7070d8c2'
