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
    while [ ! -S /tmp/ovn${DB_TYPE}_db.ctl ]; do
        echo DB Server Not ready, waiting
        sleep 1
    done

    PODNAME=$(hostname -f | cut -d. -f1,2)
    PODIPV6=$(grep "${PODNAME}" /etc/hosts | grep ':' | cut -d$'\t' -f1)

    if [[ "" = "${PODIPV6}" ]]; then
        DB_ADDR="0.0.0.0"
    else
        DB_ADDR="[::]"
    fi

{{- if .TLS }}
    ovn-${DB_TYPE}ctl --no-leader-only set-ssl /etc/pki/tls/private/ovndb.key /etc/pki/tls/certs/ovndb.crt /etc/pki/tls/certs/ovndbca.crt
    ovn-${DB_TYPE}ctl --no-leader-only set-connection ${DB_SCHEME}:${DB_PORT}:0.0.0.0
{{- end }}

    while [ "$(ovn-${DB_TYPE}ctl --no-leader-only get connection . inactivity_probe)" != "{{ .OVN_INACTIVITY_PROBE }}" ]; do
        ovn-${DB_TYPE}ctl --no-leader-only --inactivity-probe={{ .OVN_INACTIVITY_PROBE }} set-connection ${DB_SCHEME}:${DB_PORT}:${DB_ADDR}
    done
    ovn-${DB_TYPE}ctl --no-leader-only list connection
fi
