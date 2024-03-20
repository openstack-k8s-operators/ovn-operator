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

DB_NAME="OVN_Northbound"
DB_TYPE="{{ .DB_TYPE }}"
if [[ "${DB_TYPE}" == "sb" ]]; then
    DB_NAME="OVN_Southbound"
fi
if [[ "$(hostname)" != "{{ .SERVICE_NAME }}-0" ]]; then
    ovs-appctl -t /tmp/ovn${DB_TYPE}_db.ctl cluster/leave ${DB_NAME}

    # wait for when the leader confirms we left the cluster
    while true; do
        # TODO: is there a better way to detect the cluster left state?..
        STATUS=$(ovs-appctl -t /tmp/ovn${DB_TYPE}_db.ctl cluster/status ${DB_NAME} | grep Status: | awk -e '{print $2}')
        if [ -z "$STATUS" -o "x$STATUS" = "xleft cluster" ]; then
            break
        fi
        sleep 1
    done

    # now that we left, the database file is no longer valid
    rm -f /etc/ovn/ovn${DB_TYPE}_db.db
fi
