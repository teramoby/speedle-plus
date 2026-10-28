#!/bin/sh
set -eu
# Publish a complete initial store without replacing an existing policy file.
# The two development containers may initialize the shared volume concurrently.
if [ "${SPDL_STORE_TYPE:-file}" = file ]; then
    store_file="${SPDL_FILESTORE_LOC:-/var/lib/speedle/policies.json}"
    if [ ! -e "$store_file" ]; then
        candidate=$(mktemp "${store_file}.init.XXXXXX")
        trap 'rm -f "$candidate"' EXIT
        chmod 600 "$candidate"
        printf '{}\n' > "$candidate"
        ln "$candidate" "$store_file" 2>/dev/null || test -f "$store_file"
        rm -f "$candidate"
        trap - EXIT
    fi
fi
exec speedle-ads "$@"
