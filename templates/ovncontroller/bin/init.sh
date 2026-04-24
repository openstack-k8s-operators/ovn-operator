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

source $(dirname $0)/functions

# Install RBAC client certificate if available (mounted from cert-manager Secret).
# This is a pure file copy with no OVS dependency, so do it before
# wait_for_ovsdb_server to unblock ovn-controller's wait_for_rbac_cert
# as early as possible.
if [ -f "/tmp/ovn-rbac-cert/tls.crt" ]; then
    mkdir -p /etc/openvswitch
    cp /tmp/ovn-rbac-cert/tls.crt /etc/openvswitch/ovn-controller-cert.pem
    cp /tmp/ovn-rbac-cert/tls.key /etc/openvswitch/ovn-controller-privkey.pem
    chmod 0644 /etc/openvswitch/ovn-controller-cert.pem
    chmod 0600 /etc/openvswitch/ovn-controller-privkey.pem
fi

wait_for_ovsdb_server

# From now on, we should exit immediatelly when any command exits with non-zero status
set -ex

# Set system-id if provided by the controller
if [ -n "${SYSTEM_ID}" ]; then
    ovs-vsctl set open . external-ids:system-id=${SYSTEM_ID}
fi

configure_external_ids
configure_physical_networks
