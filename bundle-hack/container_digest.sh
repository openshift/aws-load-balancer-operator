# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='registry.redhat.io/albo/aws-load-balancer-rhel8-operator@sha256:81c953bc7b8808b5bb8b2f027d9a97ab26ff4516842fa2a86481b8a6ad04b96b'
# Controller
export OPERAND_IMAGE_PULLSPEC='registry.redhat.io/albo/aws-load-balancer-controller-rhel8@sha256:e76661808a8f29a170f8a6607bd6d70f3f54eed00799ecd2d98bd920804a638d'
# kube-rbac-proxy
# Latest version of v4.12 tag is used.
# Catalog link (health grade A): https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy/5cdb2634dd19c778293b4d98?image=6ad077325d0c16ad248fe84c
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy@sha256:fa2b9344a6fdd004f5f4750be6a0d27264dd8cf9c2a681efd5229ac7fd86b49a'
