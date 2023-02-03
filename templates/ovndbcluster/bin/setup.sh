#!/bin//bash
#
# Copyright 2022 Red Hat Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License"); you may
# not use this file except in compliance with the License. You may obtain
# a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
# WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
# License for the specific language governing permissions and limitations
# under the License.
set -ex
DB_TYPE="{{ .DB_TYPE }}"
DB_PORT="{{ .DB_PORT }}"
RAFT_PORT="{{ .RAFT_PORT }}"
OPTS=""
DB_NAME="OVN_Northbound"
if [[ "${DB_TYPE}" == "sb" ]]; then
    DB_NAME="OVN_Southbound"
fi
if [[ "$(hostname)" != "{{ .SERVICE_NAME }}-0" ]]; then
    rm -f /etc/ovn/ovn${DB_TYPE}_db.db
    #ovsdb-tool join-cluster /etc/ovn/ovn${DB_TYPE}_db.db ${DB_NAME} tcp:$(hostname).{{ .SERVICE_NAME }}.openstack.svc.cluster.local:${RAFT_PORT} tcp:{{ .SERVICE_NAME }}-0.{{ .SERVICE_NAME }}.openstack.svc.cluster.local:${RAFT_PORT}
    OPTS="--db-${DB_TYPE}-cluster-remote-proto=tcp --db-${DB_TYPE}-cluster-remote-addr={{ .SERVICE_NAME }}-0.{{ .SERVICE_NAME }}.openstack.svc.cluster.local --db-${DB_TYPE}-cluster-remote-port=${RAFT_PORT}"
fi
/usr/local/bin/start-${DB_TYPE}-db-server --db-${DB_TYPE}-create-insecure-remote=yes --db-${DB_TYPE}-election-timer={{ .OVN_ELECTION_TIMER }} --db-${DB_TYPE}-cluster-local-proto=tcp \
--db-${DB_TYPE}-cluster-local-addr=$(hostname).{{ .SERVICE_NAME }}.openstack.svc.cluster.local --db-${DB_TYPE}-probe-interval-to-active={{ .OVN_PROBE_INTERVAL_TO_ACTIVE }} \
--db-${DB_TYPE}-cluster-local-port=${RAFT_PORT} --db-${DB_TYPE}-addr=$(hostname).{{ .SERVICE_NAME }}.openstack.svc.cluster.local --db-${DB_TYPE}-port=${DB_PORT} \
--ovn-${DB_TYPE}-log=-vfile:{{ .OVN_LOG_LEVEL }} ${OPTS}
