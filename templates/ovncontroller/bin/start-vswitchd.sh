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

# Check which connection is used
if [ {{ .TLS }} == "Enabled" ]; then
    # TLS is used
    TLSOptions="--certificate=/etc/pki/tls/certs/ovndb.crt --private-key=/etc/pki/tls/private/ovndb.key --ca-cert=/etc/pki/tls/certs/ovndbca.crt"
    DBOptions="--db ssl:ovsdbserver-nb.openstack.svc.cluster.local:6641"
else
    # Normal TCP is used
    TLSOptions=""
    DBOptions="--db tcp:ovsdbserver-nb.openstack.svc.cluster.local:6641"
fi


# Check if we're doing an update
echo "Check if we're doing an update"
if [ -f /var/lib/openvswitch/already_executed ]; then
    while true; do
        if [ $(cat /var/lib/openvswitch/already_executed) == "OVSDB_SERVER" ]; then
            break
        fi
        if [ $(cat /var/lib/openvswitch/already_executed) == "INIT" ]; then
            break
        fi
        sleep 0.1
    done
    echo "OVSDBSERVER Already up, start script"
fi

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
/usr/sbin/ovs-vswitchd --pidfile --overwrite-pidfile --mlockall --detach

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

# Restore the priority if this was changed during the update
if [ -f /var/lib/openvswitch/old_priority ]; then
    echo "Using DBOptions: $DBOptions"
    echo "Using TLSOptions: $TLSOptions"
    priority=$(cat /var/lib/openvswitch/old_priority)
    echo "Restoring old priority, which was: $priority"
    chassis_id=$(ovs-vsctl get Open_Vswitch . external_ids:system-id)
    nb_output=$(ovn-nbctl --no-leader-only $DBOptions $TLSOptions --columns=_uuid,priority find Gateway_Chassis chassis_name=$chassis_id)
    err=$?
    if [ $err -ne 0 ]; then
        echo "Error while getting gateway chassis uuid $err"
    fi
    row_uuid=$(echo "$nb_output" | grep "_uuid" | cut -d':' -f2 | xargs)
    rm /var/lib/openvswitch/old_priority
    ovn-nbctl --no-leader-only $DBOptions $TLSOptions set Gateway_Chassis $row_uuid priority=$priority
    err=$?
    if [ $err -ne 0 ]; then
        echo "Error while setting gateway chassis priority ($priority), error: $err"
    fi
fi

# Set state to "RUNNING"
echo "RUNNING" > /var/lib/openvswitch/already_executed

# This is container command script. Block it from exiting, otherwise k8s will
# restart the container again.
sleep infinity
