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

source $(dirname $0)/functions
wait_for_ovsdb_server

# The order - first wait for db server, then set -ex - is important. Otherwise,
# wait_for_ovsdb_server interrim check would make the script exit.
set -ex

# Configure encap IP.
OVNEncapIP=$(ip -o addr show dev {{ .OVNEncapNIC }} scope global | awk '{print $4}' | cut -d/ -f1)
ovs-vsctl --no-wait set open . external-ids:ovn-encap-ip=${OVNEncapIP}

# Before starting vswitchd, block it from flushing existing datapath flows.
ovs-vsctl --no-wait set open_vswitch . other_config:flow-restore-wait=true

# It's safe to start vswitchd now. Do it.
# --detach to allow the execution to continue to restoring the flows.
# We need to use --log-file since --detach disables all logging explicitly.
# Once https://issues.redhat.com/browse/FDP-1292 is fixed we will be able to
# use system logs only.
/usr/sbin/ovs-vswitchd --pidfile --mlockall --detach --log-file

# Restore saved flows.
if [ -f $FLOWS_RESTORE_SCRIPT ]; then
    # It's unsafe to leave these files in place if they fail once. Make sure we
    # remove them if the eval fails.
    trap cleanup_flows_backup EXIT
    eval "$(cat $FLOWS_RESTORE_SCRIPT)"
    trap - EXIT
fi

# It's also unsafe to leave these files after flow-restore-wait flag is removed
# because the backup will become stale and if a container later crashes, it may
# mistakenly try to restore from this old backup.
cleanup_flows_backup

# Now, inform vswitchd that we are done.
ovs-vsctl remove open_vswitch . other_config flow-restore-wait

# This is container command script. Block it from exiting, otherwise k8s will
# restart the container again.
tail -f /var/log/openvswitch/ovs-vswitchd.log
