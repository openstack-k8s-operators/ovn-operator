#!/bin/bash

set -e

error_exit() {
    echo "$1" >&2
    exit 1
}

# Check ovs-vswitchd status
check_ovs_vswitchd_status() {

    if ! pid=$(cat /var/run/openvswitch/ovs-vswitchd.pid); then
        error_exit "ERROR - Failed to get pid for ovs-vswitchd, exit status: $?"
    fi

    if ! output=$(ovs-appctl -t /var/run/openvswitch/ovs-vswitchd."$pid".ctl ofproto/list); then
        error_exit "ERROR - Failed retrieving ofproto/list from ovs-vswitchd"
    fi

    if [ -z "$output" ]; then
        error_exit "ERROR - No bridges on ofproto/list output"
    fi
}

check_ovs_vswitchd_status
