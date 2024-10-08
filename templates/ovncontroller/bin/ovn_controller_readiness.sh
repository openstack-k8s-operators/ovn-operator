#!/bin/bash

set -e

error_exit() {
    echo "$1" >&2
    exit 1
}

# Check if ovn-controller is connected to the OVN SB database
check_ovn_controller_connection() {
    if ! output=$(ovn-appctl -t ovn-controller connection-status 2>&1); then
        error_exit "ERROR - Failed to get connection status from ovn-controller, ovn-appctl exit status: $?"
    fi

    if [ "$output" != "connected" ]; then
        error_exit "ERROR - ovn-controller connection status is '$output', expecting 'connected' status"
    fi
}


check_ovn_controller_connection
