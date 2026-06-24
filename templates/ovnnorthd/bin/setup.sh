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
    NBCTL="ovn-nbctl --no-leader-only"
    if [ -n "$CERT" ]; then
        NBCTL="$NBCTL --certificate=$CERT --private-key=$PKEY --ca-cert=$CACERT"
    fi

    # NOTE(twilson) This will wait for the ovsdb connection before detaching
    export OVN_NB_DAEMON=$(${NBCTL} --pidfile --detach --db=$NB_DB)

    # NOTE(twilson) ovn-nbctl will be connecting to the daemon successfully, so will wait forever
    # until the daemon responds. So even if there is a disconnect after initially connecting it waits.
    #
    # Due to OVN commit `171ed24dd northd: Clear stale LSP tags on tag_request removal.` and neutron's
    # historical practice of setting the tag directly instead of using tag_request, 26.03.1+ versions
    # of northd will clear all tags set by neutron prior to 18.0 FR6. For upgrades to FR6 and beyond
    # from an affected prior version (<= 18.0 FR6) this will fix the issue before northd starts.
    #
    # This is safe to remove when the minimum version from which upgrades are allowed is 18.0 FR7 or
    # later. Note that this setup script itself only exists to run this fix.
    for port in $(${NBCTL} --bare --columns _uuid find logical_switch_port tag!='[]' tag_request='[]'); do
        tag=$(${NBCTL} lsp-get-tag $port)
        echo "Fixing tag_request for $port tag=$tag"
        ${NBCTL} set logical_switch_port $port tag_request=$tag
    done

    kill $(cat $OVN_RUNDIR/ovn-nbctl.pid)
    unset OVN_NB_DAEMON
fi

exec /usr/bin/ovn-northd "$@"
