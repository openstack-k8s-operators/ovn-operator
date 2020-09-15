#!/bin/bash

. /dbtype.sh

exec ovsdb-client wait "unix:${db_sock}" "${db_name}" connected
