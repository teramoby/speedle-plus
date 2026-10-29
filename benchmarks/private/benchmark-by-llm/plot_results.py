#!/usr/bin/env python3
"""Static report figures. Requires matplotlib; reads one completed run only."""
import pathlib,sys,re,collections,statistics
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
root=pathlib.Path(sys.argv[1]);out=root/'figures';out.mkdir(exist_ok=True)
mixed_only='--mixed-only' in sys.argv[2:]
if not (root/'COMPLETE').exists() and not mixed_only:raise SystemExit('Run incomplete')
rows=collections.defaultdict(list)
for f in root.glob('bench-*.txt'):
 for line in f.read_text().splitlines():
  m=re.match(r'^Benchmark(RBACMixed|ManyUsers)/n=(\d+)/(.+)-2\s+\d+\s+([\d.]+)\s+ns/op',line)
  if m:rows[(m[1],int(m[2]),m[3])].append(float(m[4])/1000)
plt.rcParams.update({'font.size':10,'axes.spines.top':False,'axes.spines.right':False})
colors={'casbin':'#B13F45','casbin-tuned':'#C87B18','opa':'#316DB4','speedle':'#218978'}
fig,axes=plt.subplots(1,2,figsize=(12,5.8),sharey=True,layout='constrained')
for ax,fam,sizes,xlabel in [(axes[0],'RBACMixed',[100,1000,10000,100000],'Roles (10 users per role)'),(axes[1],'ManyUsers',[1000,100000],'Users (10 roles, 100 permissions)')]:
 for engine,color in colors.items():
  vs=[rows[(fam,n,engine)] for n in sizes]
  if any(len(v)!=5 for v in vs):raise SystemExit('Five repetitions required')
  med=[statistics.median(v) for v in vs]
  ax.errorbar(sizes,med,yerr=[[m-min(v) for m,v in zip(med,vs)],[max(v)-m for m,v in zip(med,vs)]],color=color,marker='o',linewidth=1.8,capsize=3,label=engine)
 ax.set_xscale('log');ax.set_yscale('log');ax.set_xlabel(xlabel);ax.set_xticks(sizes,[f'{n:,}' for n in sizes]);ax.grid(True,alpha=.2,which='both')
 ax.set_title('Many roles / growing rules' if fam=='RBACMixed' else 'Few roles / growing users')
axes[0].set_ylabel('Decision time (microseconds, log scale)')
axes[1].legend(loc='upper right',frameon=False)
fig.suptitle('Mixed decisions: median and min–max over five runs\ncharming / i5-7360U / Go 1.27.2 / one goroutine, GOMAXPROCS=2',fontsize=12)
fig.savefig(out/'mixed-latency.png',dpi=180);fig.savefig(out/'mixed-latency.svg');plt.close(fig)
if mixed_only:
 print(out/'mixed-latency.png');sys.exit(0)
http=collections.defaultdict(list)
for line in (root/'stress.md').read_text().splitlines():
 fields=[x.strip() for x in line.strip('|').split('|')]
 if len(fields)==14 and fields[1].isdigit():http[(fields[0],fields[2],int(fields[3]))].append(float(fields[4]))
setups={'thin-casbin':'#B13F45','thin-opa':'#316DB4','thin-speedle':'#218978','opa-server':'#8662AC','speedle-ads':'#A96D29'}
fig,axes=plt.subplots(1,2,figsize=(12,5.8),sharey=True,layout='constrained')
for ax,data in zip(axes,['plain n=10000','manyusers n=100000']):
 for setup,color in setups.items():
  x=[1,16,64];vs=[http[(data,setup,c)] for c in x]
  if any(len(v)!=3 for v in vs):raise SystemExit('Three rounds required')
  med=[statistics.median(v) for v in vs]
  ax.errorbar(x,med,yerr=[[m-min(v) for m,v in zip(med,vs)],[max(v)-m for m,v in zip(med,vs)]],color=color,marker='o',linestyle='--' if setup in ['opa-server','speedle-ads'] else '-',linewidth=1.8,capsize=3,label=setup)
 ax.set_xscale('log',base=2);ax.set_yscale('log');ax.set_xticks(x,[str(c) for c in x]);ax.set_xlabel('Closed-loop concurrency');ax.set_title(data);ax.grid(True,alpha=.2,which='both')
axes[0].set_ylabel('HTTP requests/s (log scale)');axes[1].legend(frameon=False,loc='best')
fig.suptitle('HTTP throughput: median and min–max over three rounds\n15 s measurement / server CPU 0,2 / generator CPU 1,3 / GOMAXPROCS=2 each',fontsize=12)
fig.savefig(out/'http-throughput.png',dpi=180);fig.savefig(out/'http-throughput.svg');plt.close(fig)
print(out)
