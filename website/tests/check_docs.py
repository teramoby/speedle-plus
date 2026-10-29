#!/usr/bin/env python3
"""Execute bilingual docs commands against owned native services.
Linux uses private user/network/PID namespaces and exact documented ports.
macOS remaps fixture listen ports; no existing service or user config is changed.
"""
import argparse
import contextlib
import json
import os
from pathlib import Path
import re
import signal
import socket
import ssl
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request

ROOT=Path(__file__).resolve().parents[2]
DOCS=ROOT/'docs/hugo/content/docs'
PAGES=['deployment','pms/discover','store','idd','decisions','pms/diagnosis','pms/policy-mgmt','pms/global-policy','assertor','custom-function','security']
PATTERN=r'<!-- (?:docs-check|deployment-check): ([a-z0-9-]+) -->\s*```bash\n(.*?)\n```'
CLIENT=urllib.request.build_opener(urllib.request.ProxyHandler({}))

def run(args,cwd,env,timeout=180,ok=True):
    p=subprocess.run(args,cwd=cwd,env=env,text=True,capture_output=True,timeout=timeout)
    if ok and p.returncode: raise RuntimeError(f'Command failed: {args}\n{p.stdout}\n{p.stderr}')
    return p

def steps(page,locale):
    text=(DOCS/(page+('.zh' if locale=='zh' else '')+'.md')).read_text()
    result=re.findall(PATTERN,text,re.S)
    assert len(result)==len(set(n for n,_ in result)), (page,locale,'duplicate test marker')
    assert len(result)+len(re.findall(r'<!-- deployment-template: [^>]+ -->\s*```bash',text))==len(re.findall(r'^```bash$',text,re.M)), (page,locale,'unmarked shell block')
    for path,code in re.findall(r'<!-- docs-source: ([^>]+) -->\s*```go\n(.*?)\n```',text,re.S):
        assert code.rstrip()==(ROOT/path).read_text().rstrip(), (page,locale,'sample source drift',path)
    return result

def static_checks():
    for page in PAGES:
        assert steps(page,'en')==steps(page,'zh'), (page,'English/Chinese command drift')
    for path in DOCS.rglob('*.md'):
        text=path.read_text()
        assert not re.search(r'(?:Go|Golang)\s+1\.(?:[0-9]|1[0-9]|2[0-4])\b',text), (path,'obsolete Go prerequisite')
        for match in re.finditer(r'^```[^\n]*\n(.*?)\n```',text,re.S|re.M):
            code=match[1].replace('\\\n',' ')
            assert not re.search(r'(?m)^\s*(?:\$ )?(?:dep\s+ensure|go get\s+-u)',code),(path,'obsolete dependency or tool installation command')
            for line in code.splitlines():
                if re.match(r'^\s*(?:\$ )?curl\b',line) and ('-d ' in line or '--data' in line):
                    assert 'Content-Type: application/json' in line,(path,'missing JSON Content-Type',line)
        for template in re.findall(r'<!-- deployment-template: [^>]+ -->\s*```bash\n(.*?)\n```',text,re.S):
            result=subprocess.run(['bash','-n'],input=template,text=True,capture_output=True)
            assert result.returncode==0,(path,result.stderr)
        for match in re.finditer(r'^```json\n(.*?)\n```',text,re.S|re.M):
            json.loads(match[1])
    for kind in ['pms','ads']:
        assert (ROOT/f'docs/hugo/static/protobuf/{kind}.proto').read_bytes()==(ROOT/f'pkg/svcs/{kind}grpc/pb/service.proto').read_bytes()
    print('PASS paired commands, complete sample sources, JSON blocks, Content-Type and public proto copies',flush=True)

def fresh_port():
    with socket.socket() as probe:
        probe.bind(('127.0.0.1',0));return probe.getsockname()[1]

def wait(process,url,client=CLIENT):
    end=time.monotonic()+30
    while time.monotonic()<end:
        if process.poll() is not None:raise RuntimeError(f'Fixture exited before readiness: {process.returncode}')
        try:
            with client.open(url,timeout=1):return
        except urllib.error.HTTPError as e:
            if e.code in (404,405):return
            raise
        except (urllib.error.URLError,TimeoutError):time.sleep(.1)
    raise RuntimeError('Fixture readiness timeout: '+url)

def stop(process):
    if process.poll() is None:
        os.killpg(process.pid,signal.SIGINT)
        try:process.wait(timeout=12)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid,signal.SIGKILL);process.wait(timeout=5)
            raise RuntimeError('Owned fixture did not stop on Ctrl-C')

def check_result(name,result):
    allowed={'idd-allow','idd-any','decision-allow','global-allow','token-allow','token-any','function-allow','discover-allow'}
    denied={'idd-deny','idd-write-deny','decision-deny','diagnosis-deny','global-deny','token-deny','function-deny','discover-deny'}
    if name in allowed|denied:
        body=json.loads(result);assert body['allowed']==(name in allowed),(name,body)
        assert body['reason']==(0 if name in allowed else 1 if name=='diagnosis-deny' else 3),(name,body)
    elif name=='discover-record':
        body=json.loads(result);assert body.get('allowed') is True and body.get('reason')==5,(name,body)
    elif name=='discover-list':assert 'alice' in result and 'observed' in result,result
    elif name=='decision-roles':assert set(json.loads(result))=={'role1','role2'},result
    elif name=='decision-permissions':assert json.loads(result)==[{'resource':'/books/HarryPotter','actions':['download']}],result
    elif name=='diagnose':
        body=json.loads(result);assert body['allowed'] is False and body['reason']=='DENY_POLICY_FOUND',body
        assert isinstance(body['attributes'],dict) and isinstance(body['attributes']['request_month'],int)
        assert any(p['effect']=='deny' and p['status']=='takeEffect' and isinstance(p['id'],str) for p in body['policies'])
        assert all(isinstance(p['principals'][0],list) for p in body['policies'])
    elif name=='token-invalid':
        status=int(re.search(r'HTTP (\d+)',result)[1]);assert status==200,(name,result)
        body=json.loads(result.split('\nHTTP ')[0]);assert body['allowed'] is False and body.get('errorMessage'),result
    elif name=='embedded':assert 'alice: allowed=true' in result and 'bob: allowed=false' in result,result
    elif name=='store':assert 'Registered myfile store; services=0' in result,result
    elif name in ['secure-cli','secure-curl']:assert json.loads(result)==[],result

def verify_page(page,locale,binaries,env,parent,isolated):
    with tempfile.TemporaryDirectory(prefix='page-',dir=parent) as tmp,contextlib.ExitStack() as resources:
        work=Path(tmp);processes=[];logs=[];ports={}
        for port in (6733,6734,8080,23456,50001,50002):
            chosen=port if isolated else fresh_port()
            while chosen in ports.values():chosen=fresh_port()
            ports[port]=chosen
        # A disposable module view permits literal go run commands without editing source.
        for name in ['go.mod','go.sum','api','pkg','cmd','3rdparty','samples','website']:(work/name).symlink_to(ROOT/name,target_is_directory=(ROOT/name).is_dir())
        (work/'bin').symlink_to(binaries,target_is_directory=True)
        commands=steps(page,locale);secure=page=='security';tlsclient=None
        def local(command):
            for original,replacement in ports.items():
                command=command.replace(f'127.0.0.1:{original}',f'127.0.0.1:{replacement}')
            # Ephemeral gRPC addresses in normal docs become known owned test ports.
            if command.startswith('./bin/speedle-pms'):command=command.replace('127.0.0.1:0',f'127.0.0.1:{ports[50001]}')
            if command.startswith('./bin/speedle-ads'):command=command.replace('127.0.0.1:0',f'127.0.0.1:{ports[50002]}')
            if not isolated and command in ['go run ./website/tests/examples/asserter','go run ./website/tests/examples/custom-function']:
                command+=' --listen 127.0.0.1:'+str(ports[8080 if 'asserter' in command else 23456])
            return command
        try:
            for name,command in commands:
                command=local(command)
                if page=='deployment' and name=='source-build':
                    quick=(ROOT/'docs/hugo/content/quick-start.md').read_text()
                    build=re.search(r'<!-- quick-start: build -->\s*```bash\n(.*?)\n```',quick,re.S)[1]
                    assert build.splitlines()[:len(command.splitlines())]==command.splitlines(), 'Deployment build differs from the exercised Quick Start'
                    continue
                if name in ['pms','ads','secure-pms','secure-ads','mock-asserter','function-server']:
                    log=work/(name+'.log');logs.append(log);stream=resources.enter_context(log.open('w'))
                    p=subprocess.Popen(['bash','-euc','exec '+command],cwd=work,env=env,stdout=stream,stderr=subprocess.STDOUT,start_new_session=True);processes.append(p)
                    if name=='mock-asserter':url=f'http://127.0.0.1:{ports[8080]}/v1/assert';end=time.monotonic()+30
                    elif name=='function-server':url=f'http://127.0.0.1:{ports[23456]}/func/isValid'
                    else:url=('https' if secure else 'http')+'://127.0.0.1:'+str(ports[6733 if name.endswith('pms') else 6734])+('/policy-mgmt/v1/service' if name.endswith('pms') else '/authz-check/v1/is-allowed')
                    if name=='mock-asserter':
                        request=urllib.request.Request(url,headers={'x-token':'githubtoken','x-idp':'github'})
                        while time.monotonic()<end:
                            if p.poll() is not None:raise RuntimeError('Mock exited')
                            try:
                                with CLIENT.open(request,timeout=1) as r:assert r.status==200;break
                            except urllib.error.URLError:time.sleep(.1)
                        else:raise RuntimeError('Mock readiness timeout')
                    else:wait(p,url,tlsclient if secure else CLIENT)
                    continue
                result=run(['bash','-euc',command],work,env).stdout
                if name=='certificates':
                    context=ssl.create_default_context(cafile=work/'tls/ca.crt');context.load_cert_chain(work/'tls/client.crt',work/'tls/client.key')
                    tlsclient=urllib.request.build_opener(urllib.request.ProxyHandler({}),urllib.request.HTTPSHandler(context=context))
                # Wait for documented asynchronous watch propagation, preserving payloads.
                deadline=time.monotonic()+5
                while True:
                    try:check_result(name,result);break
                    except (AssertionError,KeyError):
                        if time.monotonic()>=deadline:raise
                        time.sleep(.05);result=run(['bash','-euc',command],work,env).stdout
            if page=='custom-function':
                run(['go','run','./website/tests/examples/certificates','--out','./tls'],work,env)
                tlsport=fresh_port()
                log=work/'function-tls.log';logs.append(log);stream=resources.enter_context(log.open('w'))
                command=['go','run','./website/tests/examples/custom-function','--listen',f'127.0.0.1:{tlsport}','--cert','./tls/server.crt','--key','./tls/server.key']
                process=subprocess.Popen(command,cwd=work,env=env,stdout=stream,stderr=subprocess.STDOUT,start_new_session=True);processes.append(process)
                context=ssl.create_default_context(cafile=work/'tls/ca.crt')
                trusted=urllib.request.build_opener(urllib.request.ProxyHandler({}),urllib.request.HTTPSHandler(context=context))
                wait(process,f'https://127.0.0.1:{tlsport}/func/isValid',trusted)
                definition={'name':'isValidTLS','funcURL':f'https://127.0.0.1:{tlsport}/func/isValid','ca':(work/'tls/ca.crt').read_text(),'resultCachable':False}
                request=urllib.request.Request(f'http://127.0.0.1:{ports[6733]}/policy-mgmt/v1/function',data=json.dumps(definition).encode(),headers={'Content-Type':'application/json'})
                with CLIENT.open(request,timeout=3) as response:assert response.status==201
                command=local('./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user Ally access tls-library if isValidTLS(attr1)" --service-name service1')
                run(['bash','-euc',command],work,env)
                for value,expected in [(200,True),(50,False)]:
                    payload={'subject':{'principals':[{'type':'user','name':'Ally'}]},'serviceName':'service1','resource':'tls-library','action':'access','attributes':[{'name':'attr1','type':'numeric','value':value}]}
                    deadline=time.monotonic()+5
                    while True:
                        request=urllib.request.Request(f'http://127.0.0.1:{ports[6734]}/authz-check/v1/is-allowed',data=json.dumps(payload).encode(),headers={'Content-Type':'application/json'})
                        with CLIENT.open(request,timeout=3) as response:result=json.load(response)
                        if result.get('allowed') is expected and result.get('reason')==(0 if expected else 3):break
                        if time.monotonic()>deadline:raise AssertionError(('TLS function decision',result))
                        time.sleep(.05)
                # A nonnumeric parameter reaches the handler's explicit error branch.
                request=urllib.request.Request(f'https://127.0.0.1:{tlsport}/func/isValid',data=b'{"params":["bad"]}',headers={'Content-Type':'application/json'})
                try:trusted.open(request,timeout=3)
                except urllib.error.HTTPError as error:assert error.code==400
                else:raise AssertionError('Function accepted a nonnumeric parameter')
            if secure:
                # Populate a real fixture before the ADS success control; an empty
                # store returns a legitimate service-not-found RPC error.
                payload=(ROOT/'website/tests/examples/bookstore.json').read_bytes()
                request=urllib.request.Request(f'https://127.0.0.1:{ports[6733]}/policy-mgmt/v1/service',data=payload,headers={'Content-Type':'application/json'})
                with tlsclient.open(request,timeout=3) as response:assert response.status==201
                probe=[str(binaries/'grpcprobe'),'--service','ads','--endpoint',f'127.0.0.1:{ports[50002]}','--cacert','tls/ca.crt','--cert','tls/client.crt','--key','tls/client.key','--expect-allowed']
                deadline=time.monotonic()+5
                while run(probe,work,env,ok=False).returncode:
                    if time.monotonic()>deadline:raise AssertionError('ADS did not load the secure fixture')
                    time.sleep(.05)
                # Both standalone entry points must use the protected transport helper.
                for kind,port in [('pms',50001),('ads',50002)]:
                    base=[str(binaries/'grpcprobe'),'--service',kind,'--endpoint',f'127.0.0.1:{ports[port]}']
                    for credentials,expected in [(['--insecure'],False),(['--cacert','tls/ca.crt'],False),(['--cacert','tls/ca.crt','--cert','tls/client.crt','--key','tls/client.key'],True)]:
                        result=run(base+credentials,work,env,ok=False);assert (result.returncode==0)==expected,(kind,credentials,result.stderr)
                # mTLS REST rejects no-cert clients, and works with a trusted certificate.
                context=ssl.create_default_context(cafile=work/'tls/ca.crt');no_cert=urllib.request.build_opener(urllib.request.ProxyHandler({}),urllib.request.HTTPSHandler(context=context))
                for port,path in [(6733,'/policy-mgmt/v1/service'),(6734,'/authz-check/v1/is-allowed')]:
                    try:no_cert.open(f'https://127.0.0.1:{ports[port]}'+path,timeout=2)
                    except (urllib.error.URLError,ssl.SSLError,ConnectionError):pass
                    else:raise AssertionError('mTLS REST accepted no client certificate')
                assert all((work/'tls'/name).stat().st_mode&0o777==0o600 for name in ['server.key','client.key'])
            assert all(process.poll() is None for process in processes), 'Fixture exited unexpectedly'
            verified=len(commands)-(1 if page=='deployment' else 0)
            print(f'{locale} {page}: PASS {verified} executed command blocks; runner={env.get("RUNNER_NAME","local")}; isolated={isolated}',flush=True)
            return verified
        except BaseException:
            for log in logs:print(log.name,log.read_text()[-2000:],file=sys.stderr)
            raise
        finally:
            for p in reversed(processes):stop(p)

def main():
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--inside',action='store_true');parser.add_argument('--bin',type=Path);parser.add_argument('--work',type=Path);args=parser.parse_args()
    env=dict(os.environ)
    for key in list(env):
        if key.startswith('SPDL_'):env.pop(key)
    env['GOFLAGS']=(env.get('GOFLAGS','')+' -mod=readonly -buildvcs=false').strip()
    env.update({'NO_PROXY':'localhost,127.0.0.1,::1','no_proxy':'localhost,127.0.0.1,::1'})
    if args.inside:
        run(['ip','link','set','lo','up'],ROOT,env);isolated=True
    else:
        static_checks()
        with tempfile.TemporaryDirectory(prefix='speedle-docs-check-') as temp:
            work=Path(temp);binaries=work/'bin';binaries.mkdir()
            for target,name in [('cmd/speedle-pms','speedle-pms'),('cmd/speedle-ads','speedle-ads'),('cmd/spctl','spctl'),('website/tests/grpcprobe','grpcprobe')]:
                run(['go','build','-o',str(binaries/name),'./'+target],ROOT,env)
            # Warm every literal Go example before entering a namespace without egress.
            run(['go','build','-o',str(binaries)+'/', './website/tests/examples/...'],ROOT,env)
            if sys.platform.startswith('linux'):
                command=['unshare','--user','--map-root-user','--net','--pid','--fork','--mount-proc','--kill-child=SIGKILL',sys.executable,str(Path(__file__).resolve()),'--inside','--bin',str(binaries),'--work',str(work)]
                print(run(command,ROOT,env,timeout=600).stdout,end='');return
            args.bin,args.work=binaries,work
            count=sum(verify_page(page,locale,binaries,env,work,False) for locale in ['en','zh'] for page in PAGES)
            print(f'PASS {count} bilingual docs command blocks (macOS owned-port remapping)',flush=True);return
    count=sum(verify_page(page,locale,args.bin,env,args.work,True) for locale in ['en','zh'] for page in PAGES)
    print(f'PASS {count} bilingual docs command blocks at exact documented ports in private Linux namespaces',flush=True)

if __name__=='__main__':main()
