#!/bin/bash
set -euo pipefail

curl -L https://github.com/mikefarah/yq/releases/download/v4.22.1/yq_linux_amd64 -o /tmp/yq && chmod +x /tmp/yq

UPSTREAM="https://raw.githubusercontent.com/openshift/aws-load-balancer-controller/main/config/rbac"
CONTROLLER_ROLE="role.yaml"
CONTROLLER_ROLE_BINDING="role_binding.yaml"
OUTPUT_DIR=config/rbac
OUTPUT_PREFIX=upstream_

curl --silent --output "$OUTPUT_DIR/$OUTPUT_PREFIX$CONTROLLER_ROLE" "$UPSTREAM/$CONTROLLER_ROLE"
curl --silent --output "$OUTPUT_DIR/$OUTPUT_PREFIX$CONTROLLER_ROLE_BINDING" "$UPSTREAM/$CONTROLLER_ROLE_BINDING"

/tmp/yq -i '.subjects[0].name = "controller-manager"' "$OUTPUT_DIR/$OUTPUT_PREFIX$CONTROLLER_ROLE_BINDING"

