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

## Prints a yaml file that defines a kube-dns replication controller
# Requires:
#   DNS_REPLICAS - how many dns pod replicas to maintain
#   DNS_DOMAIN - domain to use
#   KUBE_SERVER - url of the api server

set -o errexit
set -o nounset
set -o pipefail

cat << EOF
apiVersion: v1
kind: ReplicationController
metadata:
  name: kube-dns-v4
  namespace: default
  labels:
    k8s-app: kube-dns
    version: v4
    kubernetes.io/cluster-service: "true"
spec:
  replicas: ${DNS_REPLICAS}
  selector:
    k8s-app: kube-dns
    version: v4
  template:
    metadata:
      labels:
        k8s-app: kube-dns
        version: v4
        kubernetes.io/cluster-service: "true"
    spec:
      containers:
      - name: etcd
        image: gcr.io/google_containers/etcd:2.0.9
        command:
        - /usr/local/bin/etcd
        - -listen-client-urls
        - http://127.0.0.1:2379,http://127.0.0.1:4001
        - -advertise-client-urls
        - http://127.0.0.1:2379,http://127.0.0.1:4001
        - -initial-cluster-token
        - skydns-etcd
      - name: kube2sky
        image: gcr.io/google_containers/kube2sky:1.10
        args:
        # command = "/kube2sky"
        - -domain=${DNS_DOMAIN}
        - -kube_master_url=${KUBE_SERVER}
      - name: skydns
        image: gcr.io/google_containers/skydns:2015-03-11-001
        args:
        # command = "/skydns"
        - -machines=http://localhost:4001
        - -addr=0.0.0.0:53
        - -domain=${DNS_DOMAIN}.
        ports:
        - containerPort: 53
          name: dns
          protocol: UDP
        - containerPort: 53
          name: dns-tcp
          protocol: TCP
        livenessProbe:
          exec:
            command:
            - /bin/sh
            - -c
            - nslookup kubernetes.default.svc.${DNS_DOMAIN} localhost >/dev/null
          initialDelaySeconds: 30
          timeoutSeconds: 5
      dnsPolicy: Default  # Don't use cluster DNS.
EOF
