#!/bin/bash

if [ "${DB_TYPE}" == "NB" ]; then
    db="${OVS_DBDIR}/ovnnb_db.db"
    db_name="OVN_Northbound"
    db_sock=${OVS_RUNDIR}/ovnnb_db.sock
    db_port=6641
    raft_port=6643
    db_global_table=NB_Global
    schema="/usr/share/openvswitch/ovn-nb.ovsschema"
elif [ "${DB_TYPE}" == "SB" ]; then
    db="${OVS_DBDIR}/ovnsb_db.db"
    db_name="OVN_Southbound"
    db_sock=${OVS_RUNDIR}/ovnsb_db.sock
    db_port=6642
    raft_port=6644
    db_global_table=SB_Global
    schema="/usr/share/openvswitch/ovn-sb.ovsschema"
else
    echo "Unknown DB_TYPE: ${DB_TYPE}" >&2
    exit 1
fi
