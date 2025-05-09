#!/bin/sh
#
# Copyright 2024 Red Hat Inc.
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


source $(dirname $0)/functions

# We need to wait for vswitchd because ovn-controller
# is not aware of flow restoration and can break the process.
# Removal of this function can be done once
# https://issues.redhat.com/browse/FDP-1292 is fixed
wait_for_vswitchd

# The order - first wait for db server, then set -ex - is important. Otherwise,
# wait_for_vswitchd interrim check would make the script exit.
set -ex

$@
