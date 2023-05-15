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

CTL_ARGS="--system-id=random --no-ovs-vswitchd"

# Initialize or upgrade database if needed
/usr/share/openvswitch/scripts/ovs-ctl start $CTL_ARGS
/usr/share/openvswitch/scripts/ovs-ctl stop $CTL_ARGS

# Start the service
ovsdb-server /etc/openvswitch/conf.db \
    --pidfile \
    --remote=punix:/var/run/openvswitch/db.sock \
    --private-key=db:Open_vSwitch,SSL,private_key \
    --certificate=db:Open_vSwitch,SSL,certificate \
    --bootstrap-ca-cert=db:Open_vSwitch,SSL,ca_cert
