#!/usr/bin/env python3
"""Run the shared four-backend regression against an explicitly provisioned store."""
import json
import os
import secrets
from pathlib import Path
from store_regression import run_regression


def main():
    backend = os.environ.get('STORE_TYPE', 'file')
    properties = json.loads(os.environ.get('SPEEDLE_TEST_STORE_PROPS', '{}'))
    if backend == 'mongodb':
        properties.setdefault('MongoURI', os.environ.get('SPEEDLE_MONGO_URI', ''))
        prefix = properties.get('MongoDatabase', 'speedle_test_runtime')
        if not isinstance(prefix, str) or not prefix.startswith('speedle_test'):
            raise ValueError('MongoDB test database prefix must start with speedle_test')
        properties['MongoDatabase'] = prefix + '_' + secrets.token_hex(8)
    elif backend == 'mysql':
        properties.setdefault('MySQLDSN', os.environ.get('SPEEDLE_MYSQL_TEST_DSN', ''))
        properties.setdefault('MySQLPollInterval', '20ms')
        dsn = properties['MySQLDSN']
        if not isinstance(dsn, str) or not dsn.rsplit('/', 1)[-1].split('?', 1)[0].startswith('speedle_test'):
            raise ValueError('MySQL regression requires a disposable speedle_test database')
    elif backend == 'etcd':
        properties.setdefault('EtcdEndpoint', os.environ.get('SPEEDLE_ETCD_ENDPOINT', ''))
        # The regression refuses to run while anything listens on 2379/2380,
        # which the etcd implementation tests start their own server on.
        endpoint = properties['EtcdEndpoint']
        if not isinstance(endpoint, str) or not endpoint or endpoint.rsplit(':', 1)[-1] in ('2379', '2380'):
            raise ValueError('etcd regression requires SPEEDLE_ETCD_ENDPOINT on a port other than 2379/2380')
        properties['EtcdKeyPrefix'] = properties.get('EtcdKeyPrefix', '').rstrip('/') + '/speedle_test_runtime_' + secrets.token_hex(8) + '/'
    configuration = {'storeConfig': {'storeType': backend, 'storeProps': properties}, 'enableWatch': True}
    failed, _ = run_regression(os.environ, configuration, Path(os.environ.get('RESULTS_DIR', 'test-results')))
    return int(failed)


if __name__ == '__main__':
    raise SystemExit(main())
