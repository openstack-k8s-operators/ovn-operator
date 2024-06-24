#!/bin/sh
#
# Copyright 2024 Red Hat Inc.
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
source $(dirname $0)/functions

# The ovs_vswitchd container has to terminate before ovsdb-server because it
# needs access to db in its preStop script. The preStop script backs up flows
# for restoration during the next startup. This semaphore ensures the vswitchd
# container is not torn down before flows are saved.
while [ ! -f $SAFE_TO_STOP_OVSDB_SERVER_SEMAPHORE ]; do
    sleep 0.5
done
cleanup_ovsdb_server_semaphore

# Now it's safe to stop db server. Do it.
/usr/share/openvswitch/scripts/ovs-ctl stop --no-ovs-vswitchd
