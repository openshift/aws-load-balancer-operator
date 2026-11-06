# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='registry.redhat.io/albo/aws-load-balancer-rhel8-operator@sha256:f626b372d0bcf203481891f362d5e01866952ccd23907f436a6c549ce73099dc'
# Controller
export OPERAND_IMAGE_PULLSPEC='registry.redhat.io/albo/aws-load-balancer-controller-rhel8@sha256:8453ce4bf171de01f4b78586c57933fcecae00a30aba51b8ec85e29d3629da1d'
# kube-rbac-proxy
# Latest version of v4.12 tag is used.
# Catalog link (health grade A): https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy/5cdb2634dd19c778293b4d98?image=6ad077325d0c16ad248fe84c
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy@sha256:fa2b9344a6fdd004f5f4750be6a0d27264dd8cf9c2a681efd5229ac7fd86b49a'
