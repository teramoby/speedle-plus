#!/usr/bin/env python3
"""Read-only checks for this fixed benchmark archive; never starts a service."""
from pathlib import Path
import collections,hashlib,json,re,tarfile
root=Path(__file__).resolve().parent
manifest=json.loads((root/'MANIFEST.json').read_text())
for entry in manifest['files']:
 path=root/entry['path'];assert path.is_file(),entry['path']
 assert hashlib.sha256(path.read_bytes()).hexdigest()==entry['sha256'],entry['path']
 assert path.stat().st_size==entry['bytes'],entry['path']
raw=root/'results/retest-20261029/charming'
assert (raw/'COMPLETE').is_file()
counts=collections.Counter()
for path in raw.glob('bench-*.txt'):
 text=path.read_text();assert text.strip().endswith('PASS'),path.name
 for line in text.splitlines():
  m=re.match(r'^(Benchmark\S+)\s+\d+\s+[\d.]+\s+ns/op\s+[\d.]+\s+B/op\s+[\d.]+\s+allocs/op',line)
  if m:counts[m[1]]+=1
assert len(counts)==107 and set(counts.values())=={5},'benchmark repetitions'
rows=[]
for line in (raw/'stress.md').read_text().splitlines():
 f=[x.strip() for x in line.strip('|').split('|')]
 if len(f)==14 and f[1].isdigit():rows.append(f)
actual={(f[0],int(f[1]),f[2],int(f[3])) for f in rows}
expected={(d,r,s,c) for d in ['plain n=10000','manyusers n=100000'] for r in [1,2,3] for s in ['thin-casbin','thin-opa','thin-speedle','opa-server','speedle-ads'] for c in [1,16,64]}
assert len(rows)==90 and actual==expected,'HTTP matrix'
assert all(int(f[10])==0 and int(f[11])==0 for f in rows),'HTTP errors or wrong decisions'
assert all(float(f[12])>=0 and float(f[13])>=0 for f in rows),'CPU accounting'
steps=[line.split('\t') for line in (raw/'steps.tsv').read_text().splitlines()]
assert len(steps)==13 and all(s[1]=='1' and s[4:] == ['0','clean'] for s in steps),'step status'
activity=(raw/'activity.log').read_text().splitlines()
assert len(activity)==734 and all('workers=0 ' in line for line in activity),'activity sampling'
meta=json.loads((root/'results/retest-20261029/final-remote-verification.json').read_text())
assert not meta['remaining_test_processes'] and not meta['remaining_test_listeners']
for name,digest in meta['hashes'].items():assert digest in (raw/'machine.txt').read_text(),name
rebuild=json.loads((root/'results/retest-20261029/reproduction-binary-check.json').read_text())
assert len(rebuild)==6 and all(x['identical'] and x['original_sha256']==x['rebuilt_sha256']==meta['hashes'][x['program']] for x in rebuild)
checked=0
with tarfile.open(root/'provenance/executed-source.tar.gz') as tar:
 for member in tar.getmembers():
  if not member.isfile():continue
  path=Path(member.name)
  assert not path.is_absolute() and '..' not in path.parts
  if member.name.endswith('.go') or member.name in ['go.mod','go.sum','run-all.sh','stress.sh']:
   assert tar.extractfile(member).read()==(root/member.name).read_bytes(),member.name
   checked+=1
assert checked==15,'executed runtime source count'
entry_hashes=json.loads((root/'provenance/archive-entry-hashes.json').read_text())
archive_members=0
for archive in ['provenance/executed-source.tar.gz','provenance/before-preparation-HISTORICAL.tar.gz']:
 with tarfile.open(root/archive) as tar:
  for member in tar.getmembers():
   path=Path(member.name)
   assert member.isfile() and not path.is_absolute() and '..' not in path.parts
   assert member.uid==member.gid==0 and not member.uname and not member.gname
   data=tar.extractfile(member).read()
   entry=next(e for e in entry_hashes if e['archive']==archive and e['path']==member.name)
   assert hashlib.sha256(data).hexdigest()==entry['published_sha256']
   archive_members+=1
assert archive_members==len(entry_hashes)

for md in root.glob('*.md'):
 for target in re.findall(r'\]\(([^)]+)\)',md.read_text()):
  if '://' in target or target.startswith('#'):continue
  assert (md.parent/target.split('#',1)[0]).exists(),f'{md.name}: {target}'
patterns=[re.compile(rb'-----BEGIN (?:OPENSSH |RSA |EC |DSA )?PRIVATE KEY-----'),re.compile(rb'\bgh[pousr]_[A-Za-z0-9_]{30,}\b'),re.compile(rb'\bgithub_pat_[A-Za-z0-9_]{40,}\b'),re.compile(rb'\b(?:AKIA|ASIA)[A-Z0-9]{16}\b')]
for entry in manifest['files']:
 path=root/entry['path'];b=path.read_bytes()
 if b'\0' in b:continue
 for account in re.findall(rb'/(?:Users|home)/([A-Za-z0-9_.-]+)',b):
  assert account in [b'LOCAL_USER',b'REMOTE_USER'],f'account path: {path.relative_to(root)}'
 assert not any(pattern.search(b) for pattern in patterns),f'credential pattern: {path.relative_to(root)}'
print(f'PASS: {len(manifest["files"])} file hashes; 107 benchmark groups x5; 90 HTTP windows; 13 clean steps; 734 CI samples; 6 binary hashes; 15 executed source files; links and high-confidence credential scan')
