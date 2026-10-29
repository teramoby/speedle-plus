#!/bin/sh

# Configure the console with SPEEDLE_CONSOLE_* variables or flags, such as
# SPEEDLE_CONSOLE_PMS_URL, SPEEDLE_CONSOLE_ADS_URL and
# SPEEDLE_CONSOLE_ASSERTER_ENDPOINT. Plain HTTP for this development image;
# pass --insecure=false with --cert/--key for TLS.
speedle-console --endpoint 0.0.0.0:6735 --insecure=true "$@"
