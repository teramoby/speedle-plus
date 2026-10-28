#!/bin/sh

if [ ! -e /var/lib/speedle/policies.json ]; then
    echo "{}" > /var/lib/speedle/policies.json
fi


# Plain HTTP for this development image; pass --insecure=false with --cert/--key for TLS.
speedle-pms --filestore-loc /var/lib/speedle/policies.json --endpoint 0.0.0.0:6733 --insecure=true $*

