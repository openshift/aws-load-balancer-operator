#!/bin/bash
set -euo pipefail

UPSTREAM="https://raw.githubusercontent.com/openshift/aws-load-balancer-controller/main/config/rbac"
CONTROLLER_ROLE="role.yaml"
CONTROLLER_ROLE_BINDING="role_binding.yaml"

## Output ENV
YQ_BIN="go run github.com/mikefarah/yq/v4"
OUTPUT_DIR=config/rbac
OUTPUT_PREFIX=upstream_

curl --silent --output "$OUTPUT_DIR/$OUTPUT_PREFIX$CONTROLLER_ROLE" "$UPSTREAM/$CONTROLLER_ROLE"
curl --silent --output "$OUTPUT_DIR/$OUTPUT_PREFIX$CONTROLLER_ROLE_BINDING" "$UPSTREAM/$CONTROLLER_ROLE_BINDING"

$YQ_BIN -i '.subjects[0].name = "controller-manager"' "$OUTPUT_DIR/$OUTPUT_PREFIX$CONTROLLER_ROLE_BINDING"
