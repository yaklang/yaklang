"""Preserve frozen SSH values; validate the existing CLI connection annotation."""
import argparse
import copy
import hashlib
import importlib.util
import json
from pathlib import Path

HERE=Path(__file__).resolve().parent
PHASE=HERE.parent
V2=PHASE/'SSH-SDK-CLI-source49-02'
PLAN=V2/'plan02'
ORIGINAL=PHASE/'SSH-SDK-CLI-source49-01'
REPO=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf-8-sig'))
def put(p,v):
    with p.open('x',encoding='utf8')as f:json.dump(v,f,indent=2,sort_keys=True);f.write('\n')
def module(p,name):
    spec=importlib.util.spec_from_file_location(name,p)
    m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m
def require(v,msg):
    if not v:raise AssertionError(msg)

def patched_old():
    old=module(ORIGINAL/'ssh_native_cli49.py','original49')
    original_verify=old.verify_rows
    def verify(rows,physical,events,CLI):
        byframe={r['frame']:r for r in rows}
        generations={c:i+1 for i,c in enumerate(dict.fromkeys(r['sourcecase']for r in physical))}
        projected=copy.deepcopy(CLI);n=0
        for row in projected:
            expected=byframe.get(row.get('number'))
            if expected is None:continue
            generation=generations[expected['sourcecase']]
            require(row.get('stream_id')==generation,'CLI independently source-ordered connection generation')
            for k,v in {'connection_protocol':'SSH','connection_stack':['SSH'],
                        'connection_evidence':'validated protocol events'}.items():
                require(row.get(k)==v,'CLI source connection '+k)
            suffix=' [connection #'+str(generation)+': SSH]'
            require(row.get('info')==expected['CLI_info']+suffix,
                    'exact source caption plus independently checked connection annotation frame'+str(row['number']))
            row['info']=expected['CLI_info'];n+=1
        require(n==76,'all source payload captions require the checked annotation')
        return {**original_verify(rows,physical,events,projected),'checked_connection_annotations':n}
    old.verify_rows=verify
    return old

def load():
    contract=read(HERE/'contract.json')
    for p,h in contract['inputs'].items():require(sha(p)==h,'annotation contract input changed '+p)
    require(sha(PLAN/'manifest.json')=='ee5e2f1d7daa26c859064f36e83aa5f25c2361e0f8ed8c56f9aae642928f0480','frozen source02')
    v2=module(V2/'ssh_native_cli49_v02.py','supplement49')
    v2.old_module=patched_old
    return v2

def prepare():
    require(not(HERE/'contract.json').exists(),'immutable contract')
    paths=[Path(__file__),V2/'ssh_native_cli49_v02.py',PLAN/'manifest.json',PLAN/'source-rows.json',
           ORIGINAL/'ssh_native_cli49.py',REPO/'common/yak/cmd/yakcmds/shark-cli/protocol.go',
           REPO/'common/yak/cmd/yakcmds/shark-cli/conversation.go']
    put(HERE/'contract.json',{'inputs':{str(p):sha(p)for p in paths},
        'scope':'Only validate existing connection caption/schema, then call original frozen76 and strict11-close checkers.',
        'derivation':'protocol.go applyApplication appends exactly one connection number and SSH stack; full SYN generations in original source determine 1..12 connection order.',
        'known_actual_read':'Only frame4 CLI identification caption was inspected to diagnose the missing connection suffix; no algorithm/Session/GetFields/private alias expected value was changed.',
        'old_failure_preserved':'both independent-audit02 directories; new candidate had passed all76 Session/GetFields before the missing suffix check',
        'not_claimed':'This does not independently check every nonpayload control-frame conversation annotation or SSH field offsets.',
        'source76_rows_SHA256':sha(PLAN/'source-rows.json')})

def selftest(out):
    out.mkdir(exist_ok=False);v2=load();old,rows,physical,closes=v2.load(PLAN)
    _,_,_,api=old.load(v2.OLDPLAN)
    payloads,CLI=old.synthetic(rows,physical,api)
    generations={c:i+1 for i,c in enumerate(dict.fromkeys(r['sourcecase']for r in physical))}
    for r in rows:
        c=CLI[r['frame']-1];g=generations[r['sourcecase']]
        c.update(stream_id=g,connection_protocol='SSH',connection_stack=['SSH'],connection_evidence='validated protocol events')
        c['info']+=' [connection #'+str(g)+': SSH]'
    cases={r['sourcecase']:(e['flow'],e['tcp_flow_id'])for r,e in zip(rows,payloads)}
    extended=[(r['frame'],e)for r,e in zip(rows,payloads)]
    for i,r in enumerate(closes,9001):
        f,t=cases[r['sourcecase']];e=v2.close_event(r,{'id':i,'flow':f,'tcp_flow_id':t})
        extended.append((r['trigger_frame'],e));c=CLI[r['trigger_frame']-1]
        c['published_pdus']=[old.compact_expected(e)];c['published_pdu_scope']=v2.PUBSCOPE
    events=[e for _,e in sorted(extended)]
    counts=v2.verify_all(PLAN,events,CLI);neg=[]
    def reject(label,fn):
        x=copy.deepcopy(CLI);fn(x)
        try:v2.verify_all(PLAN,events,x)
        except(AssertionError,KeyError,ValueError,TypeError):neg.append(label)
        else:raise AssertionError('accepted negative '+label)
    reject('wrong-connection-number',lambda x:x[3].update(info=rows[0]['CLI_info']+' [connection #2: SSH]'))
    reject('missing-annotation',lambda x:x[3].update(info=rows[0]['CLI_info']))
    reject('duplicate-annotation',lambda x:x[3].update(info=x[3]['info']+' [connection #1: SSH]'))
    reject('wrong-packet-caption',lambda x:x[68].update(info='KEXDH_REPLY [connection #6: SSH]'))
    reject('port-hint-instead-of-observation',lambda x:x[3].update(connection_evidence='port hint'))
    reject('invented-nested-protocol',lambda x:x[3].update(connection_stack=['SSH','TLS']))
    reject('reused-generation',lambda x:x[75].update(stream_id=6))
    put(out/'summary.json',{'passed':True,'contract_SHA256':sha(HERE/'contract.json'),'counts':counts,
        'negative_controls':neg,'no_candidate_export_read':True})
    print(json.dumps({'passed':True,'negative_controls':len(neg),'summary_SHA256':sha(out/'summary.json')}))

def verify(events,CLI,descriptors,out):
    v2=load();out.mkdir(exist_ok=False)
    inputs={str(p):sha(p)for p in(events,CLI,descriptors,HERE/'contract.json',PLAN/'manifest.json')}
    differences=[];counts={}
    try:
        old,_,_,_=v2.load(PLAN);_,_,_,api=old.load(v2.OLDPLAN)
        descriptors_n=old.descriptor_check(read(descriptors),api)
        counts=v2.verify_all(PLAN,v2.lines(events),v2.lines(CLI));counts['SSH_descriptor_count']=descriptors_n
    except Exception as e:differences.append({'type':type(e).__name__,'message':str(e)})
    require(inputs=={p:sha(p)for p in inputs},'closed inputs changed')
    put(out/'differences.json',differences)
    put(out/'summary.json',{'passed':not differences,'inputs':inputs,'counts':counts,
        'old_source76_rows_bytes_unchanged':True,'old_source02_close_contract_unchanged':True,
        'differences_SHA256':sha(out/'differences.json')})
    print(json.dumps({'passed':not differences,'summary_SHA256':sha(out/'summary.json'),'differences':len(differences)}))
    if differences:raise SystemExit(1)

def main():
    p=argparse.ArgumentParser();s=p.add_subparsers(dest='cmd',required=True);s.add_parser('prepare')
    q=s.add_parser('selftest');q.add_argument('--out',type=Path,required=True)
    q=s.add_parser('verify')
    for n in('events','CLI','descriptors','out'):q.add_argument('--'+n,type=Path,required=True)
    a=p.parse_args()
    if a.cmd=='prepare':prepare()
    elif a.cmd=='selftest':selftest(a.out)
    else:verify(a.events,a.CLI,a.descriptors,a.out)
if __name__=='__main__':main()
