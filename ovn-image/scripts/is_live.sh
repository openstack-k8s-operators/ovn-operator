#!/bin/bash

. /dbtype.sh

exec ovsdb-client dump "tcp:${SVC_NAME}:${db_port}" \
    "${db_name}" "${db_global_table}" _uuid
