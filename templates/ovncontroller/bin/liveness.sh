#!/bin/sh

# Ensure ovn-controller pid is up
output=$(pidof ovn-controller)
if [ "$output" != "1" ]; then
    echo "ERROR - OVN-Controller is not running"
    exit 1
fi

# Ensure ovn-controller is running and not paused, this running status is not
# affected by any other services/depedencies as the connection-status output
output=$(ovn-appctl -t ovn-controller debug/status)
if [ "$output" != "running" ]; then
    echo "ERROR - OVN-Controller is not reporting a running status"
    exit 1
fi

exit 0
