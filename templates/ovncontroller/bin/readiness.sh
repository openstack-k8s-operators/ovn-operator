#!/bin/sh

error_exit() {
    echo "$1"
    exit 1
}

# Check if ovn-controller is connected to the OVN SB database
check_ovn_controller_connection() {
    output=$(ovn-appctl -t ovn-controller connection-status)
    if [ $? -ne 0 ]; then
        error_exit "ERROR - Failed to get connection status from ovn-controller, ovn-appctl exit status: $?"
    fi
    if [ "$output" != "connected" ]; then
        error_exit "ERROR - ovn-controller connection status is '$output', expecting 'connected' status"
    fi
}

# Check if ovsdb-server is running
check_ovsdb_server() {
    ovsdbserverpidfile=/run/openvswitch/ovsdb-server.pid
    if [ ! -r $ovsdbserverpidfile ];  then
        error_exit "ERROR - ovsdb-server PID file does not exist or has wrong permissions"
    fi

    pid=$(cat "$ovsdbserverpidfile")
    if [ -z "$pid" ]; then
        error_exit "ERROR - Failed to read PID from ovsdb-server PID file"
    fi
}

# Check if ovs-vswitchd is running
check_ovs_vswitchd() {
    ovsvswitchdpidfile=/run/openvswitch/ovs-vswitchd.pid
    if [ ! -r $ovsvswitchdpidfile ]; then
        error_exit "ERROR - ovs-vswitchd PID file does not exist or has wrong permissions"
    fi

    pid=$(cat "$ovsvswitchdpidfile")
    if [ -z "$pid" ]; then
        error_exit "ERROR - Failed to read PID from ovs-vswitchd PID file"
    fi
}


check_ovn_controller_connection
check_ovsdb_server
check_ovs_vswitchd

exit 0
