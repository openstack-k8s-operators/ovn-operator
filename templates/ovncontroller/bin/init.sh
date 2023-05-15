#!/bin//bash
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

# Configs are obtained from ENV variables.
OvnBridge=${OvnBridge:-"br-int"}
OvnRemote=${OvnRemote:-"tcp:127.0.0.1:6642"}
OvnEncapType=${OvnEncapType:-"geneve"}
OvnEncapNIC=${OvnEncapNIC:="eth0"}
EnableChassisAsGateway=${EnableChassisAsGateway:-false}
PhysicalNetworks=${PhysicalNetworks:-""}
OvnHostName=${OvnHostName:-""}
OvnEncapIP=$(ip -o addr show dev ${OvnEncapNIC} scope global | awk '{print $4}' | cut -d/ -f1)

function wait_for_ovsdb_server {
    while true; do
        /usr/bin/ovs-vsctl show
        if [ $? -eq 0 ]; then
            break
        else
            echo "Ovsdb-server seems not be ready yet. Waiting..."
            sleep 1
        fi
    done
}

# configure external-ids in OVS
function configure_external_ids {
    ovs-vsctl set open . external-ids:ovn-bridge=${OvnBridge}
    ovs-vsctl set open . external-ids:ovn-remote=${OvnRemote}
    ovs-vsctl set open . external-ids:ovn-encap-type=${OvnEncapType}
    ovs-vsctl set open . external-ids:ovn-encap-ip=${OvnEncapIP}
    if [ -n "$OvnHostName" ]; then
        ovs-vsctl set open . external-ids:hostname=${OvnHostName}
    fi
    if [ "$EnableChassisAsGateway" == "true" ]; then
        ovs-vsctl set open . external-ids:ovn-cms-options=enable-chassis-as-gw
    fi
}

# Configure bridge mappings and physical bridges
function configure_physical_networks {
    local OvnBridgeMappings=""
    local net_number=1
    for physicalNetwork in ${PhysicalNetworks}; do
        br_name="br-${physicalNetwork}"
        ovs-vsctl --may-exist add-br ${br_name}
        ovs-vsctl --may-exist add-port ${br_name} net${net_number}
        net_number=$(( net_number+1 ))
        bridgeMapping="${physicalNetwork}:${br_name}"
        if [ -z "$OvnBridgeMappings"]; then
            OvnBridgeMappings=$bridgeMapping
        else
            OvnBridgeMappings="${OvnBridgeMappings},${bridgeMapping}"
        fi
    done
    if [ -n "$OvnBridgeMappings" ]; then
        ovs-vsctl set open . external-ids:ovn-bridge-mappings=${OvnBridgeMappings}
    fi
}


wait_for_ovsdb_server

# From now on, we should exit immediatelly when any command exits with non-zero status
set -ex

configure_external_ids
configure_physical_networks
