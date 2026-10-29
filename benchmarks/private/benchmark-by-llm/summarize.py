#!/usr/bin/env python3
"""Summarize only raw results in one explicit run directory."""
import argparse,collections,re,statistics,pathlib,datetime
p=argparse.ArgumentParser();p.add_argument('directory',type=pathlib.Path);a=p.parse_args();root=a.directory
complete=(root/'COMPLETE').exists()
stamp=datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8))).isoformat()
out=['# 原始结果汇总'+('' if complete else '（进行中）'),'','状态：'+('全矩阵完成。' if complete else '测试尚未完成；HTTP 仅汇总已产生的轮次，不作为三轮最终结果。'),f'生成时间：{stamp}',f'来源目录：`{root}`。缺失项目保留为空，不使用其他机器的数据。','']
def read(name):
 f=root/name;return f.read_text(errors='replace') if f.exists() else ''
def heading(s):out.extend([f'## {s}',''])
def tables(text):
 active=False
 for line in text.splitlines():
  if line.startswith('|'):out.append(line);active=True
  elif active:out.append('');active=False
def timing(ns):
 if ns>=1e6:return f'{ns/1e6:.3f} ms'
 if ns>=1e3:return f'{ns/1e3:.3f} µs'
 return f'{ns:.2f} ns'
heading('功能和策略体量')
tables(read('functional.txt'));out.append('')
heading('能力与文件监听')
for name in ['capabilities.txt','watchcheck.txt']:
 out.extend([f'`{name}`','```text',read(name).strip(),'```',''])
heading('加载成本')
tables(read('loadcost.txt'));out.append('')
heading('进程内决策延迟')
rows=collections.defaultdict(list)
pattern=re.compile(r'^(Benchmark[^\s]+)\s+\d+\s+([\d.]+)\s+ns/op\s+([\d.]+)\s+B/op\s+([\d.]+)\s+allocs/op')
for f in sorted(root.glob('bench-*.txt')):
 for line in f.read_text().splitlines():
  m=pattern.match(line)
  if m:rows[m[1]].append(tuple(map(float,m.groups()[1:])))
out.extend(['| benchmark/size/variant-CPU | repeats | median | min | max | median B/op | median allocs/op |','|---|---|---|---|---|---|---|'])
for key,vs in sorted(rows.items()):
 ns=[v[0] for v in vs];med=statistics.median(ns)
 out.append(f'| {key} | {len(vs)} | {timing(med)} | {timing(min(ns))} | {timing(max(ns))} | {statistics.median(v[1] for v in vs):g} | {statistics.median(v[2] for v in vs):g} |')
out.append('');heading('多核吞吐')
out.extend(['| benchmark | decisions/s (median ns/op converted) |','|---|---|'])
for key,vs in sorted(rows.items()):
 if key.startswith('BenchmarkParallel'):out.append(f'| {key} | {1e9/statistics.median(v[0] for v in vs):,.0f} |')
out.append('');heading('HTTP 压测：三轮中位数' if complete else 'HTTP 压测：已完成轮次的中位数（进行中）')
groups=collections.defaultdict(list)
for line in read('stress.md').splitlines():
 fields=[x.strip() for x in line.strip('|').split('|')]
 if len(fields)!=14 or not fields[1].isdigit():continue
 groups[(fields[0],fields[2],fields[3])].append(fields[4:])
out.extend(['| dataset | setup | conc | rounds | req/s | p50 µs | p90 µs | p99 µs | p99.9 µs | max µs | errors (sum) | wrong (sum) | srv cores | gen cores |','|---|---|---|---|---|---|---|---|---|---|---|---|---|---|'])
for (data,setup,c),vs in sorted(groups.items()):
 values=[]
 for i in range(10):
  nums=[float(v[i]) for v in vs if v[i]!='-']
  value=(sum(nums) if i in [6,7] else statistics.median(nums)) if nums else None
  values.append('-' if value is None else f'{value:.3f}' if i>=8 else f'{value:g}')
 out.append('| '+' | '.join([data,setup,c,str(len(vs))]+values)+' |')
out.append('');heading('执行与干扰记录')
out.extend(['```text',read('run-all.log').strip(),'```',''])
activity=read('activity.log').splitlines()
loads=[];workers=[];swapin=[];swapout=[]
for line in activity:
 m=re.search(r'load=([\d.]+) workers=(\d+) pswpin=(\d+) pswpout=(\d+)',line)
 if m:loads.append(float(m[1]));workers.append(int(m[2]));swapin.append(int(m[3]));swapout.append(int(m[4]))
if loads:out.extend([f'活动采样 {len(loads)} 行；1 分钟负载范围 {min(loads):.2f}–{max(loads):.2f}；CI Worker 最大值 {max(workers)}；换入/换出页增量 {max(swapin)-min(swapin)} / {max(swapout)-min(swapout)}。',''])
(root/'summary.md').write_text('\n'.join(out)+'\n');print(root/'summary.md')
