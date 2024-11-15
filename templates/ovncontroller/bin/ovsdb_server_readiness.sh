#!/bin/bash

set -e

error_exit() {
    echo "$1" >&2
    exit 1
}

# Check ovsdb-server status
check_ovsdb_server_status() {

    if ! pid=$(cat /var/run/openvswitch/ovsdb-server.pid); then
        error_exit "ERROR - Failed to get pid for ovsdb-server, exit status: $?"
    fi

    if ! output=$(ovs-appctl -t /var/run/openvswitch/ovsdb-server."$pid".ctl ovsdb-server/list-dbs); then
        error_exit "ERROR - Failed retrieving list of databases from ovsdb-server"
    fi

    if [[ "$output" != *"Open_vSwitch"* ]]; then
        error_exit "ERROR - 'Open_vSwitch' not found on databases"
    fi
}

check_ovsdb_server_status
