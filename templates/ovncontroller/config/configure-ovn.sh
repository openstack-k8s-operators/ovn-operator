#!/bin/bash
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

# This script configures ovn-encap-tos setting in OVS external-ids
# It is used when ovn-encap-tos is explicitly set to a non-default value or
# when OVN or OVS log level values change.

source $(dirname $0)/../container-scripts/functions

OVNEncapTos={{.OVNEncapTos}}
OVSLogLevel={{.OVSLogLevel}}
OVNLogLevel={{.OVNLogLevel}}

function configure_ovn_external_ids {
    ovs-vsctl set open . external-ids:ovn-encap-tos=${OVNEncapTos}
}

function configure_log_level {
    for svc in ovsdb-server ovs-vswitchd; do
        ctl_path=$(find /var/run/openvswitch/ -maxdepth 1 -name "${svc}.*.ctl")
        ovs-appctl -t ${ctl_path} vlog/set ${OVSLogLevel}
    done

    ctl_path=$(find /var/run/ovn/ -maxdepth 1 -name "ovn-controller.*.ctl")
    ovn-appctl -t "$ctl_path" vlog/set ${OVNLogLevel}
}


wait_for_ovsdb_server
configure_ovn_external_ids
configure_log_level
