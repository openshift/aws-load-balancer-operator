#!/bin/bash
set -euo pipefail

UPSTREAM="https://raw.githubusercontent.com/openshift/aws-load-balancer-controller/main/config/rbac"
INGRESS_CLASS_PARAMS_EDITOR_ROLE="ingressclassparams_editor_role.yaml"
CONTROLLER_ROLE="role.yaml"
CONTROLLER_ROLE_BINDING="role_binding.yaml"
TARGET_GROUP_BINDING_EDITOR_ROLE="targetgroupbinding_editor_role.yaml"

## Output ENV
YQ_BIN=./bin/yq
OUTPUT_DIR=config/rbac
OUTPUT_PREFIX=upstream_
OUTPUT_TMP=merged.yaml

if ! [ -f $YQ_BIN ]; then
    curl -L https://github.com/mikefarah/yq/releases/download/v4.22.1/yq_linux_amd64 -o $YQ_BIN && chmod +x $YQ_BIN
fi

curl --silent --output "$OUTPUT_DIR/$OUTPUT_PREFIX$INGRESS_CLASS_PARAMS_EDITOR_ROLE" "$UPSTREAM/$INGRESS_CLASS_PARAMS_EDITOR_ROLE"
curl --silent --output "$OUTPUT_DIR/$OUTPUT_PREFIX$CONTROLLER_ROLE" "$UPSTREAM/$CONTROLLER_ROLE"
curl --silent --output "$OUTPUT_DIR/$OUTPUT_PREFIX$CONTROLLER_ROLE_BINDING" "$UPSTREAM/$CONTROLLER_ROLE_BINDING"
curl --silent --output "$OUTPUT_DIR/$OUTPUT_PREFIX$TARGET_GROUP_BINDING_EDITOR_ROLE" "$UPSTREAM/$TARGET_GROUP_BINDING_EDITOR_ROLE"

$YQ_BIN -i '.subjects[0].name = "controller-manager"' "$OUTPUT_DIR/$OUTPUT_PREFIX$CONTROLLER_ROLE_BINDING"
$YQ_BIN ea '. as $item ireduce ({}; . *+ $item)' "$OUTPUT_DIR/$OUTPUT_PREFIX$INGRESS_CLASS_PARAMS_EDITOR_ROLE" "$OUTPUT_DIR/$OUTPUT_PREFIX$CONTROLLER_ROLE" > "$OUTPUT_DIR/$OUTPUT_TMP"
$YQ_BIN ea '. as $item ireduce ({}; . *+ $item)' "$OUTPUT_DIR/$OUTPUT_PREFIX$TARGET_GROUP_BINDING_EDITOR_ROLE" "$OUTPUT_DIR/$OUTPUT_TMP" > "$OUTPUT_DIR/$OUTPUT_PREFIX$CONTROLLER_ROLE"

rm -rf "$OUTPUT_DIR/$OUTPUT_TMP"
rm -rf "$OUTPUT_DIR/$OUTPUT_PREFIX$INGRESS_CLASS_PARAMS_EDITOR_ROLE"
rm -rf "$OUTPUT_DIR/$OUTPUT_PREFIX$TARGET_GROUP_BINDING_EDITOR_ROLE"
