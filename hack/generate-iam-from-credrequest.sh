#! /bin/bash

set -euo pipefail

CR_FILE="${1:-operator-credentials-request.yaml}"
POLICY_FILE="${2:-operator-permission-policy.json}"
YQ_BIN="go run github.com/mikefarah/yq/v4"

STATEMENTS=$(${YQ_BIN} -o=json .spec.providerSpec.statementEntries "${CR_FILE}")

cat <<EOF > "${POLICY_FILE}"
{
 "Version": "2012-10-17",
 "Statement": ${STATEMENTS}
}
EOF
${YQ_BIN} -i -o=json "${POLICY_FILE}"
sed -i -e 's/action/Action/g' -e 's/effect/Effect/g' -e 's/resource/Resource/g' "${POLICY_FILE}"
