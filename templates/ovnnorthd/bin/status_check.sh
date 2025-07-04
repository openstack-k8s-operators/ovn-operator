#!/bin/bash

NORTHD_PID=$(/usr/bin/pidof ovn-northd)
UNIXCTL_SOCKET="${OVN_RUNDIR}/ovn-northd.${NORTHD_PID}.ctl"

status=$(/usr/bin/ovs-appctl -t $UNIXCTL_SOCKET status)
if [[ $status != *"active"* ]]; then
    exit 1
fi
