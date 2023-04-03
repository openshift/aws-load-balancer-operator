#!/usr/bin/env bash

set -e
set -u

usage() {
  cat <<EOF
Generate the service serving certificates and add the CA bundle to the CRD conversion webhook's client config.
Usage: ${0} [OPTIONS]
The following flags are required.
    --namespace        Namespace where webhook server resides.
    --service          Service name of webhook server.
    --secret           Secret name for CA certificate and server certificate/key pair.
    --crd              CRD name to be injected with CA.
EOF
  exit 1
}

while [ $# -gt 0 ]; do
  case ${1} in
      --service)
          service="$2"
          shift
          ;;
      --crd)
          crd="$2"
          shift
          ;;
      --secret)
          secret="$2"
          shift
          ;;
      --namespace)
          namespace="$2"
          shift
          ;;
      *)
          usage
          ;;
  esac
  shift
done

[ -z "${service:-}" ] && { echo "ERROR: --service flag is required"; exit 1; }
[ -z "${secret:-}" ] && { echo "ERROR: --secret flag is required"; exit 1; }
[ -z "${namespace:-}" ] && { echo "ERROR: --namespace flag is required"; exit 1; }
[ -z "${crd:-}" ] && { echo "ERROR: --crd flag is required"; exit 1; }
[ ! -x "$(command -v oc)" ] && { echo "ERROR: oc not found"; exit 1; }

oc -n "${namespace}" annotate service "${service}" "service.beta.openshift.io/serving-cert-secret-name=${secret}" --overwrite=true
oc annotate crd "${crd}" "service.beta.openshift.io/inject-cabundle=true" --overwrite=true
