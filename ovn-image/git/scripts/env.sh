#!/bin/bash

PATH=/opt/bin:/opt/sbin:${PATH}

if [ "${DB_TYPE}" == "NB" ]; then
    db="${OVN_DBDIR}/ovnnb_db.db"
    db_name="OVN_Northbound"
    db_sock=${OVN_RUNDIR}/ovnnb_db.sock
    db_port=6641
    raft_port=6643
    db_global_table=NB_Global
    schema="/opt/share/ovn/ovn-nb.ovsschema"
elif [ "${DB_TYPE}" == "SB" ]; then
    db="${OVN_DBDIR}/ovnsb_db.db"
    db_name="OVN_Southbound"
    db_sock=${OVN_RUNDIR}/ovnsb_db.sock
    db_port=6642
    raft_port=6644
    db_global_table=SB_Global
    schema="/opt/share/ovn/ovn-sb.ovsschema"
else
    echo "Unknown DB_TYPE: ${DB_TYPE}" >&2
    exit 1
fi

if [ -z "${SERVER_NAME}" ]; then
    echo "SERVER_NAME is not set" >&2
    exit 1
fi

raft_address=tcp:${SERVER_NAME}:$raft_port
ovn_ctl=/opt/share/ovn/scripts/ovn-ctl
