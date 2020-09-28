#!/bin/bash

OVN_LIBDIR="/usr/share/openvswitch"

# These environment variables changed name at some point. OVN_* is the new name, but this version
# uses OVS_*.
export OVS_DBDIR=${OVN_DBDIR}
export OVS_RUNDIR=${OVN_RUNDIR}
