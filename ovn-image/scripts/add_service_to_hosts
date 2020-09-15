#!/bin/bash

set -e -o pipefail

cp /etc/hosts $HOSTS_VOLUME/hosts
echo "$POD_IP $SVC_NAME" >> $HOSTS_VOLUME/hosts
