#!/bin/sh
set -eu
# Both development containers share one volume. Create only a missing store;
# noclobber prevents concurrent startup from truncating another container's file.
if [ "${SPDL_STORE_TYPE:-file}" = file ]; then
    store_file="${SPDL_FILESTORE_LOC:-/var/lib/speedle/policies.json}"
    if [ ! -e "$store_file" ]; then
        (umask 077; set -C; printf '{}\n' > "$store_file") || test -f "$store_file"
    fi
fi
exec speedle-pms "$@"
