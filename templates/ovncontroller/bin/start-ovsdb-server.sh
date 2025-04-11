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

echo "start ovsdb-server"

# Check state
if [ -f /var/lib/openvswitch/already_executed ]; then
    if [ $(cat /var/lib/openvswitch/already_executed) == "UPDATE" ]; then
        echo "In a middle of an upgrade"
        # Need to stop vswitch and dbserver
        # First stop vswitchd
        vswitchd_pid=$(cat /run/openvswitch/ovs-vswitchd.pid)
        # Stop vswitch
        echo "stopping vswitchd"
        bash /usr/local/bin/container-scripts/stop-vswitchd.sh
        echo "Done, stopped vswitchd"
        # Wait for vswitchd to end checking status
        while true; do
            if [ $(cat /var/lib/openvswitch/already_executed) == "RESTART_VSWITCHD" ]; then
                break
            fi
            sleep 0.1
        done
        echo "Status is already RESTART_VSWITCHD"
        bash /usr/local/bin/container-scripts/stop-ovsdb-server.sh
        echo "Done, stopped ovsdb-server"
        # vswitchd stopped
        # We still need to run the ovsdb-server in this new container, this can be done
        # with the flag --overwrite-pidfile but we need to ignore the next SIGTERM that will
        # send openshift, creating file to noop the stop-ovsdb-server.sh
        echo "setting flag to skip ovsdb-server stop"
        touch /var/lib/openvswitch/skip_stop_ovsdbserver
    else
        # It could happen that ovsdb-server or ovs-vwsitchd pod can't start correctly or can't get to running state
        # this would cause this script to be run with already_executed with an state different than "UPDATE"
        :
    fi
fi

# Remove the obsolete semaphore file in case it still exists.
cleanup_ovsdb_server_semaphore

# Set state to "OVSDB_SERVER"
echo "OVSDB_SERVER" > /var/lib/openvswitch/already_executed

# Start the service
ovsdb-server ${DB_FILE} \
    --pidfile --overwrite-pidfile \
    --remote=punix:/var/run/openvswitch/db.sock \
    --private-key=db:Open_vSwitch,SSL,private_key \
    --certificate=db:Open_vSwitch,SSL,certificate \
    --bootstrap-ca-cert=db:Open_vSwitch,SSL,ca_cert
