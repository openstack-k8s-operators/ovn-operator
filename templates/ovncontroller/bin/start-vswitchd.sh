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

# Execute flow restoring is on a separate script to allow for ovs-vswitchd to
# log on pod console.
$START_VSWITCHD_EXTRAS_SCRIPT &> $EXTRAS_LOG &

# It's safe to start vswitchd now. Do it.
/usr/sbin/ovs-vswitchd --pidfile --mlockall

