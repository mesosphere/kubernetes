#!/bin/bash

# Copyright 2015 The Kubernetes Authors All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Deploy the addon services after the cluster is available
# TODO: integrate with or use /cluster/saltbase/salt/kube-addons/kube-addons.sh
# Requires:
#   ENABLE_CLUSTER_DNS (Optional) - 'Y' to deploy kube-dns
#   KUBE_SERVER (Optional) - url to the api server for configuring kube-dns


set -o errexit
set -o nounset
set -o pipefail

KUBE_ROOT=$(cd "$(dirname "${BASH_SOURCE}")/../../.." && pwd)
source "${KUBE_ROOT}/cluster/${KUBERNETES_PROVIDER}/${KUBE_CONFIG_FILE-"config-default.sh"}"
kubectl="${KUBE_ROOT}/cluster/kubectl.sh"

function base64_nowrap {
  input="$1"
  # detect base64 version
  if [ "$(echo test | base64 -b0 2>&1 || echo failed)" = "dGVzdAo=" ]; then
    # BSD base64 (mac)
    echo "${input}" | base64 -b0
  else
    # GNU base64 (linux)
    echo "${input}" | base64 -w0
  fi
}

function run_in_temp_dir {
  cmd="$1"
  prefix="$2"

  # create temp WORKSPACE dir in KUBE_ROOT to avoid permission issues of TMPDIR on mac os x
  local -r WORKSPACE=$(env TMPDIR=$PWD mktemp -d -t "${prefix}-XXXXXX")
  echo "Workspace created: ${WORKSPACE}" 1>&2

  cleanup() {
    rm -rf "${WORKSPACE}"
    echo "Workspace deleted: ${WORKSPACE}" 1>&2
  }
  trap 'cleanup' EXIT

  (${cmd}) || exit $?

  trap - EXIT
  cleanup
}

function deploy_dns {
  echo "Deploying DNS Addon" 1>&2

  # Process skydns-rc.yaml
  DNS_REPLICAS=${DNS_REPLICAS} DNS_DOMAIN=${DNS_DOMAIN} KUBE_SERVER=${KUBE_SERVER} \
      "${KUBE_ROOT}/cluster/addons/dns/skydns-rc.yaml.sh" > "${WORKSPACE}/skydns-rc.yaml"

  # Process skydns-svc.yaml
  DNS_SERVER_IP=${DNS_SERVER_IP} \
      "${KUBE_ROOT}/cluster/addons/dns/skydns-svc.yaml.sh" > "${WORKSPACE}/skydns-svc.yaml"

  # Process config secrets (ssl certs) into a mounted Secret volume
  local -r kubeconfig=$("${kubectl}" config view --raw)
  local -r kubeconfig_base64=$(base64_nowrap "${kubeconfig}")
  cat > "${WORKSPACE}/skydns-secret.yaml" <<EOF
apiVersion: v1
data:
  kubeconfig: ${kubeconfig_base64}
kind: Secret
metadata:
  name: token-system-dns
  namespace: default
type: Opaque
EOF

  # Use kubectl to create skydns rc and service
  "${kubectl}" create -f "${WORKSPACE}/skydns-secret.yaml"
  "${kubectl}" create -f "${WORKSPACE}/skydns-rc.yaml"
  "${kubectl}" create -f "${WORKSPACE}/skydns-svc.yaml"
}

function deploy_ui {
  echo "Deploying UI Addon" 1>&2

  # Use kubectl to create ui rc and service
  "${kubectl}" create -f "${KUBE_ROOT}/cluster/addons/kube-ui/kube-ui-rc.yaml"
  "${kubectl}" create -f "${KUBE_ROOT}/cluster/addons/kube-ui/kube-ui-svc.yaml"
}

if [ "${ENABLE_CLUSTER_DNS}" == true ]; then
  run_in_temp_dir 'deploy_dns' 'k8sm-dns'
fi

if [ "${ENABLE_CLUSTER_UI}" == true ]; then
  deploy_ui
fi
