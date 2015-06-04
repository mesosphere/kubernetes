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

## Contains configuration values for interacting with the Vagrant cluster


KUBE_MASTER="docker-compose" # config context passed to kubectrl
#MASTER_IP="$(boot2docker ip):8888"
#KUBE_MASTER_IP=${MASTER_IP}
#KUBE_SERVER="http://${KUBE_MASTER_IP}"
#KUBERNETES_MASTER="http://${KUBE_MASTER_IP}"

NUM_MINIONS=${NUM_MINIONS:-2}
INSTANCE_PREFIX="${INSTANCE_PREFIX:-kubernetes}"
MASTER_NAME="${INSTANCE_PREFIX}-master"
MINION_NAMES=($(eval echo ${INSTANCE_PREFIX}-minion-{1..${NUM_MINIONS}}))

#KUBE_MINION_IP_ADDRESSES=()

#PORTAL_NET=10.10.10.0/24
SERVICE_CLUSTER_IP_RANGE=10.10.10.0/24  # formerly PORTAL_NET

# Admission Controllers to invoke prior to persisting objects in cluster
#ADMISSION_CONTROL=NamespaceLifecycle,NamespaceAutoProvision,LimitRanger,SecurityContextDeny,ServiceAccount,ResourceQuota

# Optional: Install node monitoring.
ENABLE_NODE_MONITORING=false

# Optional: Enable node logging.
#ENABLE_NODE_LOGGING=false
#LOGGING_DESTINATION=elasticsearch

# Optional: When set to true, Elasticsearch and Kibana will be setup as part of the cluster bring up.
#ENABLE_CLUSTER_LOGGING=false
#ELASTICSEARCH_LOGGING_REPLICAS=1

# Optional: When set to true, heapster, Influxdb and Grafana will be setup as part of the cluster bring up.
#ENABLE_CLUSTER_MONITORING="${KUBE_ENABLE_CLUSTER_MONITORING:-true}"

# Extra options to set on the Docker command line.  This is useful for setting
# --insecure-registry for local registries.
DOCKER_OPTS=""

# Optional: Install cluster DNS.
ENABLE_CLUSTER_DNS=false
#DNS_SERVER_IP="10.247.0.10"
#DNS_DOMAIN="cluster.local"
#DNS_REPLICAS=1

# Optional: Enable setting flags for kube-apiserver to turn on behavior in active-dev
RUNTIME_CONFIG=""
#RUNTIME_CONFIG="api/v1beta3"
