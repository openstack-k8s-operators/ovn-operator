#!/bin/bash

# Script returns:
# 0 - if both NB and SB connections are OK
# 1 - if there is no NB connection
# 2 - if there is no SB connection
# 3 - if there are no NB and SB connection

NORTHD_PID=$(/usr/bin/pidof ovn-northd)
UNIXCTL_SOCKET="${OVN_RUNDIR}/ovn-northd.${NORTHD_PID}.ctl"

NB_CONNECTION_STATUS_OK=0
NB_CONNECTION_STATUS_FAILED=1
SB_CONNECTION_STATUS_OK=0
SB_CONNECTION_STATUS_FAILED=2

FINAL_STATUS=0

nb_connection_status=$(/usr/bin/ovn-appctl -t $UNIXCTL_SOCKET nb-connection-status)
if [[ $nb_connection_status != *"connected"* ]]; then
    FINAL_STATUS=$(( $FINAL_STATUS + $NB_CONNECTION_STATUS_FAILED ))
fi

sb_connection_status=$(/usr/bin/ovn-appctl -t $UNIXCTL_SOCKET sb-connection-status)
if [[ $sb_connection_status != *"connected"* ]]; then
    FINAL_STATUS=$(( $FINAL_STATUS + $SB_CONNECTION_STATUS_FAILED ))
fi

exit $FINAL_STATUS
