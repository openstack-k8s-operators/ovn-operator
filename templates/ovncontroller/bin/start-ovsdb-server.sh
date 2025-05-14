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

# Remove the obsolete semaphore file in case it still exists.
cleanup_ovsdb_server_semaphore

# Check if we're on the update path
if [ -f $update_semaphore_file ]; then
echo "In the middle of an upgrade"
    # Need to stop vsitchd
    echo "Stopping vswitchd"
    bash $stop_vswitchd_script_file
    # with this script the current lflows should be already stored in a file
    # and vswitchd should be stopped.
    # Need to wait until vswitchd is stoped in order to stop also the ovsdb-server
    while true; do
        if [ ! -f $ovs_vswitchd_pid_file ]; then
            break
        fi
        sleep 0.1
    done
    # Ovs-vswtichd was already restarted, need to skip the preStop from the openshift
    # lifecicle when the old pod gets deleted
    echo "Creating flag file to skip ovs-vswitchd stop"
    touch $skip_vswitchd_stop_file
    # Run stop-ovsdbserver script to ensure lflows semaphor is cleaned correctly
    bash $stop_ovsdb_server_script_file
    # Need to create a flag-file to skip ovsdb-server stop
    # to avoid triggering it again when openshift triggers the preStop script.
    echo "Creating flag file to skip ovsdb-server stop"
    touch $skip_ovsdb_server_stop_file
    # Ensure that ovsdb-server is stopped
    while true; do
        if [ ! -f $ovsdb_server_pid_file ]; then
            break
        fi
        sleep 0.1
    done
fi

# Start the service
ovsdb-server ${DB_FILE} \
    --pidfile \
    --remote=punix:/var/run/openvswitch/db.sock \
    --private-key=db:Open_vSwitch,SSL,private_key \
    --certificate=db:Open_vSwitch,SSL,certificate \
    --bootstrap-ca-cert=db:Open_vSwitch,SSL,ca_cert
