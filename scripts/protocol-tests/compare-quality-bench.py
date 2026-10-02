#!/usr/bin/env python3
"""Five alternating A/B rounds on identical harness/corpus; preserve all results."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import statistics
import subprocess
import time

HARNESS=['common/pcapx/pcaputil/first_batch_t06_quality_test.go','common/pcapx/pcaputil/protocol_native_dns_doh_test.go']

def fingerprints(root):
    paths=[root/name for name in HARNESS]+sorted((root/'common/bin-parser/testdata/protocol-native/dns-doh').iterdir())
    return {str(p.relative_to(root)):hashlib.sha256(p.read_bytes()).hexdigest() for p in paths if p.is_file()}

def benchmark_rows(log):
    rows={}
    for line in log.splitlines():
        if not line.startswith('BenchmarkT06'):
            continue
        cells=line.split()
        if len(cells)<4 or not cells[1].isdigit():
            continue
        metrics={}
        for i in range(2,len(cells)-1,2):
            try:metrics[cells[i+1]]=float(cells[i])
            except ValueError:pass
        rows[cells[0]]=metrics
    return rows

def evidence_rows(path):
    rows={}
    for line in path.read_text().splitlines():
        e=json.loads(line);rows[e['benchmark']+'/procs'+str(e['gomaxprocs'])]=e
    return rows

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('base',type=Path);parser.add_argument('head',type=Path)
    parser.add_argument('--output',type=Path,required=True);parser.add_argument('--benchtime',default='200ms')
    parser.add_argument('--base-sha',required=True);parser.add_argument('--head-sha',required=True)
    args=parser.parse_args();base=args.base.resolve();head=args.head.resolve();out=args.output.resolve();out.mkdir(parents=True,exist_ok=True)
    if fingerprints(base)!=fingerprints(head):parser.error('A/B must use byte-identical harness and corpus')
    meta={'started_unix':time.time(),'platform':platform.platform(),'machine':platform.machine(),'go':subprocess.check_output(['go','version'],text=True).strip(),'base':str(base),'head':str(head),'base_sha':args.base_sha,'head_sha':args.head_sha,'inputs':fingerprints(head),'round_order':[],'verification_in_timing':True}
    meta['logical_cpus']=os.cpu_count()
    if platform.system()=='Darwin':
        meta['hardware']={key:subprocess.check_output(['sysctl','-n',key],text=True).strip() for key in ['machdep.cpu.brand_string','hw.physicalcpu','hw.memsize']}
    (out/'environment.json').write_text(json.dumps(meta,indent=2)+'\n')
    arm_rows={'base':[],'head':[]};all_evidence=[]
    command=['go','test','./common/pcapx/pcaputil','-run','^$','-bench','^BenchmarkT06','-benchtime='+args.benchtime,'-count=1','-cpu=1,2,4','-timeout=20m']
    for round_no in range(1,6):
        pair={}
        for arm in (['base','head'] if round_no%2 else ['head','base']):
            evidence=out/f'round{round_no}-{arm}-oracles.jsonl';evidence.unlink(missing_ok=True)
            env=os.environ.copy();env['GOMAXPROCS']='2';env['YAK_T06_BENCH_EVIDENCE']=str(evidence)
            path=out/f'round{round_no}-{arm}.log';begin=time.time()
            with path.open('w') as log:result=subprocess.run(command,cwd=base if arm=='base' else head,env=env,stdout=log,stderr=subprocess.STDOUT)
            meta['round_order'].append({'round':round_no,'arm':arm,'exit_code':result.returncode,'elapsed_seconds':time.time()-begin,'command':command})
            (out/'environment.json').write_text(json.dumps(meta,indent=2)+'\n')
            if result.returncode:raise SystemExit('benchmark failed: '+str(path))
            rows=benchmark_rows(path.read_text());assert rows,'empty benchmark output'
            arm_rows[arm].append(rows);pair[arm]=evidence_rows(evidence)
        if pair['base'].keys()!=pair['head'].keys():raise SystemExit('semantic evidence key mismatch')
        for name in pair['base']:
            a,b=pair['base'][name]['result'],pair['head'][name]['result']
            for key in ['Messages','Bytes','Opaque','Unknown','Limited','Drops','Envelopes','Digest','FieldDigest','OrderedDigest']:
                if a[key]!=b[key]:raise SystemExit(f'A/B semantic mismatch: {name} {key}')
        all_evidence.append({'round':round_no,'case_count':len(pair['base']),'semantics_equal':True})
    keys=set(arm_rows['base'][0])
    if any(set(rows)!=keys for arm in arm_rows.values() for rows in arm):raise SystemExit('benchmark case mismatch')
    comparisons=[]
    for name in sorted(keys):
        metrics={}
        for metric in arm_rows['base'][0][name]:
            a=[r[name][metric] for r in arm_rows['base']];b=[r[name][metric] for r in arm_rows['head']]
            am,bm=statistics.median(a),statistics.median(b)
            metrics[metric]={'base':a,'head':b,'base_median':am,'head_median':bm,'change_percent':100*(bm/am-1) if am else None,'base_range':[min(a),max(a)],'head_range':[min(b),max(b)]}
        comparisons.append({'benchmark':name,'metrics':metrics})
    (out/'comparison.json').write_text(json.dumps({'rounds':5,'oracle_equivalence':all_evidence,'comparisons':comparisons},indent=2)+'\n')
    meta['completed_unix']=time.time();meta['complete']=True;(out/'environment.json').write_text(json.dumps(meta,indent=2)+'\n')
    print(json.dumps({'complete':True,'rounds':5,'benchmark_cases':len(comparisons),'oracle_equivalence':all_evidence},indent=2))

if __name__=='__main__':main()
