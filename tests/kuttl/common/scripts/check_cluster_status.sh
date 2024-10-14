#!/bin/bash

# arguments: db-type: {nb, sb}, num-pods, ssl/tcp
# Check arguments
if [ $# -lt 2 ]; then
    echo "Usage: $0 <db-type> <num-pods> [ssl]"
    exit 1
fi

DB_TYPE="$1"
NUM_PODS="$2"
PROTOCOL="${3:-tcp}"
POD_PREFIX="ovsdbserver-${DB_TYPE}"
CTL_FILE="ovn${DB_TYPE}_db.ctl"
if [ "$DB_TYPE" == "nb" ]; then
    DB_NAME="OVN_Northbound"
elif [ "$DB_TYPE" == "sb" ]; then
    DB_NAME="OVN_Southbound"
fi

declare -a pods
for i in $(seq 0 $((NUM_PODS-1))); do
    pods+=("${POD_PREFIX}-${i}")
done

# check each pod replica
for pod in "${pods[@]}"; do

    echo "Checking status of $pod"
    output=$(oc exec $pod -n $NAMESPACE -- bash -c "OVS_RUNDIR=/tmp ovs-appctl -t /tmp/$CTL_FILE cluster/status $DB_NAME")

    # Example of part of output string that needs parsing:
    # Status: cluster member
    # Connections: ->0000 ->5476 <-5476 <-36cf
    # Disconnections: 0
    # Servers:
    #     36cf (36cf at ssl:ovsdbserver-nb-0.ovsdbserver-nb.openstack.svc.cluster.local:6643) last msg 590 ms ago
    #     85de (85de at ssl:ovsdbserver-nb-2.ovsdbserver-nb.openstack.svc.cluster.local:6643) (self)
    #     5476 (5476 at ssl:ovsdbserver-nb-1.ovsdbserver-nb.openstack.svc.cluster.local:6643) last msg 12063993 ms ago

    # Check if the pod is a cluster member
    is_cluster_member=$(echo "$output" | grep -q "Status: cluster member"; echo $?)
    if [ $is_cluster_member -ne 0 ]; then
        exit 1
    fi

    # check if the pod is connected with all other pods
    for server in "${pods[@]}"; do
    echo "Checking if $server is mentioned in the output"
    if ! echo "$output" | grep -q "$PROTOCOL:$server"; then
        exit 1
    fi
    done
done
