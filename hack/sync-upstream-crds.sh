#!/bin/bash
set -euo pipefail

ORG="openshift"
if [[ $# -gt 0 && "${1}" == "-k" ]]; then
    # Sync with Kubernetes upstream to enable testing of unmerged controller rebase PRs.
    ORG="kubernetes-sigs"
fi
UPSTREAM="https://raw.githubusercontent.com/${ORG}/aws-load-balancer-controller/main/config/crd/bases"
INGRESS_CLASS_PARAMS="elbv2.k8s.aws_ingressclassparams.yaml"
TARGET_GROUP_BINDINGS="elbv2.k8s.aws_targetgroupbindings.yaml"
OUTPUT_DIR=config/crd/bases

curl --output "$OUTPUT_DIR/$INGRESS_CLASS_PARAMS" "$UPSTREAM/$INGRESS_CLASS_PARAMS"
curl --output "$OUTPUT_DIR/$TARGET_GROUP_BINDINGS" "$UPSTREAM/$TARGET_GROUP_BINDINGS"
