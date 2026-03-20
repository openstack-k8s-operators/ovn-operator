#!/usr/bin/env bash
#
# Copyright 2026 Red Hat Inc.
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

set -e
source $(dirname $0)/functions

error_exit() {
    echo "$1" >&2
    exit 1
}

# Set socket path based on database type from functions
SOCKET_PATH="/tmp/ovn${DB_TYPE}_db.ctl"

# Set database name based on type
DB_NAME="OVN_Northbound"
if [[ "${DB_TYPE}" == "sb" ]]; then
    DB_NAME="OVN_Southbound"
fi

# Check if ovsdb-server process is running first
check_ovsdb_server_process() {
    if ! pidof ovsdb-server > /dev/null 2>&1; then
        error_exit "ERROR - ovsdb-server process is not running"
    fi
}

# Check ovsdb-server cluster status for RAFT
check_ovsdb_server_cluster_status() {
    if ! output=$(ovs-appctl -t "$SOCKET_PATH" cluster/status "$DB_NAME" 2>&1); then
        error_exit "ERROR - Failed to get cluster status from ovsdb-server: $output"
    fi

    # Extract just the Status field for logging
    status_line=$(echo "$output" | grep "Status:" | head -1)

    # Check if the server is a cluster member (this is the key check for RAFT readiness)
    if echo "$output" | grep -q "Status: cluster member"; then
        echo "INFO - ovsdb-server is cluster member and ready ($status_line)"
        return 0
    else
        error_exit "ERROR - ovsdb-server not ready as cluster member. $status_line"
    fi
}

echo "INFO - Checking OVN database cluster readiness for ${DB_TYPE} database"
check_ovsdb_server_process
check_ovsdb_server_cluster_status
echo "INFO - OVN database cluster readiness check passed for ${DB_TYPE}"
