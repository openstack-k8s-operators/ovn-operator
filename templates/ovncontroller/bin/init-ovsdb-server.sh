#!/bin/sh
#
# Copyright 2023 Red Hat Inc.
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
trap wait_for_db_creation EXIT

# If db file is empty, remove it; otherwise service won't start.
# See https://issues.redhat.com/browse/FDP-689 for more details.
if ! [ -s ${DB_FILE} ]; then
    rm -f ${DB_FILE}
fi

# Check if it's a normal start or an update
# Normal start: ovsdb-server & ovs-vswitchd are not running, start normal
# Update: ovsdb-server & ovs-vswitchd still running, need different approach
if [ -f $ovs_vswitchd_pid_file ] || [ -f $ovsdb_server_pid_file ]; then
    # Some process it's running, it's an update. Create semaphore
    echo "UPDATE" > $update_semaphore_file
    # No need to initializice ovs-vswitchd in this path, as this has done before
    # TODO: check what happens if during the update an update to the ovs db is needed
else
    # In case something went wrong last run, ensure that semaphor_file is not present in this path
    if [ -f $update_semaphore_file ]; then
        rm $update_semaphore_file
    fi
    # Initialize or upgrade database if needed
    CTL_ARGS="--system-id=random --no-ovs-vswitchd"
    /usr/share/openvswitch/scripts/ovs-ctl start $CTL_ARGS
    /usr/share/openvswitch/scripts/ovs-ctl stop $CTL_ARGS

    wait_for_db_creation
    trap - EXIT
fi
