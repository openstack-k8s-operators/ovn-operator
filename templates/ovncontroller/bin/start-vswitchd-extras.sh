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

set -ex

if [ ! -f /var/run/openvswitch/ovs-vswitchd.pid ]; then
       sleep 1
fi

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
