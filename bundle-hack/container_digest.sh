# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='registry.redhat.io/albo/aws-load-balancer-rhel8-operator@sha256:1202d8cf5b9031a7e248ee3d24f9765bc9c31d67e53111ab799c16534a3ec7e1'
# Controller
export OPERAND_IMAGE_PULLSPEC='registry.redhat.io/albo/aws-load-balancer-controller-rhel8@sha256:a800157a787b84a59f2e169ae8a3816e3f25b72c2d5d3239f3521e37e3f01050'
# kube-rbac-proxy
# Latest version of v4.12 tag is used.
# Catalog link (health grade A): https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy/5cdb2634dd19c778293b4d98?image=6ad077325d0c16ad248fe84c
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy@sha256:fa2b9344a6fdd004f5f4750be6a0d27264dd8cf9c2a681efd5229ac7fd86b49a'
