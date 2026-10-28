#!/usr/bin/env python3
"""Run the paired deployment guides in owned Docker and kind resources."""
import contextlib
import fcntl
import grp
import json
import os
from pathlib import Path
import re
import pwd
import shlex
import shutil
import signal
import socket
import ssl
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request
import uuid

ROOT=Path(__file__).resolve().parents[2]
NODE='kindest/node:v1.36.4@sha256:099e049362a1526b2db71494e1947aae99bd16290d7c895f2b7ea312e3cbfaed'
PATTERN=r'<!-- deployment-check: ([a-z0-9-]+) -->\s*```bash\n(.*?)\n```'

def commands(locale):
    text=(ROOT/'docs/hugo/content/docs'/('deployment'+('.zh' if locale=='zh' else '')+'.md')).read_text()
    result=dict(re.findall(PATTERN,text,re.S))
    assert len(result)==len(re.findall(r'^```bash$',text,re.M)), 'Unmarked/duplicate deployment command'
    return result

def run(args,env=None,timeout=240,ok=True,input=None):
    result=subprocess.run(args,cwd=ROOT,env=env,text=True,input=input,capture_output=True,timeout=timeout)
    if ok and result.returncode:
        raise RuntimeError(f'Command failed: {args}\n{result.stdout[-5000:]}\n{result.stderr[-5000:]}')
    return result

def port():
    with socket.socket() as sock:
        sock.bind(('127.0.0.1',0));return sock.getsockname()[1]

def wait(check,description,timeout=90):
    end=time.monotonic()+timeout;last=None
    while time.monotonic()<end:
        try:
            value=check()
            if value:return value
        except (Exception,) as error:last=error
        time.sleep(.5)
    raise RuntimeError(f'Timed out: {description}; last error={last}')

class Acceptance:
    def __init__(self,work):
        self.work=work;self.id='speedle-check-'+uuid.uuid4().hex[:10]
        self.cluster=self.id;self.kind_created=False;self.nodes=[]
        self.env=dict(os.environ, KUBECONFIG=str(work/'kubeconfig'))
        self.env['NO_PROXY']=self.env['no_proxy']='localhost,127.0.0.1,::1'
        self.commands={locale:commands(locale) for locale in ['en','zh']}
        assert self.commands['en']==self.commands['zh'], 'English/Chinese deployment command drift'
        self.done={locale:set() for locale in ['en','zh']}
        self.containers=[];self.networks=[];self.volumes=[];self.images=[];self.processes=[]
        self.locale='en';self.phase={}

    def docker(self,*args,**kw):return run(['docker',*args],self.env,**kw)
    def kube(self,*args,**kw):return run(['kubectl','--context','kind-'+self.cluster,'--request-timeout=15s',*args],self.env,**kw)
    def ns(self,*args,**kw):return self.kube('--namespace',self.phase['NAMESPACE'],*args,**kw)

    def block(self,name,client=None,background=False):
        print(f'RUN {self.locale} {self.phase_dir.name}: {name}',flush=True)
        prefix=self.commands[self.locale]['setup']+'\n'
        if client:prefix+=self.commands[self.locale][client+'-client']+'\n'
        code=prefix+self.commands[self.locale][name]
        env=dict(self.env,**self.phase)
        if background:
            pidfile=self.phase_dir/(name+'-pids')
            vars=['DEV_FORWARD_PID'] if name=='forward-dev' else ['PMS_FORWARD_PID','ADS_FORWARD_PID']
            code+='\nprintf "%s\\n" '+ ' '.join('"$'+x+'"' for x in vars)+' > '+str(pidfile)+'\nwait\n'
            log=(self.phase_dir/(name+'.log')).open('w')
            process=subprocess.Popen(['bash','-euc',code],cwd=ROOT,env=env,stdout=log,stderr=subprocess.STDOUT,start_new_session=True)
            self.processes.append((process,log))
            values=wait(lambda:pidfile.read_text().splitlines() if pidfile.exists() else None,'forwarding PIDs')
            self.phase.update(dict(zip(vars,values)))
            output=''
        else:output=run(['bash','-euc',code],env,timeout=600 if name=='build' else 240).stdout
        self.done[self.locale].add(name)
        return output

    def phase_start(self,locale,name):
        self.locale=locale;self.phase_dir=self.work/(locale+'-'+name);self.phase_dir.mkdir()
        pms_port=port();ads_port=port()
        while ads_port==pms_port:ads_port=port()
        self.phase={
            'DEMO':self.id+'-'+locale+'-'+name,
            'NAMESPACE':self.id+'-'+locale+'-'+name,
            'DOCS_WORKDIR':str(self.phase_dir),
            'PMS_PORT':str(pms_port),'ADS_PORT':str(ads_port),
            'IMAGE_PREFIX':self.prefix,'IMAGE_TAG':self.tag,
        }
        self.block('setup')

    def client(self,tls):
        handlers=[urllib.request.ProxyHandler({})]
        if tls:
            context=ssl.create_default_context(cafile=self.phase_dir/'tls/ca.crt')
            context.load_cert_chain(self.phase_dir/'tls/client.crt',self.phase_dir/'tls/client.key')
            handlers.append(urllib.request.HTTPSHandler(context=context))
        return urllib.request.build_opener(*handlers)

    def ready(self,tls):
        url=('https' if tls else 'http')+'://127.0.0.1:'+self.phase['PMS_PORT']+'/policy-mgmt/v1/service'
        client=self.client(tls)
        wait(lambda:client.open(url,timeout=3).status==200,'forwarded PMS')
        url=('https' if tls else 'http')+'://127.0.0.1:'+self.phase['ADS_PORT']+'/authz-check/v1/is-allowed'
        def available():
            try:client.open(url,timeout=3)
            except urllib.error.HTTPError as e:return e.code in (404,405)
            return False
        wait(available,'forwarded ADS')

    def policy_checks(self,tls):
        client='https' if tls else 'http';self.block(client+'-client')
        created=json.loads(self.block('create',client));assert created['name']=='onlineBookStore',created
        def allowed():
            value=json.loads(self.block('allow',client));return value.get('allowed') is True and value.get('reason')==0
        wait(allowed,'grant propagated to ADS')
        denied=json.loads(self.block('deny',client));assert denied['allowed'] is False and denied['reason']==3,denied
        value=json.loads(self.block('read',client));assert value['name']=='onlineBookStore',value

    def probe_rpc(self,tls,pms_port,ads_port):
        for service,target in [('pms',pms_port),('ads',ads_port)]:
            base=[str(self.work/'grpcprobe'),'--service',service,'--endpoint',f'127.0.0.1:{target}']
            trust=['--cacert',str(self.phase_dir/'tls/ca.crt'),'--cert',str(self.phase_dir/'tls/client.crt'),'--key',str(self.phase_dir/'tls/client.key')]
            if service=='ads':base+=['--expect-allowed']
            run(base+(trust if tls else ['--insecure']),self.env)
            if tls:
                for credentials in [['--insecure'],trust[:2]]:
                    isolated=list(base)
                    target=self.forward('speedle-prod-'+service,50001 if service=='pms' else 50002)
                    isolated[isolated.index('--endpoint')+1]=f'127.0.0.1:{target}'
                    run(isolated+trust,self.env)
                    assert run(isolated+credentials,self.env,ok=False).returncode!=0, 'Unauthenticated RPC succeeded'

    def check_no_cert(self):
        context=ssl.create_default_context(cafile=self.phase_dir/'tls/ca.crt')
        no_cert=urllib.request.build_opener(urllib.request.ProxyHandler({}),urllib.request.HTTPSHandler(context=context))
        for service,remote,path in [('pms',6733,'/policy-mgmt/v1/service'),('ads',6734,'/authz-check/v1/is-allowed')]:
            target=self.forward('speedle-prod-'+service,remote)
            url=f'https://127.0.0.1:{target}'+path
            # Establish that this exact transport accepts a trusted client.
            try:self.client(True).open(url,timeout=3).close()
            except urllib.error.HTTPError as error:
                if error.code not in (404,405):raise
            try:no_cert.open(url,timeout=3)
            except urllib.error.HTTPError as error:
                raise AssertionError('Business API completed HTTP without a client certificate') from error
            except (urllib.error.URLError,ssl.SSLError,ConnectionError):pass
            else:raise AssertionError('Business API accepted a missing client certificate')

    def forward(self,service,remote):
        target=port();logpath=self.phase_dir/(service+'-'+str(remote)+'-'+str(target)+'.log');log=logpath.open('w')
        process=subprocess.Popen(['kubectl','--context','kind-'+self.cluster,'--namespace',self.phase['NAMESPACE'],'port-forward','--address','127.0.0.1','svc/'+service,f'{target}:{remote}'],env=self.env,cwd=ROOT,stdout=log,stderr=subprocess.STDOUT,start_new_session=True)
        self.processes.append((process,log))
        def bound():
            if process.poll() is not None:raise RuntimeError('Forwarding process exited: '+logpath.read_text())
            # Opening and closing a bare TCP socket against a TLS service can
            # reset the forwarded stream. Wait for kubectl's listener instead.
            return 'Forwarding from 127.0.0.1:' in logpath.read_text()
        wait(bound,'gRPC forwarding');return target

    def check_backend_failure(self):
        pods=json.loads(self.ns('get','pods','-l','release=speedle-prod','-o','json').stdout)['items']
        before={pod['metadata']['name']:pod['status']['containerStatuses'][0]['restartCount'] for pod in pods}
        self.ns('scale','statefulset/speedle-etcd','--replicas=0')
        self.ns('wait','--for=delete','pod/speedle-etcd-0','--timeout=90s')
        for pod in pods:
            name=pod['metadata']['name'];kind=pod['spec']['containers'][0]['name'];health=8081 if kind=='pms' else 8082
            command=['exec',name,'-c',kind,'--','wget','-q','-T','3','-O','/dev/null',f'http://127.0.0.1:{health}']
            wait(lambda:self.ns(*command[:-1],command[-1]+'/readyz',ok=False).returncode!=0,'readiness fails on backend outage')
            self.ns(*command[:-1],command[-1]+'/livez')
            self.ns('wait','--for=condition=Ready=false','pod/'+name,'--timeout=45s')
            current=json.loads(self.ns('get','pod',name,'-o','json').stdout)
            assert current['status']['containerStatuses'][0]['restartCount']==before[name], 'Backend failure restarted a healthy process'
        self.ns('scale','statefulset/speedle-etcd','--replicas=1')
        self.ns('rollout','status','statefulset/speedle-etcd','--timeout=120s')
        self.ns('rollout','status','deployment/speedle-prod-pms','--timeout=120s')
        self.ns('rollout','status','deployment/speedle-prod-ads','--timeout=120s')
        self.ready(True)
        result=json.loads(self.block('allow','https'));assert result['allowed'] is True, 'PVC-backed policy did not survive etcd replacement'
        print('PASS backend outage: readiness fails, liveness stays healthy, no restart, PVC data recovers',flush=True)

    def check_etcd_auth(self):
        directory=self.phase_dir/'etcd-tls'
        def probe(credentials):
            # A failed TLS handshake may terminate a port-forward stream;
            # give each control its own owned transport to the same backend.
            target=self.forward('speedle-etcd',2379)
            base=[str(self.work/'grpcprobe'),'--service','etcd','--endpoint',f'127.0.0.1:{target}',
                  '--timeout','5s','--cacert',str(directory/'ca.crt'),'--key-name','/'+self.id+'/auth-probe']
            trust=['--cert',str(credentials/'client.crt'),'--key',str(credentials/'client.key')] if credentials else []
            return run(base+trust,self.env,ok=False)
        positive=probe(directory)
        assert positive.returncode==0, 'Trusted etcd client cannot write/read/delete: '+positive.stderr
        for credentials in [None,self.phase_dir/'tls']:
            assert probe(credentials).returncode!=0, 'etcd accepted missing/untrusted client certificate'
        positive=probe(directory)
        assert positive.returncode==0, 'etcd became unavailable during rejection checks: '+positive.stderr
        print('PASS etcd gRPC authentication: trusted write/read/delete succeeds; missing and foreign client certificates rejected',flush=True)

    def check_liveness_failure(self):
        pod=json.loads(self.ns('get','pods','-l','app=pms,release=speedle-prod','-o','json').stdout)['items'][0]
        name=pod['metadata']['name'];restarts=pod['status']['containerStatuses'][0]['restartCount']
        # PID 1 ignores SIGSTOP sent from its own PID namespace. Send it from
        # the owned kind node, after binding the CRI ID/PID to this exact Pod.
        node=pod['spec']['nodeName'];assert node in self.nodes
        container_id=pod['status']['containerStatuses'][0]['containerID'].split('://',1)[1]
        assert re.fullmatch('[0-9a-f]+',container_id)
        info=json.loads(self.docker('exec',node,'crictl','inspect',container_id).stdout)
        assert info['status']['labels']['io.kubernetes.pod.uid']==pod['metadata']['uid']
        pid=int(info['info']['pid']);assert pid>1
        self.docker('exec',node,'kill','-STOP',str(pid))
        def recovered():
            state=json.loads(self.ns('get','pod',name,'-o','json').stdout)['status']['containerStatuses'][0]
            return state['restartCount']>restarts and state.get('ready')
        wait(recovered,'liveness restarts an unresponsive process',timeout=150)
        # A restart terminates old port-forward streams. Start a fresh owned one.
        self.phase['PMS_PORT']=str(self.forward('speedle-prod-pms',6733))
        self.ready(True)
        assert json.loads(self.block('allow','https'))['allowed'] is True
        print('PASS liveness: unresponsive PMS is restarted and stored policy remains usable',flush=True)

    def docker_phase(self,locale):
        self.phase_start(locale,'docker')
        self.networks.append(self.phase['DEMO']);self.volumes.append(self.phase['DEMO']+'-policies')
        self.containers.extend([self.phase['DEMO']+'-pms',self.phase['DEMO']+'-ads'])
        self.block('docker')
        for name in self.containers[-2:]:
            wait(lambda:json.loads(self.docker('inspect',name).stdout)[0]['State'].get('Health',{}).get('Status')=='healthy','Docker HEALTHCHECK')
        self.block('docker-status');self.ready(False);self.policy_checks(False)
        # A private health listener must not be reachable by another container.
        result=self.docker('run','--rm','--network',self.phase['DEMO'],'--entrypoint','wget',self.pms_image,'-q','-T','2','-O','/dev/null',f'http://{self.phase["DEMO"]}-pms:8081/livez',ok=False)
        assert result.returncode!=0, 'Health listener was reachable through the container network'
        self.block('delete','http')
        self.block('docker-cleanup')
        print(f'PASS {locale} Docker guide: shared store, health, CRUD, Allow/Deny, private probe and cleanup',flush=True)

    def kubernetes_phase(self,locale,mode,raw=False):
        label=('raw-' if raw else 'helm-')+mode;self.phase_start(locale,label)
        self.block('namespace')
        if mode=='prod':
            self.block('certificates');self.block('etcd');self.check_etcd_auth()
        self.block(('raw-' if raw else 'helm-')+mode)
        self.block('forward-'+mode,background=True);self.ready(mode=='prod')
        self.policy_checks(mode=='prod')
        if mode=='prod':
            self.check_no_cert()
            pms=self.forward('speedle-prod-pms',50001);ads=self.forward('speedle-prod-ads',50002)
            self.probe_rpc(True,pms,ads)
            if locale=='en' and not raw:
                self.check_backend_failure();self.check_liveness_failure()
        self.block('delete','https' if mode=='prod' else 'http')
        self.block('forward-'+mode+'-cleanup')
        self.block('namespace-cleanup')
        print(f'PASS {locale} {label}: server dry-run/apply or Helm, rollout, probes, CRUD, Allow/Deny, TLS/mTLS and cleanup',flush=True)

    def start(self):
        # Local registry is task-owned and published on an ephemeral loopback port.
        registry=self.id+'-registry';self.containers.append(registry)
        self.docker('run','-d','--name',registry,'--label','io.speedle.validation='+self.id,'--cpus=.5','--memory=256m','-p','127.0.0.1::5000','registry:3.0.0')
        mapping=self.docker('port',registry,'5000/tcp').stdout.strip();registry_port=mapping.rsplit(':',1)[1]
        self.prefix=f'127.0.0.1:{registry_port}/{self.id}'
        self.tag=run(['git','rev-parse','--short','HEAD'],self.env).stdout.strip()
        self.pms_image=self.prefix+'-pms:'+self.tag;self.ads_image=self.prefix+'-ads:'+self.tag
        self.images=[self.pms_image,self.ads_image]
        for locale in ['en','zh']:
            self.phase_start(locale,'images');self.block('build');self.block('push')
        for image in self.images:self.docker('pull',image)
        print('PASS repository-root Docker build, exact tags, registry push and fresh pull',flush=True)
        run(['go','build','-o',str(self.work/'grpcprobe'),'./website/tests/grpcprobe'],self.env)
        for locale in ['en','zh']:self.docker_phase(locale)
        config=self.work/'kind.yaml';config.write_text('kind: Cluster\napiVersion: kind.x-k8s.io/v1alpha4\nnodes:\n- role: control-plane\n')
        self.kind_created=True
        # A host-loopback proxy cannot be reached from a container. Keep this
        # owned cluster on its direct network path, without editing host config.
        cluster_env={key:value for key,value in self.env.items() if key.lower() not in ('http_proxy','https_proxy','all_proxy')}
        run(['kind','create','cluster','--name',self.cluster,'--image',NODE,'--config',str(config),'--kubeconfig',self.env['KUBECONFIG'],'--wait','180s'],cluster_env,timeout=480)
        nodes=run(['kind','get','nodes','--name',self.cluster],self.env).stdout.splitlines()
        self.nodes=nodes
        for node in nodes:
            owner=self.docker('inspect','--format','{{index .Config.Labels "io.x-k8s.kind.cluster"}}',node).stdout.strip()
            assert owner==self.cluster
            self.docker('update','--cpus=2','--memory=5g','--memory-swap=5g',node)
        self.docker('network','connect','kind',registry)
        config_path=f'/etc/containerd/certs.d/127.0.0.1:{registry_port}'
        hosts=f'[host."http://{registry}:5000"]\n  capabilities = ["pull", "resolve"]\n'
        for node in nodes:
            self.docker('exec',node,'mkdir','-p',config_path)
            self.docker('exec','-i',node,'cp','/dev/stdin',config_path+'/hosts.toml',input=hosts)
        # The etcd fixture uses the node's normal registry pull path. Importing
        # a multi-platform Docker image archive can retain absent-platform
        # digests with Docker's containerd image store.
        self.kube('get','nodes')
        # All documented raw resources are regenerated from the same charts.
        rendered=self.work/'rendered'
        run(['bash','deployment/render-manifests.sh',str(rendered)],self.env)
        for mode in ['dev','prod']:
            assert (rendered/f'speedle-{mode}.yaml').read_bytes()==(ROOT/f'deployment/k8s/speedle-{mode}.yaml').read_bytes(),'Generated manifest drift'
        run([sys.executable,'deployment/tests/check-charts.py'],self.env)
        for locale in ['en','zh']:
            for raw in [False,True]:
                for mode in ['dev','prod']:self.kubernetes_phase(locale,mode,raw)
        for locale in ['en','zh']:
            assert self.done[locale]==set(self.commands[locale]),(locale,'unexecuted deployment blocks',set(self.commands[locale])-self.done[locale])
        print('PASS all paired deployment commands, Docker + Helm + independent YAML, both locales',flush=True)

    def cleanup(self):
        for process,log in reversed(self.processes):
            if process.poll() is None:
                with contextlib.suppress(ProcessLookupError):os.killpg(process.pid,signal.SIGTERM)
                try:process.wait(timeout=8)
                except subprocess.TimeoutExpired:
                    with contextlib.suppress(ProcessLookupError):os.killpg(process.pid,signal.SIGKILL)
                    process.wait(timeout=5)
            log.close()
        if self.kind_created:run(['kind','delete','cluster','--name',self.cluster],self.env,timeout=120,ok=False)
        for name in reversed(self.containers):self.docker('rm','-f',name,ok=False)
        for name in reversed(self.volumes):self.docker('volume','rm',name,ok=False)
        for name in reversed(self.networks):self.docker('network','rm',name,ok=False)
        for name in self.images:self.docker('image','rm',name,ok=False)
        # A successful cleanup claim requires checking the daemon, rather than
        # interpreting every failed inspect as an absent resource.
        self.docker('info','--format','{{.OSType}}')
        for args in [('ps','-a','--format','{{.Names}}'),('volume','ls','--format','{{.Name}}'),('network','ls','--format','{{.Name}}')]:
            leftovers=[name for name in self.docker(*args).stdout.splitlines() if name.startswith(self.id)]
            assert not leftovers,('Owned validation resources remain',leftovers)
        print('PASS cleanup: no owned containers, volumes or networks remain',flush=True)


def main():
    def interrupted(_signal,_frame):raise KeyboardInterrupt('Deployment acceptance interrupted')
    signal.signal(signal.SIGTERM,interrupted)
    # A long-running runner may predate its owner's Docker-group membership.
    # sg only activates an existing, administratively granted membership.
    if not os.environ.get('SPEEDLE_DOCKER_GROUP_ACTIVE') and shutil.which('docker'):
        available=run(['docker','info','--format','{{.OSType}}'],ok=False)
        if available.returncode and shutil.which('sg'):
            try:
                group=grp.getgrnam('docker');user=pwd.getpwuid(os.geteuid())
                member=user.pw_name in group.gr_mem or user.pw_gid==group.gr_gid
            except KeyError:member=False
            if member:
                os.environ['SPEEDLE_DOCKER_GROUP_ACTIVE']='1'
                command=shlex.join([sys.executable,str(Path(__file__).resolve())])
                os.execv(shutil.which('sg'),['sg','docker','-c',command])
    run(['bash','deployment/tests/check-environment.sh'])
    # Both runner instances can share one Docker host; serialize only these tests.
    with open('/tmp/speedle-deployment-validation.lock','a') as lock:
        wait(lambda:try_lock(lock),'deployment validation lock',timeout=600)
        with tempfile.TemporaryDirectory(prefix='speedle-deployment-') as temp:
            work=Path(temp);tools=work/'bin'
            run(['bash','deployment/tests/install-tools.sh',str(tools)],timeout=600)
            acceptance=Acceptance(work)
            acceptance.env['PATH']=str(tools)+os.pathsep+acceptance.env['PATH']
            try:acceptance.start()
            except BaseException:
                if acceptance.kind_created and (work/'kubeconfig').exists():
                    for args in [('get','pods','-A','-o','wide'),('get','events','-A','--sort-by=.lastTimestamp')]:
                        print(acceptance.kube(*args,ok=False).stdout[-12000:],flush=True)
                    for log in acceptance.phase_dir.glob('*.log'):
                        print(f'{log.name}:\n{log.read_text()[-4000:]}',flush=True)
                raise
            finally:acceptance.cleanup()

def try_lock(lock):
    try:fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB);return True
    except BlockingIOError:return False

if __name__=='__main__':main()
