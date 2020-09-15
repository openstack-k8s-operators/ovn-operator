#!/bin/bash

set -e -o pipefail

# Always required:
#   DB_TYPE: "NB" or "SB"
#   SVC_NAME: Name of the service exposing this server
#   OVS_DBDIR: volume mount containing databases
#
# For bootstrap:
#   BOOTSTRAP: "true"
#
# For scale up:
#   CID: cluster ID of previously initialised cluster database
#   REMOTES: Space-separated list of remote server addresses

. /dbtype.sh

address=tcp:${SVC_NAME}:$db_port

if [ ! -f "$db" ]; then
    if [ "$BOOTSTRAP" == "true" ]; then
        ovsdb-tool create-cluster "$db" "$schema" "$address"
    else
        ovsdb-tool join-cluster --cid="$CID" \
            "$db" "$db_name" "$address" $REMOTES
    fi
fi

cid=$(ovsdb-tool db-cid "$db")
name=$(ovsdb-tool db-name "$db")
sid=$(ovsdb-tool db-sid "$db")
address=$(ovsdb-tool db-local-address "$db")

jq -n --arg cid "$cid" \
      --arg name "$name" \
      --arg sid "$sid" \
      --arg address "$address" \
      '{"clusterID": $cid, "name": $name, "serverID": $sid, "address": $address}'
