"""Shared full regression for file, etcd, MongoDB and MySQL test backends.

Database provisioning belongs to the backend launcher; this engine reuses the
MySQL regression's native PMS/ADS, preparation and tagged test execution.
"""
import fcntl
import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import tempfile
import time
import urllib.request

ROOT = Path(__file__).resolve().parents[2]
CLIENT = urllib.request.build_opener(urllib.request.ProxyHandler({}))


def free_port():
    with socket.socket() as listener:
        listener.bind(('127.0.0.1', 0))
        return listener.getsockname()[1]


def stop(process):
    if process.poll() is None:
        process.send_signal(signal.SIGINT)
        try:
            process.wait(timeout=15)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=10)


def run_regression(environment, configuration, results):
    backend = configuration['storeConfig']['storeType']
    if backend not in ('file', 'etcd', 'mongodb', 'mysql'):
        raise ValueError('unsupported regression backend')
    results = Path(results).resolve()
    results.mkdir(parents=True, exist_ok=True)
    environment = dict(environment, STORE_TYPE=backend)
    for key in list(environment):
        if key.startswith('SPDL_'):
            environment.pop(key)
    environment.update({key: '' for key in (
        'SP_APP_NAME', 'PMS_ADMIN_TOKEN', 'ADS_ADMIN_TOKEN', 'CA_LOCATION',
        'CERT_LOCATION', 'KEY_LOCATION', 'SPEEDLE_CLI_COVERPROFILE',
    )})
    failed, suites, processes, logs = False, [], [], []
    temporary = None

    def suite(name, args, env=None):
        nonlocal failed
        name = backend + '-' + name
        print(name + ': starting', flush=True)
        with (results / (name + '.json')).open('w') as out, (results / (name + '.stderr')).open('w') as err:
            code = subprocess.run(args, cwd=ROOT, env=env or environment, stdout=out, stderr=err).returncode
        suites.append((name, code))
        failed |= code != 0
        print(f'{name}: exit {code}', flush=True)
        return code

    try:
        # The existing etcd implementation tests use fixed ports. Serialize
        # this engine on the host and refuse to touch an existing listener.
        temporary = tempfile.TemporaryDirectory(prefix='speedle-' + backend + '-')
        with open('/tmp/speedle-plus-regression.lock', 'a') as lock:
            fcntl.flock(lock, fcntl.LOCK_EX)
            for port in (2379, 2380):
                with socket.socket() as probe:
                    probe.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
                    probe.bind(('127.0.0.1', port))
            work = Path(temporary.name)
            binaries = work / 'bin'
            binaries.mkdir()
            with (results / (backend + '-build.log')).open('w') as build_log:
                for name in ('speedle-pms', 'speedle-ads', 'spctl'):
                    subprocess.run(['go', 'build', '-race', '-o', str(binaries / name), './cmd/' + name],
                                   cwd=ROOT, env=environment, stdout=build_log, stderr=build_log, check=True)
            # Include every package in the root module, without backend filters.
            suite('go', ['go', 'test', '-race', '-json', '-p', '1', '-count=1', '-timeout', '600s', './...'])
            # The same feature stress and fuzz stages must run for every store.
            suite('feature-stress', ['go', 'test', '-race', '-json', '-count=1', '-timeout', '600s',
                                     '-tags', 'feature_stress', './pkg/eval', '-run', '^TestFeature'])
            for package, target in (('pdl', 'FuzzParseStatements'), ('eval/condition', 'FuzzConditionCheck'),
                                    ('store/file', 'FuzzCheckSPDL')):
                suite('fuzz-' + target, ['go', 'test', '-json', '-run', '^$', '-fuzz', '^' + target + '$',
                                        '-fuzztime=5s', '-parallel=2', './pkg/' + package])
            pms_port, ads_port = free_port(), free_port()
            while ads_port == pms_port:
                ads_port = free_port()
            if backend == 'file':
                location = work / 'policies.json'
                location.write_text('{}')
                configuration['storeConfig']['storeProps']['FileLocation'] = str(location)
            configuration['enableWatch'] = True
            config_file = work / 'store.json'
            config_file.write_text(json.dumps(configuration))
            config_file.chmod(0o600)
            for kind, port in (('pms', pms_port), ('ads', ads_port)):
                log = (results / (backend + '-' + kind + '.log')).open('w')
                logs.append(log)
                process = subprocess.Popen([str(binaries / ('speedle-' + kind)), '--config-file', str(config_file),
                                            '--insecure=true', '--endpoint', f'127.0.0.1:{port}'],
                                           cwd=work, env=environment, stdout=log, stderr=log)
                processes.append(process)
                deadline = time.monotonic() + 30
                while True:
                    if process.poll() is not None:
                        raise RuntimeError(kind.upper() + ' exited before readiness')
                    try:
                        with socket.create_connection(('127.0.0.1', port), timeout=1):
                            break
                    except OSError:
                        if time.monotonic() > deadline:
                            raise RuntimeError(kind.upper() + ' did not listen')
                        time.sleep(0.1)
            runtime_env = dict(environment, SP_APP_NAME=str(binaries / 'spctl'),
                               PMS_ENDPOINT=f'http://127.0.0.1:{pms_port}', ADS_ENDPOINT=f'http://127.0.0.1:{ads_port}',
                               SPEEDLE_TEST_EXTERNAL_SERVICES='1')
            with CLIENT.open(urllib.request.Request(runtime_env['PMS_ENDPOINT'] + '/policy-mgmt/v1/service', method='DELETE'), timeout=10):
                pass
            prepared = suite('ads-prepare', ['go', 'test', '-race', '-json', '-count=1', '-timeout', '600s',
                                            '-tags', 'runtime_test_prepare', './pkg/svcs/adsrest', '-run', '^TestPrepareCheckEnvironment$'], runtime_env)
            if prepared == 0:
                time.sleep(0.2)
                suite('ads-runtime', ['go', 'test', '-race', '-json', '-count=1', '-timeout', '1200s',
                                      '-tags', 'runtime_test,runtime_cache_test', './pkg/svcs/adsrest'], runtime_env)
            else:
                failed = True
                (results / (backend + '-ads-runtime.stderr')).write_text('Blocked by failed preparation.\n')
            if any(p.poll() is not None for p in processes):
                raise RuntimeError('native PMS/ADS exited during regression')
    except Exception as error:
        failed = True
        # Keep exception details in the controlled artifact; do not print DSNs.
        (results / (backend + '-fixture.stderr')).write_text(str(error) + '\n')
    finally:
        for process in reversed(processes):
            stop(process)
            failed |= process.returncode != 0
        for log in logs:
            log.close()
        if temporary is not None:
            temporary.cleanup()
        counts = {'pass': 0, 'fail': 0, 'skip': 0}
        failed_tests, skipped_tests = [], []
        for path in results.glob(backend + '-*.json'):
            # A reused results directory holds the previous run's summary.
            if path.name == backend + '-summary.json':
                continue
            for line in path.read_text().splitlines():
                try:
                    event = json.loads(line)
                except ValueError:
                    continue
                if not isinstance(event, dict):
                    continue
                action = event.get('Action')
                if event.get('Test') and action in counts:
                    counts[action] += 1
                if action == 'fail':
                    failed_tests.append({'suite': path.stem, 'package': event.get('Package'), 'test': event.get('Test')})
                if action == 'skip':
                    skipped_tests.append({'suite': path.stem, 'package': event.get('Package'), 'test': event.get('Test')})
        for path in results.glob(backend + '-*'):
            if path.is_file() and 'WARNING: DATA RACE' in path.read_text(errors='replace'):
                failed = True
        summary = {'backend': backend, 'counts': counts, 'suites': suites, 'failed': failed,
                   'failed_tests': failed_tests, 'skipped_tests': skipped_tests}
        (results / (backend + '-summary.json')).write_text(json.dumps(summary, indent=2))
        print(f'{backend} full regression: {counts}; suites={suites}; failed={failed}', flush=True)
    return failed, suites
