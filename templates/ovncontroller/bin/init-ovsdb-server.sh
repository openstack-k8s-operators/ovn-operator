#!/bin/sh
#
# Copyright 2023 Red Hat Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License"); you may
# not use this file except in compliance with the License. You may obtain
# a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
# WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
# License for the specific language governing permissions and limitations
# under the License.

set -ex
source $(dirname $0)/functions
trap wait_for_db_creation EXIT

# If db file is empty, remove it; otherwise service won't start.
# See https://issues.redhat.com/browse/FDP-689 for more details.
if ! [ -s ${DB_FILE} ]; then
    rm -f ${DB_FILE}
fi

# Compact the Database file if it exists
if [ -f ${DB_FILE} ]; then
    ovsdb-tool compact ${DB_FILE}
fi

# Initialize or upgrade database if needed
CTL_ARGS="--system-id=random --no-ovs-vswitchd"
/usr/share/openvswitch/scripts/ovs-ctl start $CTL_ARGS
/usr/share/openvswitch/scripts/ovs-ctl stop $CTL_ARGS

wait_for_db_creation
trap - EXIT
