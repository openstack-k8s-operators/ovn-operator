#!/bin/sh

error_exit() {
    echo "$1"
    exit 1
}

# Check if ovn-controller is running
check_ovn_controller_pid() {
    pidof -q ovn-controller
    if [ $? -ne 0 ]; then
        error_exit "ERROR - ovn-controller is not running, pidof command exit status: $?"
    fi
}

# Function to check the running status of ovn-controller
check_ovn_controller_status() {
    output=$(ovn-appctl -t ovn-controller debug/status)
    if [ $? -ne 0 ]; then
        error_exit "ERROR - Failed to get status from ovn-controller, ovn-appctl exit status: $?"
    fi
    if [ "$output" != "running" ]; then
        error_exit "ERROR - ovn-controller status is '$output' status, expecting 'running' status"
    fi
}


check_ovn_controller_pid
check_ovn_controller_status

exit 0
