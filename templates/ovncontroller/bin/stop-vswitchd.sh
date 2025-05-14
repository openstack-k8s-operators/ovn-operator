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

# If file is present, skip stop script
if [ -f $skip_vswitchd_stop_file ]; then
    rm $skip_vswitchd_stop_file
    exit 0
fi

# Clean up any previously created flow backups to avoid conflict with newly
# generated backup.
cleanup_flows_backup

# Passing --real to mimic what upstream startup scripts do; maybe redundant.
bridges=$(ovs-vsctl -- --real list-br)

# Saving flows to avoid disrupting gateway datapath.
mkdir $FLOWS_RESTORE_DIR
TMPDIR=$FLOWS_RESTORE_DIR /usr/share/openvswitch/scripts/ovs-save save-flows $bridges > $FLOWS_RESTORE_SCRIPT

# Once save-flows logic is complete it no longer needs ovsdb-server, this file
# unlocks the db preStop script, working as a semaphore
touch $SAFE_TO_STOP_OVSDB_SERVER_SEMAPHORE

# Finally, stop vswitchd.
/usr/share/openvswitch/scripts/ovs-ctl stop --no-ovsdb-server
