#!/usr/bin/env python3
"""Explicit local quality tiers; sustained fuzz/soak never run by default CI."""
import argparse
from concurrent.futures import ThreadPoolExecutor
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import time

ROOT=Path(__file__).resolve().parents[2]
INVENTORY=ROOT/'common/bin-parser/testdata/quality-gates/required-tests.json'
PACKAGE='./common/pcapx/pcaputil'

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('tier',choices=['quick','full','race','sustained'])
    parser.add_argument('--output',required=True,type=Path)
    parser.add_argument('--duration',default='30m',help='Sustained target duration; must be at least 30m')
    parser.add_argument('--jobs',type=int,default=2,help='Parallel sustained subprocesses (each fuzz uses one worker)')
    args=parser.parse_args();out=args.output.resolve();out.mkdir(parents=True,exist_ok=True)
    entries=json.loads(INVENTORY.read_text())['tests'];required=[e for e in entries if args.tier in e['tiers']]
    env=os.environ.copy();env['GOMAXPROCS']='2'
    commands=[]
    if args.tier=='sustained':
        if args.jobs<1 or args.jobs>6:parser.error('jobs must be between 1 and 6')
        units={'h':3600,'m':60,'s':1}
        pieces=re.findall(r'(\d+(?:\.\d+)?)([hms])',args.duration)
        if ''.join(n+u for n,u in pieces)!=args.duration or sum(float(n)*units[u] for n,u in pieces)<1800:
            parser.error('sustained duration must be at least 30m, for example 30m or 1h')
        for entry in required:
            name=entry['test'];command=['go','test','-json',PACKAGE,'-count=1','-run','^$','-timeout','2h']
            child_env=env.copy();child_env['GOMAXPROCS']='1'
            if name.startswith('Fuzz'):
                command+=['-fuzz','^'+name+'$','-fuzztime='+args.duration,'-fuzzminimizetime=100ms','-parallel=1']
            else:
                command[command.index('^$')]='^'+name+'$';child_env['YAK_T06_SOAK_DURATION']=args.duration;child_env['YAK_T06_SOAK_LOG']=str(out/'soak-metrics.jsonl')
            commands.append((name,command,child_env))
    else:
        packages=['./'+p.removeprefix('github.com/yaklang/yaklang/') for p in sorted({e['package'] for e in required})]
        command=['go','test','-json','-count=1','-timeout=10m']
        if args.tier=='quick':command+=['-run','^('+'|'.join(sorted({e['test'] for e in required}))+')$']
        elif args.tier=='full':packages=['./common/bin-parser/...','./common/pcapx/...','./common/yak/cmd/yakcmds/shark-cli','./scripts/ci']
        elif args.tier=='race':
            command[command.index('-timeout=10m')]='-timeout=30m'
            command+=['-race'];packages=['./common/bin-parser/...','./common/pcapx/...','./common/yak/cmd/yakcmds/shark-cli','./scripts/ci']
        commands=[(args.tier,command+packages,env)]
    started=time.time()
    metadata={'tier':args.tier,'git_head':subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip(),'duration':args.duration,'started_unix':started,'inventory_sha256':hashlib.sha256(INVENTORY.read_bytes()).hexdigest(),'commands':[c for _,c,_ in commands]}
    (out/'run.json').write_text(json.dumps(metadata,indent=2)+'\n')
    def run(item):
        name,command,child_env=item;begin=time.time()
        with (out/(name+'.jsonl')).open('w') as log,(out/(name+'.stderr')).open('w') as errors:
            result=subprocess.run(command,cwd=ROOT,env=child_env,stdout=log,stderr=errors)
        return {'name':name,'exit_code':result.returncode,'elapsed_seconds':time.time()-begin}
    with ThreadPoolExecutor(max_workers=args.jobs if args.tier=='sustained' else 1) as pool:
        results=list(pool.map(run,commands))
    combined=out/'combined.jsonl'
    with combined.open('wb') as log:
        for name,_,_ in commands:log.write((out/(name+'.jsonl')).read_bytes())
    checker=subprocess.run(['python3',str(ROOT/'scripts/protocol-tests/check_go_test_json.py'),str(combined),'--inventory',str(INVENTORY),'--tier',args.tier],cwd=ROOT,capture_output=True,text=True)
    (out/'inventory-result.json').write_text(checker.stdout);(out/'inventory.stderr').write_text(checker.stderr)
    metadata.update(results=results,elapsed_seconds=time.time()-started,complete=all(r['exit_code']==0 for r in results) and checker.returncode==0)
    (out/'run.json').write_text(json.dumps(metadata,indent=2)+'\n');print(json.dumps(metadata,indent=2))
    return 0 if metadata['complete'] else 1

if __name__=='__main__':raise SystemExit(main())
