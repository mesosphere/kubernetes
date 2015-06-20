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

set -o errexit
set -o nounset
set -o pipefail

KUBE_ROOT=$(cd "$(dirname "${BASH_SOURCE}")/../../.." && pwd)
source "${KUBE_ROOT}/cluster/${KUBERNETES_PROVIDER}/${KUBE_CONFIG_FILE-"config-default.sh"}"
kubectl="${KUBE_ROOT}/cluster/kubectl.sh"

function deploy_dns {
  echo "Deploying DNS Addon" 1>&2

  # create temp workspace to place compiled pod yamls
  # create temp workspace dir in KUBE_ROOT to avoid permission issues of TMPDIR on mac os x
  local -r workspace=$(env TMPDIR=$PWD mktemp -d -t "k8sm-dns-XXXXXX")
  echo "Workspace created: ${workspace}" 1>&2

  cleanup() {
    rm -rf "${workspace}"
    echo "Workspace deleted: ${workspace}" 1>&2
  }
  trap 'cleanup' EXIT

  # Process salt pillar templates manually
  sed -e "s/{{ pillar\['dns_replicas'\] }}/${DNS_REPLICAS}/g;s/{{ pillar\['dns_domain'\] }}/${DNS_DOMAIN}/g" "${KUBE_ROOT}/cluster/addons/dns/skydns-rc.yaml.in" > "${workspace}/skydns-rc.yaml"
  sed -e "s/{{ pillar\['dns_server'\] }}/${DNS_SERVER_IP}/g" "${KUBE_ROOT}/cluster/addons/dns/skydns-svc.yaml.in" > "${workspace}/skydns-svc.yaml"

  # Process config secrets (ssl certs) into a mounted Secret volume
  local -r kubeconfig_base64=$("${KUBE_ROOT}/cluster/kubectl.sh" config view --raw | base64 -w0)
  cat > "${workspace}/skydns-secret.yaml" <<EOF
apiVersion: v1beta3
data:
  kubeconfig: ${kubeconfig_base64}
kind: Secret
metadata:
  name: token-system-dns
  namespace: default
type: Opaque
EOF

  # Use kubectl to create skydns rc and service
  "${kubectl}" create -f "${workspace}/skydns-secret.yaml"
  "${kubectl}" create -f "${workspace}/skydns-rc.yaml"
  "${kubectl}" create -f "${workspace}/skydns-svc.yaml"

  trap - EXIT
  cleanup
}

if [ "${ENABLE_CLUSTER_DNS}" == true ]; then
  deploy_dns
fi
