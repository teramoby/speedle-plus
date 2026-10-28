#!/usr/bin/env python3
"""Check production backend defaults and intentional legacy compatibility."""
from pathlib import Path
import os
import subprocess

ROOT=Path(__file__).resolve().parents[2]
HELM=os.environ.get('HELM_BIN','helm')
CHART='deployment/helm/speedle-prod'
EXAMPLE=CHART+'/values-example.yaml'

def render(options,success):
    result=subprocess.run([HELM,'template','speedle-prod',CHART,*options],cwd=ROOT,text=True,capture_output=True)
    assert (result.returncode==0)==success,(options,result.stdout,result.stderr)
    return result.stdout

render([],False)
for value in ['false','true','0','1']:
    render(['--set-string','store.etcd.endpoint=http://speedle-etcd:2379','--set-string','store.etcd.allowInsecure='+value],False)
for endpoint in ['http://speedle-etcd:2379','speedle-etcd:2379','HTTPS://speedle-etcd:2379']:
    render(['-f',EXAMPLE,'--set-string','store.etcd.endpoint='+endpoint],False)
for field in ['endpoint','etcdClientCertSecret','certFile','keyFile','trustedCAFile']:
    render(['-f',EXAMPLE,'--set-string','store.etcd.'+field+'='],False)
secure=render(['-f',EXAMPLE],True)
assert '--etcdstore-endpoint=https://speedle-etcd:2379' in secure
for suffix in ['cert=/etc/etcdtls/client.crt','key=/etc/etcdtls/client.key','ca=/etc/etcdtls/ca.crt']:
    assert '--etcdstore-tls-'+suffix in secure
legacy=['-f',EXAMPLE,'--set','store.etcd.allowInsecure=true']
ca_only=render(legacy+['--set-string','store.etcd.certFile=','--set-string','store.etcd.keyFile='],True)
assert '--etcdstore-tls-ca=' in ca_only and '--etcdstore-tls-cert=' not in ca_only and '--etcdstore-tls-key=' not in ca_only
render(legacy+['--set-string','store.etcd.keyFile='],False)
plaintext=render(['--set','store.etcd.allowInsecure=true','--set-string','store.etcd.endpoint=http://speedle-etcd:2379'],True)
assert '--etcdstore-tls-' not in plaintext
for chart,options in [('deployment/helm/speedle-dev',[]),(CHART,['-f',EXAMPLE])]:
    subprocess.run([HELM,'lint',chart,*options],cwd=ROOT,check=True)
print('PASS chart secure defaults, credential rejection and explicit CA-only/plaintext compatibility')
