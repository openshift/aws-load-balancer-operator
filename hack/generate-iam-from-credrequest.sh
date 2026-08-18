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
python3 -c '
import json, sys
p = sys.argv[1]
data = json.load(open(p))
for stmt in data.get("Statement", []):
    if "action" in stmt: stmt["Action"] = stmt.pop("action")
    if "effect" in stmt: stmt["Effect"] = stmt.pop("effect")
    if "resource" in stmt: stmt["Resource"] = stmt.pop("resource")
with open(p, "w") as f:
    json.dump(data, f, indent=2)
    f.write("\n")
' "${POLICY_FILE}"
