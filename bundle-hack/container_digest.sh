# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='registry.redhat.io/albo/aws-load-balancer-rhel8-operator@sha256:bcb52bdd0465dea60130dad11d804dc5348a16cfe89aab315e95fd7a94d05410'
# Controller
export OPERAND_IMAGE_PULLSPEC='registry.redhat.io/albo/aws-load-balancer-controller-rhel8@sha256:8d49aa64aad7bf98d66dc8aab36e2cb104cfdd610fc68b36db781449ca0a3cba'
# kube-rbac-proxy
# Latest version of v4.12 tag is used.
# Catalog link (health grade A): https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy/5cdb2634dd19c778293b4d98?image=6aab84313772c4c0c3adae20
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy@sha256:sha256:1942b0309b8978f20cdd55875a363b0c16a52646745db6deb0c47546343ad2af'
