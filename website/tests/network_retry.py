"""Bounded retries for the transient network steps of the walkthrough checkers.

The checkers execute real clones and Go module downloads from public services.
A TLS error, connection reset or timeout there says nothing about the
documentation, so those steps retry. Checks of the result (allow/deny output,
module identity) must not be retried.
"""
import subprocess
import time


def run_with_retry(action, attempts=3, delay=5.0):
    """Retry a network action up to `attempts` times with exponential backoff.

    Only command failures and timeouts are retried: they cover the transient
    TLS and connection resets of clone, module download and install steps.
    Assertions about the result are never retried.
    """
    for attempt in range(1, attempts + 1):
        try:
            return action()
        except (RuntimeError, subprocess.TimeoutExpired) as error:
            if attempt == attempts:
                raise
            detail = str(error).splitlines()[0]
            print(f"Transient failure (attempt {attempt}/{attempts}); retrying in {delay:.0f}s: {detail}", flush=True)
            time.sleep(delay)
            delay *= 2
