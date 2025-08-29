#!/bin/bash
set -euxo pipefail

CHECKER=$INSTALL_DIR/crd-schema-checker

DISABLED_VALIDATORS=NoBools # TODO: https://issues.redhat.com/browse/OSPRH-12254

CHECKER_ARGS=""
if [[ ${DISABLED_VALIDATORS:+x} ]]; then
    CHECKER_ARGS="$CHECKER_ARGS "
    for check in ${DISABLED_VALIDATORS//,/ }; do
        CHECKER_ARGS+=" --disabled-validators $check"
    done
fi

TMP_DIR=$(mktemp -d)

function cleanup {
    rm -rf "$TMP_DIR"
}

trap cleanup EXIT


for crd in config/crd/bases/*.yaml; do
    mkdir -p "$(dirname "$TMP_DIR/$crd")"
    git show "$BASE_REF:$crd" > "$TMP_DIR/$crd"
    $CHECKER check-manifests \
        $CHECKER_ARGS \
        --existing-crd-filename="$TMP_DIR/$crd" \
        --new-crd-filename="$crd"
done
