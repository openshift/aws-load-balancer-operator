# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='registry.redhat.io/albo/aws-load-balancer-rhel8-operator@sha256:3216ac39cd2353b73f540ff06b60b4f26ce1d47095f32c6b3f522f48906084e7'
# Controller
export OPERAND_IMAGE_PULLSPEC='registry.redhat.io/albo/aws-load-balancer-controller-rhel8@sha256:a800157a787b84a59f2e169ae8a3816e3f25b72c2d5d3239f3521e37e3f01050'
# kube-rbac-proxy
# Latest version of v4.12 tag is used.
# Catalog link (health grade A): https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy/5cdb2634dd19c778293b4d98?image=6aab84313772c4c0c3adae20
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy@sha256:sha256:1942b0309b8978f20cdd55875a363b0c16a52646745db6deb0c47546343ad2af'
