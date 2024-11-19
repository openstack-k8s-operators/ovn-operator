#!/bin/bash

set -e

error_exit() {
    echo "$1" >&2
    exit 1
}

# Check if ovsdb-server is running
check_ovsdb_server_pid() {
    if ! pidof -q ovsdb-server; then
        error_exit "ERROR - ovsdb-server is not running"
    fi
}

# Function to check the running status of ovsdb-server
check_ovsdb_server_status() {
    if ! /usr/bin/ovs-vsctl show 2>&1; then
        error_exit "ERROR - Failed to get output from ovsdb-server, ovs-vsctl exit status: $?"
    fi
}


check_ovsdb_server_pid
check_ovsdb_server_status
