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

wait_for_ovsdb_server

# From now on, we should exit immediatelly when any command exits with non-zero status
set -ex

# Set deterministic UUID5 system-id derived from hostname for RBAC compatibility.
# The system-id must match the certificate CN for OVN RBAC ownership checks.
if [ -n "${OVNHostName}" ]; then
    SYSTEM_ID=$(hostname_to_uuid "${OVNHostName}")
    ovs-vsctl set open . external-ids:system-id=${SYSTEM_ID}
fi

configure_external_ids
configure_physical_networks

# Generate per-node RBAC certificate if the RBAC PKI CA is available
if [ -n "${OVN_RBAC_CA_CERT}" ] && [ -f "${OVN_RBAC_CA_CERT}" ]; then
    generate_rbac_certificate "${SYSTEM_ID}"
fi
