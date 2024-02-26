#!/usr/bin/env bash
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
NAMESPACE="{{ .NAMESPACE }}"
OPTS=""
DB_NAME="OVN_Northbound"
if [[ "${DB_TYPE}" == "sb" ]]; then
    DB_NAME="OVN_Southbound"
fi

PODNAME=$(hostname -f | cut -d. -f1,2)
PODIPV6=$(grep "${PODNAME}" /etc/hosts | grep ':' | cut -d$'\t' -f1)

if [[ "" = "${PODIPV6}" ]]; then
    DB_ADDR="0.0.0.0"
else
    DB_ADDR="[::]"
fi

if [[ "$(hostname)" != "{{ .SERVICE_NAME }}-0" ]]; then
    rm -f /etc/ovn/ovn${DB_TYPE}_db.db
    #ovsdb-tool join-cluster /etc/ovn/ovn${DB_TYPE}_db.db ${DB_NAME} tcp:$(hostname).{{ .SERVICE_NAME }}.${NAMESPACE}.svc.cluster.local:${RAFT_PORT} tcp:{{ .SERVICE_NAME }}-0.{{ .SERVICE_NAME }}.${NAMESPACE}.svc.cluster.local:${RAFT_PORT}
    OPTS="--db-${DB_TYPE}-cluster-remote-addr={{ .SERVICE_NAME }}-0.{{ .SERVICE_NAME }}.openstack.svc.cluster.local --db-${DB_TYPE}-cluster-remote-port=${RAFT_PORT} --db-${DB_TYPE}-addr=${DB_ADDR}"
fi


# call to ovn-ctl directly instead of start-${DB_TYPE}-db-server to pass
# extra_args after --
set /usr/share/ovn/scripts/ovn-ctl --no-monitor

set "$@" --db-${DB_TYPE}-election-timer={{ .OVN_ELECTION_TIMER }}
set "$@" --db-${DB_TYPE}-cluster-local-addr=$(hostname).{{ .SERVICE_NAME }}.${NAMESPACE}.svc.cluster.local
set "$@" --db-${DB_TYPE}-cluster-local-port=${RAFT_PORT}
set "$@" --db-${DB_TYPE}-probe-interval-to-active={{ .OVN_PROBE_INTERVAL_TO_ACTIVE }}
set "$@" --db-${DB_TYPE}-addr=${DB_ADDR}
set "$@" --db-${DB_TYPE}-port=${DB_PORT}
{{- if .TLS }}
set "$@" --ovn-${DB_TYPE}-db-ssl-key={{.OVNDB_KEY_PATH}}
set "$@" --ovn-${DB_TYPE}-db-ssl-cert={{.OVNDB_CERT_PATH}}
set "$@" --ovn-${DB_TYPE}-db-ssl-ca-cert={{.OVNDB_CACERT_PATH}}
set "$@" --db-${DB_TYPE}-cluster-local-proto=ssl
set "$@" --db-${DB_TYPE}-cluster-remote-proto=ssl
set "$@" --db-${DB_TYPE}-create-insecure-remote=no
{{- else }}
set "$@" --db-${DB_TYPE}-cluster-local-proto=tcp
set "$@" --db-${DB_TYPE}-cluster-remote-proto=tcp
{{- end }}

# log to console
set "$@" --ovn-${DB_TYPE}-log=-vconsole:{{ .OVN_LOG_LEVEL }}

# if server attempts to log to file, ignore
#
# note: even with -vfile:off (see below), the server sometimes attempts to
# create a log file -> this argument makes sure it doesn't polute OVN_LOGDIR
# with a nearly empty log file
set "$@" --ovn-${DB_TYPE}-logfile=/dev/null

# don't log to file (we already log to console)
$@ ${OPTS} run_${DB_TYPE}_ovsdb -- -vfile:off
