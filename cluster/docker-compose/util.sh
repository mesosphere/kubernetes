#!/bin/bash

# Copyright 2014 The Kubernetes Authors All rights reserved.
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

# Example: KUBERNETES_PROVIDER=docker-compose go run hack/e2e.go --v --test --check_version_skew=false

set -o errexit
set -o nounset
set -o pipefail

KUBE_ROOT=$(dirname "${BASH_SOURCE}")/../..
source "${KUBE_ROOT}/cluster/docker-compose/${KUBE_CONFIG_FILE-"config-default.sh"}"
source "${KUBE_ROOT}/cluster/common.sh"
source "${KUBE_ROOT}/cluster/kube-util.sh" #default no-op method impls

KUBE_PREPARED=false

# Generate kubeconfig data for the created cluster.
# Assumed vars:
#   KUBE_ROOT
#   KUBECONFIG or DEFAULT_KUBECONFIG
#   KUBE_MASTER_IP
#
# Vars set:
#   CONTEXT
#   KUBECONFIG
function create-kubeconfig {
  local kubectl="${KUBE_ROOT}/cluster/kubectl.sh"

  export CONTEXT="docker-compose"
  export KUBECONFIG=${KUBECONFIG:-$DEFAULT_KUBECONFIG}
  # KUBECONFIG determines the file we write to, but it may not exist yet
  if [[ ! -e "${KUBECONFIG}" ]]; then
    mkdir -p $(dirname "${KUBECONFIG}")
    touch "${KUBECONFIG}"
  fi
  "${kubectl}" config set-cluster "${CONTEXT}" --server="http://${KUBE_MASTER_IP}"
  "${kubectl}" config set-context "${CONTEXT}" --cluster="${CONTEXT}" --user="${CONTEXT}"
  "${kubectl}" config use-context "${CONTEXT}" --cluster="${CONTEXT}"

   echo "Wrote config for ${CONTEXT} to ${KUBECONFIG}" 1>&2
}

function is-pod-running {
  local pod_name="$1"
  local kubectl="${KUBE_ROOT}/cluster/kubectl.sh"
  phase=$("${kubectl}" get pod "${pod_name}" -o template --template="{{.status.phase}}" 2>&1)
#  echo "Pod '${pod_name}' status: ${phase}" 1>&2
  if [ "${phase}" != "Running" ]; then
    return 1
  fi
  return 0
}

# Create & Delete a test pod to ensure a Kublet is running (on a Mesos slave)
function init-kublets {
  local kubectl="${KUBE_ROOT}/cluster/kubectl.sh"
  local pod="${KUBE_ROOT}/cluster/docker-compose/pod-nginx.json"
  local pod_name=$(jq '.metadata.name' --raw-output < ${pod})
  #TODO: use gcr.io/google_containers/pause:0.8.0 "/pause" ?

  if is-pod-running "${pod_name}"; then
    echo "Test pod '${pod_name}' is already running." 1>&2
    return 0
  fi

  echo "Creating test pod '${pod_name}'" 1>&2
  "${kubectl}" create -f "${pod}" 1> /dev/null

  echo "Waiting for pod '${pod_name}' to come up..." 1>&2
  next_wait_time=0
  until is-pod-running "${pod_name}" || [ ${next_wait_time} -eq 10 ]; do
    [ ${next_wait_time} -gt 1 ] && echo "Sleeping ${next_wait_time} seconds..." 1>&2
    sleep $(( next_wait_time++ ))
  done

  if ! is-pod-running "${pod_name}"; then
    echo "ERROR: Test pod '${pod_name}' never came up!" 1>&2
    return 1
  fi

  echo "Pod '${pod_name}' is Running" 1>&2

  echo "Deleting test pod '${pod_name}'" 1>&2
  "${kubectl}" delete -f "${pod}" 1> /dev/null
}

# Perform preparations required to run e2e tests
function prepare-e2e {
  if [ "${KUBE_PREPARED}" = true ]; then
    echo "Skipping preparing e2e environment" 1>&2
    return 0
  fi

  echo "Preparing e2e environment" 1>&2
  detect-master
  detect-minions
  create-kubeconfig
  init-kublets

  KUBE_PREPARED=true
}

# Execute prior to running tests to build a release if required for env
function test-build-release {
  # Make a release
  "${KUBE_ROOT}/build/release.sh"
}

# Must ensure that the following ENV vars are set
function detect-master {
  echo "KUBE_MASTER: $KUBE_MASTER" 1>&2

  docker_id=$(docker ps --filter="name=docker_apiserver" --quiet)
  if [[ "${docker_id}" == *'\n'* ]]; then
    echo "ERROR: Multiple API Servers running in docker" 1>&2
    return 1
  fi

  details_json=$(docker inspect ${docker_id})
  master_ip=$(jq '.[0].NetworkSettings.IPAddress' --raw-output <<< "$details_json")
  master_port=$(jq '.[0].NetworkSettings.Ports[][].HostPort' --raw-output <<< "$details_json")

  KUBE_MASTER_IP="${master_ip}:${master_port}"
  KUBE_SERVER="http://${KUBE_MASTER_IP}"

  echo "KUBE_MASTER_IP: $KUBE_MASTER_IP" 1>&2
}

# Get minion IP addresses and store in KUBE_MINION_IP_ADDRESSES[]
# These Mesos slaves MAY host Kublets,
# but might not have a Kublet running unless a kubernetes task has been scheduled on them.
function detect-minions {
  docker_ids=$(docker ps --filter="name=docker_mesosslave" --quiet)
  while read -r docker_id; do

    details_json=$(docker inspect ${docker_id})
    minion_ip=$(jq '.[0].NetworkSettings.IPAddress' --raw-output <<< "$details_json")
    minion_port=$(jq '.[0].NetworkSettings.Ports[][].HostPort' --raw-output <<< "$details_json")
    #TODO: filter for 505* port

    KUBE_MINION_IP_ADDRESSES+=("${minion_ip}")
#    KUBE_MINION_IP_ADDRESSES+=("${minion_ip}:${minion_port}")

  done <<< "$docker_ids"
  echo "KUBE_MINION_IP_ADDRESSES: [${KUBE_MINION_IP_ADDRESSES[*]}]" 1>&2
}

## Below functions used by hack/e2e-suite/services.sh

# SSH to a node by name or IP ($1) and run a command ($2).
function ssh-to-node {
  echo "TODO: ssh-to-node" 1>&2
}

# Restart the kube-proxy on a node ($1)
function restart-kube-proxy {
  echo "TODO: restart-kube-proxy" 1>&2
}

# Restart the apiserver
function restart-apiserver {
  echo "TODO: restart-apiserver" 1>&2
}
