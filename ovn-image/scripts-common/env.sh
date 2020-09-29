#!/bin/bash

. /image-env.sh

OVN_SCRIPTSDIR=${OVN_SCRIPTSDIR:-${OVN_LIBDIR}/scripts}

if [ "${DB_TYPE}" == "NB" ]; then
    lower="nb"
    db_name="OVN_Northbound"
    db_port=6641
    raft_port=6643
    db_global_table=NB_Global
elif [ "${DB_TYPE}" == "SB" ]; then
    lower="sb"
    db_name="OVN_Southbound"
    db_port=6642
    raft_port=6644
    db_global_table=SB_Global
else
    echo "Unknown DB_TYPE: ${DB_TYPE}" >&2
    exit 1
fi

db="${OVN_DBDIR}/ovn${lower}_db.db"
db_sock=${OVN_RUNDIR}/ovn${lower}_db.sock
db_ctl=${OVN_RUNDIR}/ovn${lower}_db.ctl
schema=${OVN_LIBDIR}/ovn-${lower}.ovsschema

if [ -z "${SERVER_NAME}" ]; then
    echo "SERVER_NAME is not set" >&2
    exit 1
fi

raft_address=tcp:${SERVER_NAME}:$raft_port
ovn_ctl=${OVN_SCRIPTSDIR}/ovn-ctl
