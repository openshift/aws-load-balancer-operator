#!/bin/bash
set -euo pipefail

ORG="openshift"
if [[ $# -gt 0 && "${1}" == "-k" ]]; then
    # sync from Kubernetes upstream
    # useful for rebase test PRs
    ORG="kubernetes-sigs"
fi
UPSTREAM="https://raw.githubusercontent.com/${ORG}/aws-load-balancer-controller/main/config/rbac"
CONTROLLER_ROLE="role.yaml"

## Output ENV
YQ_BIN="go run github.com/mikefarah/yq/v4"
OUTPUT_DIR=config/rbac
OUTPUT_PREFIX=upstream_

curl --output "$OUTPUT_DIR/$OUTPUT_PREFIX$CONTROLLER_ROLE" "$UPSTREAM/$CONTROLLER_ROLE"
