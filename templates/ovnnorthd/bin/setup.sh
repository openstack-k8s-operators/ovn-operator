#!/bin/bash
set -e

for arg in "$@"; do
    case $arg in
        --ovnnb-db=*) NB_DB="${arg#*=}" ;;
        --certificate=*) CERT="${arg#*=}" ;;
        --private-key=*) PKEY="${arg#*=}" ;;
        --ca-cert=*) CACERT="${arg#*=}" ;;
    esac
done

if [ -n "$NB_DB" ]; then
    NBCTL="ovn-nbctl --no-leader-only --db=$NB_DB"
    if [ -n "$CERT" ]; then
        NBCTL="$NBCTL --certificate=$CERT --private-key=$PKEY --ca-cert=$CACERT"
    fi

    export OVN_NB_DAEMON=$(${NBCTL} --pidfile --detach)

    for port in $(${NBCTL} --bare --columns _uuid find logical_switch_port tag!='[]' tag_request='[]')
    do
        tag=$(${NBCTL} lsp-get-tag $port)
        echo "Fixing tag_request for $port tag=$tag"
        ${NBCTL} set logical_switch_port $port tag_request=$tag
    done

    kill $(cat $OVN_RUNDIR/ovn-nbctl.pid)
    unset OVN_NB_DAEMON
fi

exec /usr/bin/ovn-northd "$@"
