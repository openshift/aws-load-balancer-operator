#!/bin/bash
set -euo pipefail

UPSTREAM="https://raw.githubusercontent.com/openshift/aws-load-balancer-controller/main/config/crd/bases"
INGRESS_CLASS_PARAMS="elbv2.k8s.aws_ingressclassparams.yaml"
TARGET_GROUP_BINDINGS="elbv2.k8s.aws_targetgroupbindings.yaml"
OUTPUT_DIR=config/crd/bases

curl --silent --output "$OUTPUT_DIR/$INGRESS_CLASS_PARAMS" "$UPSTREAM/$INGRESS_CLASS_PARAMS"
curl --silent --output "$OUTPUT_DIR/$TARGET_GROUP_BINDINGS" "$UPSTREAM/$TARGET_GROUP_BINDINGS"
