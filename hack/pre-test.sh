#!/bin/bash
set -eo pipefail

# This script is run by the Tekton task to set up ALBO on an STS cluster.
# It creates the IAM role and installs the operator.

# 1. Define unique names and the policy
# A unique name for the role based on the CI build
ROLE_NAME="albo-test-role-${RANDOM}"
ROLE_POLICY_NAME="albo-test-policy-${RANDOM}"
NAMESPACE="aws-load-balancer-operator"
OPERATOR_NAME="aws-load-balancer-operator"

echo "Using Role Name: ${ROLE_NAME}"
echo "Installing in Namespace: ${NAMESPACE}"

# Get the cluster OIDC provider
# (This assumes the cluster is already an STS cluster from the workflow)
OIDC_PROVIDER=$(oc get authentication.config.openshift.io cluster -o jsonpath='{.spec.serviceAccountIssuer}' | sed 's|^https://||')
if [ -z "$OIDC_PROVIDER" ]; then
    echo "Failed to get OIDC Provider."
    exit 1
fi
echo "Using OIDC Provider: ${OIDC_PROVIDER}"

# Create the Trust Policy JSON
cat > /tmp/trust-policy.json <<EOF
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Principal": {
                "Federated": "arn:aws:iam::$(aws sts get-caller-identity --query Account --output text):oidc-provider/${OIDC_PROVIDER}"
            },
            "Action": "sts:AssumeRoleWithWebIdentity",
            "Condition": {
                "StringEquals": {
                    "${OIDC_PROVIDER}:sub": "system:serviceaccount:${NAMESPACE}:${OPERATOR_NAME}-controller-manager"
                }
            }
        }
    ]
}
EOF

# 2. Create the IAM Role
echo "Creating IAM Role..."
ROLE_ARN=$(aws iam create-role --role-name "${ROLE_NAME}" --assume-role-policy-document file:///tmp/trust-policy.json --query 'Role.Arn' --output text)
if [ -z "$ROLE_ARN" ]; then
    echo "Failed to create IAM role."
    exit 1
fi
echo "Successfully created IAM Role: ${ROLE_ARN}"

# 3. Create and Attach the IAM Policy
# (Using the minimal policy from the ALBO docs)
echo "Creating and attaching IAM Policy..."
cat > /tmp/iam-policy.json <<EOF
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "ec2:DescribeVpcs"
            ],
            "Resource": "*"
        }
    ]
}
EOF

aws iam create-policy --policy-name "${ROLE_POLICY_NAME}" --policy-document file:///tmp/iam-policy.json
aws iam attach-role-policy --role-name "${ROLE_NAME}" --policy-arn "arn:aws:iam::$(aws sts get-caller-identity --query Account --output text):policy/${ROLE_POLICY_NAME}"
echo "Policy attached."

# 4. Create Namespace and OperatorGroup
echo "Installing ALBO..."
oc create ns "${NAMESPACE}" || echo "Namespace ${NAMESPACE} already exists."

cat <<EOF | oc apply -f -
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: ${OPERATOR_NAME}-og
  namespace: ${NAMESPACE}
spec:
  targetNamespaces:
  - ${NAMESPACE}
EOF

# 5. Create the Subscription (Injecting the Role ARN)
cat <<EOF | oc apply -f -
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: ${OPERATOR_NAME}-sub
  namespace: ${NAMESPACE}
spec:
  channel: "stable"
  name: "aws-load-balancer-operator"
  source: "redhat-operators"
  sourceNamespace: "openshift-marketplace"
  config:
    env:
    - name: ROLEARN
      value: ${ROLE_ARN}
EOF

# 6. Wait for Operator to install
echo "Waiting for ALBO Operator to install..."
sleep 30 # Give OLM time to react
oc wait deployment -n "${NAMESPACE}" "${OPERATOR_NAME}-controller-manager" --for=condition=Available --timeout=5m

# 7. Create the AWSLoadBalancerController CR (Injecting the Role ARN again)
cat <<EOF | oc apply -f -
apiVersion: networking.olm.openshift.io/v1
kind: AWSLoadBalancerController
metadata:
  name: cluster
spec:
  credentialsRequestConfig:
    stsIAMRoleARN: ${ROLE_ARN}
EOF

echo "ALBO setup complete. Waiting for controller to settle..."
sleep 15 # Give the controller time to start and check the VPC
echo "Setup finished."
