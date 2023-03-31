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

exec 1>/proc/1/fd/1 2>&1

if [[ "$(hostname)" == "{{ .SERVICE_NAME }}-0" ]]; then
    while [ ! -S /tmp/ovn${DB_TYPE}_db.ctl ]; do
        echo DB Server Not ready, waiting
        sleep 1
    done

    while [ "$(ovn-${DB_TYPE}ctl --no-leader-only get connection . inactivity_probe)" != "{{ .OVN_INACTIVITY_PROBE }}" ]; do
        ovn-${DB_TYPE}ctl --no-leader-only --inactivity-probe={{ .OVN_INACTIVITY_PROBE }} set-connection ptcp:${DB_PORT}:0.0.0.0
    done
    ovn-${DB_TYPE}ctl --no-leader-only list connection
fi
