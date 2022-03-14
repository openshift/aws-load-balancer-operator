#!/bin/bash
set -euo pipefail

curl -L https://github.com/mikefarah/yq/releases/download/v4.22.1/yq_linux_amd64 -o /tmp/yq && chmod +x /tmp/yq

UPSTREAM="https://raw.githubusercontent.com/openshift/aws-load-balancer-controller/main/config/rbac"
INGRESS_CLASS_PARAMS_EDITOR_ROLE="ingressclassparams_editor_role.yaml"
CONTROLLER_ROLE="role.yaml"
CONTROLLER_ROLE_BINDING="role_binding.yaml"
TARGET_GROUP_BINDING_EDITOR_ROLE="targetgroupbinding_editor_role.yaml"
OUTPUT_DIR=config/rbac
OUTPUT_PREFIX=upstream_
OUTPUT_TMP=merged.yaml

curl --silent --output "$OUTPUT_DIR/$OUTPUT_PREFIX$INGRESS_CLASS_PARAMS_EDITOR_ROLE" "$UPSTREAM/$INGRESS_CLASS_PARAMS_EDITOR_ROLE"
curl --silent --output "$OUTPUT_DIR/$OUTPUT_PREFIX$CONTROLLER_ROLE" "$UPSTREAM/$CONTROLLER_ROLE"
curl --silent --output "$OUTPUT_DIR/$OUTPUT_PREFIX$CONTROLLER_ROLE_BINDING" "$UPSTREAM/$CONTROLLER_ROLE_BINDING"
curl --silent --output "$OUTPUT_DIR/$OUTPUT_PREFIX$TARGET_GROUP_BINDING_EDITOR_ROLE" "$UPSTREAM/$TARGET_GROUP_BINDING_EDITOR_ROLE"

/tmp/yq -i '.subjects[0].name = "controller-manager"' "$OUTPUT_DIR/$OUTPUT_PREFIX$CONTROLLER_ROLE_BINDING"
/tmp/yq ea '. as $item ireduce ({}; . *+ $item)' "$OUTPUT_DIR/$OUTPUT_PREFIX$INGRESS_CLASS_PARAMS_EDITOR_ROLE" "$OUTPUT_DIR/$OUTPUT_PREFIX$CONTROLLER_ROLE" > "$OUTPUT_DIR/$OUTPUT_TMP"
/tmp/yq ea '. as $item ireduce ({}; . *+ $item)' "$OUTPUT_DIR/$OUTPUT_PREFIX$TARGET_GROUP_BINDING_EDITOR_ROLE" "$OUTPUT_DIR/$OUTPUT_TMP" > "$OUTPUT_DIR/$OUTPUT_PREFIX$CONTROLLER_ROLE"

rm -rf "$OUTPUT_DIR/$OUTPUT_TMP"
rm -rf "$OUTPUT_DIR/$OUTPUT_PREFIX$INGRESS_CLASS_PARAMS_EDITOR_ROLE"
rm -rf "$OUTPUT_DIR/$OUTPUT_PREFIX$TARGET_GROUP_BINDING_EDITOR_ROLE"