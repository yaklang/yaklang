"""Freeze/verify complete old50->new53 exports with exact Raw-proven SSH additions.
This is an interpretation-preservation contract, not independent PCAP correctness.
It never strips metadata from a candidate, rewrites IDs or normalizes other fields.
"""
from pathlib import Path
import argparse,base64,copy,hashlib,importlib.util,itertools,json,re
HERE=Path(__file__).resolve().parent;P=HERE.parent
REPO=Path(r'C:/Users/V/.codex/worktrees/shark-protocol-infrastructure/yaklang')
UTIL=REPO/'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/public_native_literal_continuity.py'
SPEC=importlib.util.spec_from_file_location('literal_binding53',UTIL);util=importlib.util.module_from_spec(SPEC);SPEC.loader.exec_module(util)
SPEC=importlib.util.spec_from_file_location('old_raw_ssh_ranges53',HERE/'ssh_old_wire53.py');wire=importlib.util.module_from_spec(SPEC);SPEC.loader.exec_module(wire)
assert util.sha(UTIL)=='cea420bb3cc200cbddd8c023cb9ea25f4a756a2f857f606cbf77f440edfa955d'
MARK=re.compile(br'"protocol"\s*:\s*"ssh"')
BEFORE='26f01f4baa5fd39b88deadf901257a70bc5c1dfe'
def demand(x,m):
    if not x:raise AssertionError(m)
def read(p):return json.loads(Path(p).read_text(encoding='utf-8-sig'))
def put(p,x):
    with Path(p).open('x',encoding='utf-8')as f:json.dump(x,f,indent=2,sort_keys=True);f.write('\n')
def h(b):return hashlib.sha256(b).hexdigest()
def tables(root,mode,expected_source=None):
    s=read(root/'summary.json');demand(s['cases']==37 and s['actual_new_binary_execution']is True,'37 closed runtime inputs')
    if expected_source:demand(s['source']==expected_source,'runtime exact source')
    paths=sorted(root.glob('*/*/*-producer.json'if mode=='native'else'*/*/yak-producer.json'))
    demand(len(paths)==len(s['outputs'])==(39 if mode=='native'else 37),'complete producer census')
    out={}
    for p in paths:
        r=read(p);demand(r in s['outputs']and r['source']==s['source']and r['exit_code']==0,'closed producer membership')
        kind=p.stem.removesuffix('-producer');demand(kind in(('events','flows')if mode=='native'else('yak',)),'known producer kind')
        binary=Path(r['arguments'][0]);build=binary.with_name(kind+'-receipt.json');b=read(build)
        demand(b['source']==s['source']and b['exit_code']==0 and util.sha(binary)==r['binary_SHA256']==b['binary_SHA256'],'exact source-built binary')
        demand(util.sha(r['capture'])==r['capture_SHA256'],'original physical capture')
        target=p.with_name(kind+('.json'if kind=='flows'else'.jsonl'));key=target.relative_to(root).as_posix()
        demand(key not in out,'duplicate export')
        out[key]={'kind':kind,'case':p.parent.relative_to(root).as_posix(),'export':str(target),'receipt':str(p),'receipt_SHA256':util.sha(p),'source':s['source'],'producer':r,'export_SHA256':r['export_SHA256']}
    return s,out

def entry(oldline,expected,adds,line,offset,proof):
    demand(adds,'only actual source-proven additions')
    return {'line':line,'byte_offset':offset,'old_line_SHA256':h(oldline),'expected_whole_object_SHA256':wire.digest(expected),'additions':adds,'source_proof':proof}
def projected(oldline,change):
    demand(h(oldline)==change['old_line_SHA256'],'frozen whole old line')
    expected=json.loads(oldline)
    for a in change['additions']:wire.apply_addition(expected,a)
    demand(wire.digest(expected)==change['expected_whole_object_SHA256'],'source-constructed full expected object must match frozen hash')
    return expected

def prepare(out,native_before,CLI_before):
    out.mkdir(exist_ok=False);ns,nt=tables(native_before,'native',BEFORE);cs,ct=tables(CLI_before,'CLI',BEFORE)
    native=[];CLI=[];lookups={};totals={'native_event_rows':0,'native_SSH_rows':0,'native_changed_rows':0,'native_unmodified_SSH_rows':0,'native_exact_metadata_additions':0,'CLI_rows':0,'CLI_changed_rows':0,'CLI_exact_alias_additions':0,'whole_flow_files':0}
    for key,record in nt.items():
        f=Path(record['export']);changes=[];index={};d=hashlib.sha256();offset=0;rows=0
        if record['kind']=='flows':
            demand(util.sha(f)==record['export_SHA256'],'old whole flow export');totals['whole_flow_files']+=1
        else:
            for n,line in enumerate(f.open('rb'),1):
                rows=n;d.update(line)
                if MARK.search(line):
                    old=json.loads(line)
                    if old.get('protocol')=='ssh':
                        totals['native_SSH_rows']+=1
                        expected,adds,alias=wire.native_expected(old)
                        anchor=wire.compact_key(old,True);demand(anchor not in index,'ambiguous native ID/source/snapshot')
                        index[anchor]={'aliases':alias,'old_private_snapshot_SHA256':wire.digest(old['field_values']),'id':old['id'],'source_bytes_SHA256':wire.digest(old['source_bytes'])}
                        if adds:
                            raw=base64.b64decode(old['raw'],validate=True)if old.get('raw')else b''
                            proof={'kind':'old Raw and every old field owner validated by sequential bounded SSH parser; preservation only','old_Raw_SHA256':h(raw),'old_Raw_length':len(raw),'old_private_snapshot_SHA256':wire.digest(old['field_values']),'old_id':old['id'],'old_source_bytes_SHA256':wire.digest(old['source_bytes'])}
                            changes.append(entry(line,expected,adds,n,offset,proof));totals['native_changed_rows']+=1;totals['native_exact_metadata_additions']+=len(adds)
                        else:totals['native_unmodified_SSH_rows']+=1
                offset+=len(line)
            demand(d.hexdigest()==record['export_SHA256'],'old native full export SHA');totals['native_event_rows']+=rows
            lookups[record['case']]=index
        native.append({**record,'rows':rows,'changes':changes,'unchanged_requires_raw_bytes':True})
    for key,record in ct.items():
        changes=[];d=hashlib.sha256();offset=0;rows=0;index=lookups[record['case']]
        for n,line in enumerate(Path(record['export']).open('rb'),1):
            rows=n;d.update(line)
            if MARK.search(line):
                old=json.loads(line);expected=copy.deepcopy(old);adds=[];proofs=[]
                for group in('pdus','published_pdus'):
                    for i,pdu in enumerate(old.get(group)or[]):
                        if pdu.get('protocol')!='ssh':continue
                        anchor=wire.compact_key(pdu);demand(anchor in index,'CLI old ID/source/full snapshot must have exact native witness')
                        source=index[anchor]
                        for a in source['aliases']:
                            v=pdu['field_values']['values'][a['index']]
                            demand(v['name']==a['name']and wire.digest(v['value'])==a['old_value_SHA256'],'exact private alias occurrence')
                            patch={'path':[group,i,'field_values','values',a['index'],'pdu_range'],'value':copy.deepcopy(a['value'])}
                            wire.apply_addition(expected,patch);adds.append(patch)
                        proofs.append({'group':group,'index':i,'old_id':pdu['id'],'native_anchor_SHA256':anchor,'old_private_snapshot_SHA256':source['old_private_snapshot_SHA256'],'old_source_bytes_SHA256':source['source_bytes_SHA256']})
                if adds:
                    changes.append(entry(line,expected,adds,n,offset,{'kind':'exact old native ID+SourceBytes+complete private snapshot witness; no ID mapping','witnesses':proofs}));totals['CLI_changed_rows']+=1;totals['CLI_exact_alias_additions']+=len(adds)
            offset+=len(line)
        demand(d.hexdigest()==record['export_SHA256'],'old CLI full export SHA');totals['CLI_rows']+=rows
        CLI.append({**record,'rows':rows,'changes':changes,'unchanged_requires_raw_bytes':True})
    demand(totals['native_event_rows']==307819 and totals['CLI_rows']==384518 and totals['whole_flow_files']==2,'full old50 corpus counts')
    demand(totals['native_SSH_rows']==74,'old50 SSH census independently closed')
    put(out/'native-files.json',native);put(out/'CLI-files.json',CLI);put(out/'counts.json',totals)
    scope={'purpose':'whole export interpretation preservation; not an independent public PCAP protocol-correctness oracle','changes':'complete old DTO copied plus exact numeric WR/pdu_range insertions derived from sequential old Raw parse and exact old field/value match; frozen full resulting object hash','all_other_values':'all IDs/Raw/refs/field values/flows andall nonSSH bytes unchanged; no blanket stripping, no ignored range keys, no ID mapper','independent_evidence':'source49/50 independent authored90binary/1487literal source51 ledger remains separate correctness evidence','future_candidate_read_for_expected':False,'before_native':str(native_before),'before_CLI':str(CLI_before),'before_source':BEFORE}
    put(out/'scope.json',scope)
    sources=[Path(__file__),HERE/'ssh_old_wire53.py',UTIL,native_before/'summary.json',CLI_before/'summary.json']
    sources+=[out/'native-files.json',out/'CLI-files.json',out/'counts.json',out/'scope.json']
    put(out/'manifest.json',{'schema':'SSH-public-ranges-preservation53-plan01','inputs':{str(p):util.sha(p)for p in sources},'before_native':str(native_before),'before_CLI':str(CLI_before),'before_native_runtime_SHA256':util.sha(native_before/'summary.json'),'before_CLI_runtime_SHA256':util.sha(CLI_before/'summary.json'),'counts':totals,'no_candidate53_read':True})
    print(json.dumps({'plan':str(out),'SHA256':util.sha(out/'manifest.json'),'counts':totals}),flush=True)

def plan_load(plan):
    m=read(plan/'manifest.json')
    for p,d in m['inputs'].items():demand(util.sha(p)==d,'frozen preservation source/input '+p)
    return m

def compare_line(oldline,newline,change):
    if change is None:demand(oldline==newline,'unplanned byte difference in a whole retained row');return
    expected=projected(oldline,change);demand(wire.strict(json.loads(newline),expected),'complete resulting object differs from frozen expected; only exact source-bound insertions permitted')

def verify(plan,native_after,CLI_after,after_source,out):
    out.mkdir(exist_ok=False);m=plan_load(plan);ns,nt=tables(native_after,'native',after_source);cs,ct=tables(CLI_after,'CLI',after_source)
    expected_native=read(plan/'native-files.json');expected_CLI=read(plan/'CLI-files.json');results=[]
    for records,actual in[(expected_native,nt),(expected_CLI,ct)]:
        demand({Path(r['export']).relative_to(Path(m['before_native']if r['kind']!='yak'else m['before_CLI'])).as_posix()for r in records}==set(actual),'complete export identities')
        for record in records:
            before_root=Path(m['before_CLI']if record['kind']=='yak'else m['before_native']);key=Path(record['export']).relative_to(before_root).as_posix();after=actual[key]
            for k in('capture','capture_SHA256','keylog_used'):demand(record['producer'][k]==after['producer'][k],'same physical/keylog source metadata')
            demand(record['producer']['arguments'][1:]==after['producer']['arguments'][1:],'same actual replay command parameters')
            demand(util.sha(record['export'])==record['export_SHA256'],'old baseline full file unchanged')
            demand(util.sha(after['export'])==after['export_SHA256'],'new closed full file bound')
            if not record['changes']:
                demand(record['export_SHA256']==after['export_SHA256'],'whole literal retained file '+key)
                results.append({'export':key,'kind':record['kind'],'whole_literal':True,'rows':record['rows'],'changes':0,'after_receipt_SHA256':after['receipt_SHA256']});continue
            changes={x['line']:x for x in record['changes']};count=0
            with Path(record['export']).open('rb')as a,Path(after['export']).open('rb')as b:
                for n,(oldline,newline)in enumerate(itertools.zip_longest(a,b),1):
                    demand(oldline is not None and newline is not None,'complete export row cardinality');compare_line(oldline,newline,changes.get(n));count=n
            demand(count==record['rows'],'entire closed export consumed')
            results.append({'export':key,'kind':record['kind'],'whole_literal':False,'rows':count,'changes':len(changes),'all_other_rows_raw_byte_equal':True,'after_receipt_SHA256':after['receipt_SHA256']})
    put(out/'summary.json',{'passed':True,'before_source':BEFORE,'after_source':after_source,'plan_SHA256':util.sha(plan/'manifest.json'),'source_constructor_SHA256':util.sha(HERE/'ssh_old_wire53.py'),'counts':m['counts'],'ID_mapping':False,'actual_metadata_blanket_stripping':False,'all_unchanged_rows_byte_identical':True,'all_changed_rows_whole_objects_exact':True,'whole_flow_files_byte_identical':2,'independent_PCAP_correctness_claim':False,'native_after_summary_SHA256':util.sha(native_after/'summary.json'),'CLI_after_summary_SHA256':util.sha(CLI_after/'summary.json'),'checks':results})
    print(json.dumps({'passed':True,'summary_SHA256':util.sha(out/'summary.json'),'counts':m['counts']}),flush=True)

def row_at(record,change):
    with Path(record['export']).open('rb')as f:f.seek(change['byte_offset']);return f.readline()

def selftest(plan,out):
    out.mkdir(exist_ok=False);m=plan_load(plan);records=read(plan/'native-files.json');clis=read(plan/'CLI-files.json')
    pair=next((r,c)for r in records for c in r['changes']if any(a['path'][0]=='field_values'for a in c['additions']))
    record,change=pair;oldline=row_at(record,change);expected=projected(oldline,change);line=lambda x:wire.canonical(x)+b'\n';compare_line(oldline,line(expected),change);negative=[]
    def bad(name,fn):
        x=copy.deepcopy(expected);fn(x)
        try:compare_line(oldline,line(x),change)
        except(AssertionError,KeyError,ValueError,TypeError):negative.append(name);return
        raise AssertionError('bad whole object accepted '+name)
    bad('native-ID-changed',lambda x:x.update(id=x['id']+1))
    bad('native-Raw-changed',lambda x:x.update(raw='AA=='))
    bad('native-source-ref-changed',lambda x:x['source_bytes']['PacketRefs'][0].update(Number=0))
    bad('native-old-message-value-changed',lambda x:x['session'].update({'Message Number':0}))
    bad('root-field-span-moved',lambda x:x['fields']['Wire Field Ranges'].update({'Packet Length':[1,5]}))
    bad('old-context-gets-wire-range',lambda x:x['session']['Wire Field Ranges'].update({'Identification':[0,4]}))
    bad('derived-gets-wire-range',lambda x:x['session']['Wire Field Ranges'].update({'Host Key Fingerprint':[0,4]}))
    bad('alias-range-wrong',lambda x:next(v for v in x['field_values']['values']if v['name']=='ssh.message_code')['pdu_range'].update(offset=0))
    bad('alias-range-float',lambda x:next(v for v in x['field_values']['values']if v['name']=='ssh.message_code')['pdu_range'].update(offset=5.0))
    bad('extra-field-not-on-allowlist',lambda x:x.update(unplanned='value'))
    unknown=b'{"protocol":"http3","transaction":"unknown-format","Wire Field Ranges":{"not-SSH":[1,2]}}\n';compare_line(unknown,unknown,None)
    for name,new in [('unknown-transaction-changed',unknown.replace(b'unknown-format',b'other-format')),('nonSSH-range-changed',unknown.replace(b'[1,2]',b'[1,3]'))]:
        try:compare_line(unknown,new,None)
        except AssertionError:negative.append(name)
        else:raise AssertionError('unmodified raw-byte fastpath became permissive')
    r,c=next((r,c)for r in clis for c in r['changes']);a=row_at(r,c);e=projected(a,c);compare_line(a,line(e),c)
    q=copy.deepcopy(e);patch=c['additions'][0];p=q
    for k in patch['path'][:-1]:p=p[k]
    p[patch['path'][-1]]={'offset':0,'length':1}
    try:compare_line(a,line(q),c)
    except AssertionError:negative.append('CLI-private-range-not-its-source')
    else:raise AssertionError('CLI wrong alias range accepted')
    put(out/'summary.json',{'passed':True,'plan_SHA256':util.sha(plan/'manifest.json'),'negative_controls_rejected':negative,'old50_only':True,'candidate53_read':False,'counts':m['counts']})
    print(json.dumps({'passed':True,'summary_SHA256':util.sha(out/'summary.json'),'negative_controls':len(negative)}))

def main():
    p=argparse.ArgumentParser();s=p.add_subparsers(dest='cmd',required=True)
    q=s.add_parser('prepare');q.add_argument('--native-before',type=Path,required=True);q.add_argument('--CLI-before',dest='CLI_before',type=Path,required=True);q.add_argument('--out',type=Path,required=True)
    q=s.add_parser('selftest');q.add_argument('--plan',type=Path,required=True);q.add_argument('--out',type=Path,required=True)
    q=s.add_parser('verify')
    for k in('plan','native-after','CLI-after','out'):q.add_argument('--'+k,dest=k.replace('-','_'),type=Path,required=True)
    q.add_argument('--after-source',required=True);a=p.parse_args()
    if a.cmd=='prepare':prepare(a.out,a.native_before,a.CLI_before)
    elif a.cmd=='selftest':selftest(a.plan,a.out)
    else:verify(a.plan,a.native_after,a.CLI_after,a.after_source,a.out)
if __name__=='__main__':main()