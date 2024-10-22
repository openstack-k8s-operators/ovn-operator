#!/bin/bash

set -e

error_exit() {
    echo "$1" >&2
    exit 1
}

# Check if ovn-controller is running
check_ovn_controller_pid() {
    if ! pidof -q ovn-controller; then
        error_exit "ERROR - ovn-controller is not running"
    fi
}

# Function to check the running status of ovn-controller
check_ovn_controller_status() {
    # Capture output and check exit status
    if ! output=$(ovn-appctl -t ovn-controller debug/status 2>&1); then
        error_exit "ERROR - Failed to get status from ovn-controller, ovn-appctl exit status: $?"
    fi

    if [ "$output" != "running" ]; then
        error_exit "ERROR - ovn-controller status is '$output', expecting 'running' status"
    fi
}


check_ovn_controller_pid
check_ovn_controller_status
