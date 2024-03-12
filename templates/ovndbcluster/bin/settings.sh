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
{{- if .TLS }}
DB_SCHEME="pssl"
{{- else }}
DB_SCHEME="ptcp"
{{- end }}

exec 1>/proc/1/fd/1 2>&1

if [[ "$(hostname)" == "{{ .SERVICE_NAME }}-0" ]]; then
    # The command will wait until the daemon is connected and the DB is available
    # All following ctl invocation will use the local DB replica in the daemon
    export OVN_${DB_TYPE^^}_DAEMON=$(ovn-${DB_TYPE}ctl --detach)
    daemon_var=OVN_${DB_TYPE^^}_DAEMON

    PODNAME=$(hostname -f | cut -d. -f1,2)
    PODIPV6=$(grep "${PODNAME}" /etc/hosts | grep ':' | cut -d$'\t' -f1)

    if [[ "" = "${PODIPV6}" ]]; then
        DB_ADDR="0.0.0.0"
    else
        DB_ADDR="[::]"
    fi

{{- if .TLS }}
    ovn-${DB_TYPE}ctl --no-leader-only set-ssl {{.OVNDB_KEY_PATH}} {{.OVNDB_CERT_PATH}} {{.OVNDB_CACERT_PATH}}
    ovn-${DB_TYPE}ctl --no-leader-only set-connection ${DB_SCHEME}:${DB_PORT}:0.0.0.0
{{- end }}

    while [ "$(ovn-${DB_TYPE}ctl --no-leader-only get connection . inactivity_probe)" != "{{ .OVN_INACTIVITY_PROBE }}" ]; do
        ovn-${DB_TYPE}ctl --no-leader-only --inactivity-probe={{ .OVN_INACTIVITY_PROBE }} set-connection ${DB_SCHEME}:${DB_PORT}:${DB_ADDR}
    done
    ovn-${DB_TYPE}ctl --no-leader-only list connection

    # The daemon is no longer needed, kill it
    kill $(echo ${!daemon_var} | sed "s/.*ovn-${DB_TYPE}ctl\.\([0-9]*\).*/\1/")
fi
