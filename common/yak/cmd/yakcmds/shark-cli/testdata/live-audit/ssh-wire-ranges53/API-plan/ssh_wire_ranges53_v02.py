"""SSH source51 ranges -> source49/50 SDK/CLI contract; no candidate oracle.
All execution is pure Python. Old fact files and checkers remain unchanged.
"""
from pathlib import Path
import argparse,base64,copy,hashlib,importlib.util,json
P=Path(__file__).resolve().parent.parent
HERE=Path(__file__).resolve().parent
L=P/'SSH-wire-ranges-source51-01'
S49=P/'SSH-SDK-CLI-source49-03'
S50=P/'SSH-role-API-CLI-source50-03'
F49=P/'SSH-SDK-CLI-source49-01/plan01/source-rows.json'
F50=S50/'plan03/source-rows.json'
PINS={L/'manifest.json':'8739f8fed549047f117efbda6b38c0a8914f6d94cf2269a5e8c658fac28f6de3',F49:'d9421805893b738fa47d95b6baa177cc907df37475e2dae30c90dd45547dcf73',F50:'fbbcda79bd880324bfc541649f271580b626a764e88af14cd554e39eee5eac77',S49/'contract.json':'5de2a91f843b8c3871df54f4dfce1a50c9ff7a0157e84334f5ae7549670dc0c7',S50/'plan03/manifest.json':'5492fa27bcd060eb22d82163aec99349336be3a9e0af9b2822f7aa5e3269455f'}
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text(encoding='utf-8-sig'))
def put(p,x):
    with Path(p).open('x',encoding='utf-8')as f:json.dump(x,f,indent=2,sort_keys=True);f.write('\n')
def strict(a,b):return json.dumps(a,sort_keys=True,separators=(',',':'))==json.dumps(b,sort_keys=True,separators=(',',':'))
def demand(v,msg):
    if not v:raise AssertionError(msg)
def module(path,name):
    spec=importlib.util.spec_from_file_location(name,path);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m
def validate_frozen():
    for p,h in PINS.items():demand(sha(p)==h,'frozen source dependency '+str(p))
    for a in read(L/'manifest.json')['artifacts']:demand(sha(a['path'])==a['SHA256'],'source51 artifact')
    return {(r['dataset'],r['frame']):r for r in read(L/'range-plan.json')}
def at_path(tree,path):
    for key in path:tree=tree[key]
    return tree
def without_ranges(x):
    if isinstance(x,dict):return{k:without_ranges(v)for k,v in x.items()if k!='Wire Field Ranges'and k!='pdu_range'}
    if isinstance(x,list):return[without_ranges(v)for v in x]
    return x

def tree_with_ranges(tree,spans):
    demand(isinstance(tree,dict),'range tree must have real fields')
    out=copy.deepcopy(tree)
    def own(x):
        for v in x.values():
            if isinstance(v,dict):own(v)
        demand('Wire Field Ranges'not in x,'old fields already have unexpected range metadata')
        x['Wire Field Ranges']={}
    own(out)
    for path,span in spans.items():
        parts=path.split('/');owner=at_path(out,parts[:-1]);demand(parts[-1]in owner,'source leaf path exists')
        owner['Wire Field Ranges'][parts[-1]]=copy.deepcopy(span)
    demand(strict(without_ranges(out),tree),'range graft changed an old field')
    return out

def augment(dataset,rows,ledger):
    original={r['frame']:r for r in read(F49 if dataset=='source49'else F50)}
    out=[]
    for r in rows:
        old=original[r['frame']];g=ledger[(dataset,r['frame'])]
        demand(g['sourcecase']==old['sourcecase']and g['raw_SHA256']==old['raw_SHA256'],'sourcecase/frame/full raw identity')
        demand(strict(r['fields'],old['fields'])and strict(r['session'],old['session'])and strict(r['private_aliases'],old['private_aliases']),'legacy runtime metadata may not alter frozen fields')
        demand(sha(old['raw_path'])==g['raw_SHA256'],'original full raw source')
        q=copy.deepcopy(r)
        if r['fields']is not None:q['fields']=tree_with_ranges(r['fields'],g['API_fields_ranges'])
        else:demand(not g['API_fields_ranges'],'refusal/opaque cannot gain fields')
        if r['session']is not None:
            q['session']['Wire Field Ranges']=copy.deepcopy(g['Session_current_literal_ranges'])
            for k in g['Session_current_literal_ranges']:demand(k in r['session'],'Session range requires its current literal field '+k)
            if 'Initial Packet Fields'in r['session']:
                q['session']['Initial Packet Fields']=tree_with_ranges(r['session']['Initial Packet Fields'],g['API_fields_ranges'])
        else:demand(not g['Session_current_literal_ranges'],'opaque cannot gain Session fields')
        aliases=set()
        for v in q['private_aliases']:
            span=g['private_alias_ranges'].get(v['name'])
            if span is not None:
                demand(v['origin']=='pdu','context/derived alias must not have a PDU range')
                demand(v['name']not in aliases,'one source alias occurrence per packet');aliases.add(v['name'])
                v['pdu_range']={'offset':span[0],'length':span[1]-span[0]}
        demand(aliases==set(g['private_alias_ranges']),'all source-proven pdu aliases present')
        demand(strict(without_ranges(q),r),'all old nonrange row values must remain literal')
        out.append(q)
    return out

# Load immutable legacy wrappers, changing only their in-memory expected rows.
# Every original metadata/Raw/SourceBytes/EOF/role/CLI/schema check still runs.
def engine(dataset,plan=None):
    ledger=validate_frozen()
    if dataset=='source49':
        ctl=module(S49/'ssh_native_cli49_v03.py','frozen49_annotation53');v=ctl.load();frozen=v.load
        def projected(p):
            old,rows,physical,closes=frozen(p);updated=augment(dataset,rows,ledger)
            if plan is not None:demand(strict(updated,read(plan/'source49-rows.json')),'frozen phase53 source49 rows')
            return old,updated,physical,closes
        v.load=projected
        def synthetic():
            old,rows,physical,closes=v.load(ctl.PLAN);_,_,_,api=old.load(v.OLDPLAN)
            payload,cli=old.synthetic(rows,physical,api)
            generation={c:i+1 for i,c in enumerate(dict.fromkeys(r['sourcecase']for r in physical))}
            for r in rows:
                c=cli[r['frame']-1];g=generation[r['sourcecase']]
                c.update(stream_id=g,connection_protocol='SSH',connection_stack=['SSH'],connection_evidence='validated protocol events');c['info']+=' [connection #'+str(g)+': SSH]'
            identities={r['sourcecase']:(e['flow'],e['tcp_flow_id'])for r,e in zip(rows,payload)}
            extended=[(r['frame'],e)for r,e in zip(rows,payload)]
            for i,r in enumerate(closes,9001):
                f,t=identities[r['sourcecase']];e=v.close_event(r,{'id':i,'flow':f,'tcp_flow_id':t});extended.append((r['trigger_frame'],e))
                cli[r['trigger_frame']-1].update(published_pdus=[old.compact_expected(e)],published_pdu_scope=v.PUBSCOPE)
            return[e for _,e in sorted(extended)],cli
        return {'load':lambda:v.load(ctl.PLAN),'verify':lambda e,c:v.verify_all(ctl.PLAN,e,c),'synthetic':synthetic}
    demand(dataset=='source50','known dataset')
    ctl=module(S50/'ssh_role_native_cli50_v03.py','frozen50_metadata53');v2=ctl.load(S50/'plan03');base=v2.load(S50/'plan03');frozen=base.load
    def projected(p):
        old,eof,api,rows,physical,closes=frozen(p);updated=augment(dataset,rows,ledger)
        if plan is not None:demand(strict(updated,read(plan/'source50-rows.json')),'frozen phase53 source50 rows')
        return old,eof,api,updated,physical,closes
    base.load=projected
    return {'load':lambda:base.load(S50/'plan03'),'verify':lambda e,c:base.verify_all(S50/'plan03',e,c),'synthetic':lambda:base.synthetic(S50/'plan03')}

def rows_of(dataset,e):return e['load']()[1]if dataset=='source49'else e['load']()[3]
def capture_for(dataset):
    p=P/('SSH-SDK-CLI-source49-01/plan01/runtime-command-plan.json'if dataset=='source49'else'SSH-role-API-CLI-source50-03/plan03/runtime-command-plan.json')
    x=read(p)
    # Both existing source plans identify an external capture; no fresh input.
    if dataset=='source49':return Path(x['capture']),p
    manifest=read(P/'SSH-role-source50-01/plan02/manifest.json')
    cap=Path(manifest['capture']['path']);demand(sha(cap)==manifest['capture']['SHA256'],'source50 original capture')
    return cap,p

def range_entries(x):
    if isinstance(x,dict):return len(x.get('Wire Field Ranges',{}))+sum(range_entries(v)for k,v in x.items()if k!='Wire Field Ranges')
    if isinstance(x,list):return sum(range_entries(v)for v in x)
    return 0

def prepare(out):
    out.mkdir(exist_ok=False);ledger=validate_frozen();bridges=[];counts={};commands={}
    for ds in('source49','source50'):
        e=engine(ds);rows=rows_of(ds,e);put(out/(ds+'-rows.json'),rows)
        for r in rows:
            # The legacy summary and preview projections remain untouched.
            normalized=without_ranges(r);g=ledger[(ds,r['frame'])]
            bridges.append({'dataset':ds,'sourcecase':r['sourcecase'],'frame':r['frame'],'original_full_raw_SHA256':g['raw_SHA256'],'legacy_runtime_nonrange_SHA256':hashlib.sha256(json.dumps(normalized,sort_keys=True,separators=(',',':')).encode()).hexdigest(),'strip_only_verified_range_metadata_restores_legacy':True})
        counts[ds]={'payload_rows':len(rows),'GetFields_range_entries':sum(range_entries(r['fields'])for r in rows),'Session_range_entries_with_duplicate_initial_tree':sum(range_entries(r['session'])for r in rows),'private_alias_ranges':sum('pdu_range'in v for r in rows for v in r['private_aliases']),'legacy_API_wire_literals':sum(len(r['API_wire_leaves'])for r in rows),'old_nonrange_rows_preserved':True}
        cap,oldcmd=capture_for(ds);commands[ds]={'capture':str(cap),'capture_SHA256':sha(cap),'old_runtime_plan':str(oldcmd),'old_runtime_plan_SHA256':sha(oldcmd),'events_args':['replay',str(cap),'-','--ignore-embedded-secrets'],'CLI_args':['shark','-r',str(cap),'--json','--ignore-embedded-secrets'],'descriptor_args':['shark','--list-fields','--json']}
    put(out/'nonrange-bridge.json',bridges);put(out/'runtime-command-plan.json',commands)
    put(out/'range-contract.json',{'schema':'metadata addition only; all nonWR values unchanged','WR':'each owner map contains only direct current literal leaves; [lo,hi) in logical-PDU bytes as JSON integers','Session':'current Message Number including role refusal; KEXINIT own andOffered-renamed lists/languages/follows/reserved; current banner Identification/Version; successful selected reply Host Key Format only','Session_root_excludes':'GetFields root PacketLength/Padding ranges; rendering combines root sources separately','typed':'only existing three origin=pdu SSH aliases get pdu_range; context/derived origin unchanged','refusal':'GetFields remainsnil/error; no successful generic fields','opaque':'postNEWKEYS preview andFIN carry no fabricated fields/ranges','CLI':'original physical metadata/captions/scopes/connection annotations andwhole compact PDU+snapshot/FIN bridges remain strict; no actual Windows range rendering proof','no_candidate_oracle':True})
    inputs={str(p):sha(p)for p in PINS};inputs[str(Path(__file__))]=sha(__file__)
    for p in out.iterdir():
        if p.is_file():inputs[str(p)]=sha(p)
    for ds in('source49','source50'):
        cap,oldcmd=capture_for(ds);inputs[str(cap)]=sha(cap);inputs[str(oldcmd)]=sha(oldcmd)
    put(out/'manifest.json',{'schema':'SSH-wire-ranges-source53-plan01','inputs':inputs,'source51_manifest_SHA256':sha(L/'manifest.json'),'old_source_rows_unchanged':True,'counts':counts,'expected_not_from_candidate_outputs':True,'comparison':'complete original field/Session/snapshot checks with source-derived additive ranges, not blanket stripping or changed-path allowlist'})
    print(json.dumps({'plan':str(out),'SHA256':sha(out/'manifest.json'),'counts':counts}))

def check_plan(plan):
    m=read(plan/'manifest.json')
    for p,h in m['inputs'].items():demand(sha(p)==h,'phase53 immutable dependency '+p)
    validate_frozen();return m

def selftest(plan,out):
    out.mkdir(exist_ok=False);m=check_plan(plan);reports={}
    for ds in('source49','source50'):
        e=engine(ds,plan);events,cli=e['synthetic']();positive=e['verify'](events,cli);negative=[]
        byframe=lambda a,n:next(x for x in a if x.get('raw')and x['source_bytes']['PacketRefs'][0]['Number']==n)
        binary=next(x for x in events if x.get('fields')and'Payload'in x['fields'])
        kex=next(x for x in events if x.get('session')and'Kex Algorithms'in x['session'])
        reply=next(x for x in events if x.get('session')and'Host Key Format'in x['session'])
        unknown=next(x for x in events if x.get('fields')and'Payload'in x['fields']and'Opaque Message Data'in x['fields']['Payload'])
        protected=next(x for x in events if x.get('expert')=='Encrypted')
        fid=lambda x:x['source_bytes']['PacketRefs'][0]['Number']
        def bad(name,fn):
            a,b=copy.deepcopy(events),copy.deepcopy(cli);fn(a,b)
            try:e['verify'](a,b)
            except(AssertionError,KeyError,TypeError,ValueError):negative.append(name);return
            raise AssertionError('accepted range/semantic mutation '+ds+'/'+name)
        bad('missing-root-WR',lambda a,b:byframe(a,fid(binary))['fields'].pop('Wire Field Ranges'))
        bad('packet-length-off-by-one',lambda a,b:byframe(a,fid(binary))['fields']['Wire Field Ranges'].update({'Packet Length':[1,5]}))
        bad('nested-cookie-borrows-root-owner',lambda a,b:byframe(a,fid(kex))['fields']['Wire Field Ranges'].update({'Cookie':[6,22]}))
        bad('same-value-compression-owner',lambda a,b:byframe(a,fid(kex))['fields']['Payload']['Wire Field Ranges'].update({'Compression S2C':byframe(a,fid(kex))['fields']['Payload']['Wire Field Ranges']['Compression C2S']}))
        bad('zero-width-language-invented-byte',lambda a,b:byframe(a,fid(kex))['fields']['Payload']['Wire Field Ranges']['Languages C2S'].__setitem__(1,byframe(a,fid(kex))['fields']['Payload']['Wire Field Ranges']['Languages C2S'][0]+1))
        bad('current-Session-selected-cipher-borrows-offer',lambda a,b:byframe(a,fid(kex))['session']['Wire Field Ranges'].update({'Encryption C2S':[26,27]}))
        bad('carried-banner-in-current-message',lambda a,b:byframe(a,fid(binary))['session']['Wire Field Ranges'].update({'Identification':[0,5]}))
        bad('derived-fingerprint-span',lambda a,b:byframe(a,fid(reply))['session']['Wire Field Ranges'].update({'Host Key Fingerprint':[0,32]}))
        bad('selected-reply-format-wrong-owner',lambda a,b:byframe(a,fid(reply))['session']['Wire Field Ranges'].pop('Host Key Format'))
        bad('unknown-method-standard-hostkey',lambda a,b:byframe(a,fid(unknown))['fields']['Payload'].update({'Host Key':{'Host Key Format':'ssh-ed25519','Wire Field Ranges':{'Host Key Format':[10,21]}}}))
        bad('protected-clear-prefix-fields',lambda a,b:byframe(a,fid(protected)).update(fields={'Packet Length':1,'Wire Field Ranges':{'Packet Length':[0,4]}}))
        bad('private-context-alias-range',lambda a,b:next(v for v in byframe(a,fid(binary))['field_values']['values']if v['name']=='ssh.protocol').update(pdu_range={'offset':0,'length':1}))
        bad('private-message-range-missing',lambda a,b:next(v for v in byframe(a,fid(binary))['field_values']['values']if v['name']=='ssh.message_code').pop('pdu_range'))
        bad('private-range-wrong-type',lambda a,b:next(v for v in byframe(a,fid(binary))['field_values']['values']if v['name']=='ssh.message_code')['pdu_range'].update(offset=5.0))
        bad('old-nonWR-cookie-value-still-strict',lambda a,b:byframe(a,fid(kex))['fields']['Payload'].update(Cookie='AA=='))
        bad('old-context-value-still-strict',lambda a,b:byframe(a,fid(binary))['session'].update(Identification='forged'))
        bad('CLI-range-not-SDK-range',lambda a,b:next(v for v in b[fid(binary)-1]['pdus'][0]['field_values']['values']if v['name']=='ssh.message_code')['pdu_range'].update(offset=0))
        bad('physical-frame-length-still-strict',lambda a,b:b[0].update(length=b[0]['length']+1))
        if ds=='source50':
            refusal=next(x for x in events if x.get('status')=='malformed')
            bad('role-refusal-generic-success',lambda a,b:byframe(a,fid(refusal)).update(fields={'Wire Field Ranges':{'Message Number':[5,6]},'Message Number':31}))
            bad('opaque-preview64-policy-stays',lambda a,b:byframe(a,13)['source_bytes'].update(SHA256='0'*64))
        reports[ds]={'old_strict_counts':positive,'range_counts':m['counts'][ds],'negative_controls_rejected':negative}
    put(out/'summary.json',{'passed':True,'plan_SHA256':sha(plan/'manifest.json'),'checker_SHA256':sha(__file__),'datasets':reports,'candidate_exports_read':False,'Go_SDK_TS_processes':0})
    print(json.dumps({'passed':True,'summary_SHA256':sha(out/'summary.json'),'negative_controls':{k:len(v['negative_controls_rejected'])for k,v in reports.items()}}))

def producer(path,export,expected_args,source,cap):
    r=read(path);demand(r['exit_code']==0 and r['source']==source,'closed correct-source producer')
    demand(r['export_SHA256']==sha(export),'producer exact export hash')
    demand(r['arguments'][1:]==expected_args,'default source-command arguments')
    for p,h in r['inputs'].items():demand(sha(p)==h,'producer frozen input '+p)
    demand(any(Path(p).resolve()==cap.resolve()and h==sha(cap)for p,h in r['inputs'].items()),'full physical PCAP provenance')
    binary=Path(r['arguments'][0]);demand(str(binary)in r['inputs'],'binary hash inproducer')
    build=binary.with_name(binary.stem+'-receipt.json');demand(str(build)in r['inputs'],'closed build receipt inproducer')
    b=read(build);demand(b['source']==source and b['exit_code']==0 and b['binary_SHA256']==sha(binary),'binary closes exact source build')
    return{'path':str(path),'SHA256':sha(path),'source':source,'export_SHA256':sha(export),'capture_SHA256':sha(cap),'binary_SHA256':sha(binary)}

def verify(plan,dataset,events,cli,descriptors,source,out,events_producer=None,CLI_producer=None,descriptors_producer=None):
    out.mkdir(exist_ok=False);m=check_plan(plan);failure=[];counts={};bindings=[]
    inputs={str(p):sha(p)for p in(plan/'manifest.json',events,cli,descriptors)}
    commands=read(plan/'runtime-command-plan.json')[dataset];cap=Path(commands['capture'])
    try:
        for path,export,args in[(events_producer or events.with_name('events-producer.json'),events,commands['events_args']),(CLI_producer or cli.with_name('yak-producer.json'),cli,commands['CLI_args']),(descriptors_producer or descriptors.with_name('field-descriptors-producer.json'),descriptors,commands['descriptor_args'])]:bindings.append(producer(path,export,args,source,cap))
        e=engine(dataset,plan);loaded=e['load']();old=loaded[0]
        if dataset=='source49':_,_,_,api=old.load(P/'SSH-SDK-CLI-source49-01/plan01')
        else:api=loaded[2]
        n=old.descriptor_check(read(descriptors),api)
        rows=lambda p:[json.loads(x)for x in p.read_text(encoding='utf-8-sig').splitlines()if x.strip()]
        counts=e['verify'](rows(events),rows(cli));counts.update(SSH_descriptors=n,source_range_counts=m['counts'][dataset])
    except Exception as err:failure.append({'type':type(err).__name__,'message':str(err)})
    demand(inputs=={p:sha(p)for p in inputs},'closed actual inputs changed')
    put(out/'differences.json',failure);put(out/'summary.json',{'passed':not failure,'dataset':dataset,'source':source,'inputs':inputs,'plan_SHA256':sha(plan/'manifest.json'),'counts':counts,'capture_producer_bindings':bindings,'differences_SHA256':sha(out/'differences.json'),'original_nonWR_fields_all_strict':True,'expected_not_from_candidate_outputs':True,'no_whole_encrypted_tail_coverage_claim':True})
    print(json.dumps({'passed':not failure,'dataset':dataset,'summary_SHA256':sha(out/'summary.json'),'differences':len(failure)}))
    if failure:raise SystemExit(1)

def main():
    p=argparse.ArgumentParser();s=p.add_subparsers(dest='cmd',required=True)
    q=s.add_parser('prepare');q.add_argument('--out',type=Path,required=True)
    q=s.add_parser('selftest');q.add_argument('--plan',type=Path,required=True);q.add_argument('--out',type=Path,required=True)
    q=s.add_parser('verify');q.add_argument('--dataset',choices=['source49','source50'],required=True);q.add_argument('--source',required=True)
    for n in('plan','events','CLI','descriptors','out'):q.add_argument('--'+n,dest='cli'if n=='CLI'else n,type=Path,required=True)
    for n in('events-producer','CLI-producer','descriptors-producer'):q.add_argument('--'+n,dest=n.replace('-','_'),type=Path)
    a=p.parse_args()
    if a.cmd=='prepare':prepare(a.out)
    elif a.cmd=='selftest':selftest(a.plan,a.out)
    else:verify(a.plan,a.dataset,a.events,a.cli,a.descriptors,a.source,a.out,a.events_producer,a.CLI_producer,a.descriptors_producer)
if __name__=='__main__':main()