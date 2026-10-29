"""Freeze/verify37 complete CLI exports with807 source-built OSPFv3 rows.

Prepare consumes only closed a82 CLI/native metadata, original source facts,
the frozen independent field plan02 and pure public formatting/schema. Future
native ledgers may substitute generated PDU IDs only after whole native proof.
No candidate CLI row can contribute a value to these expectations.
"""
import argparse,collections,copy,hashlib,importlib.util,json,shutil,subprocess,sys,traceback
from pathlib import Path

FIELD_PLAN_SHA='0f7caa92c06cf6971fe13cae804810b6cab4bbda38235d1f7cd78dd7a3326036'
NATIVE_PLAN_SHA='cdd52b2aa22d1a1d67b363cbf605932a123b5ccc911706ca47c0a631d4ee1e01'
BEFORE_SOURCE='a82acd0fff97ae50dfc1bbb403c16af359d7bf8f'


def sha(p):
    h=hashlib.sha256()
    with Path(p).open('rb')as f:
        for b in iter(lambda:f.read(1<<20),b''):h.update(b)
    return h.hexdigest()


def canonical(x):return json.dumps(x,sort_keys=True,ensure_ascii=False,separators=(',',':')).encode()
def read(p):return json.loads(Path(p).read_text('utf-8-sig'))
def rows(p):
    with Path(p).open(encoding='utf-8-sig')as f:
        for line in f:yield json.loads(line)
def write(p,value):
    with Path(p).open('x',encoding='utf8')as f:json.dump(value,f,indent=2,ensure_ascii=False);f.write('\n')
def same(a,b):
    if type(a)is not type(b):return False
    if isinstance(a,dict):return a.keys()==b.keys()and all(same(a[k],b[k])for k in a)
    if isinstance(a,list):return len(a)==len(b)and all(same(x,y)for x,y in zip(a,b))
    return a==b
def load(name,p):
    s=importlib.util.spec_from_file_location(name,p);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m


def field_grid(path):
    assert sha(path)==FIELD_PLAN_SHA
    p=read(path);f=Path(p['fields_session_grid']);assert sha(f)==p['fields_session_grid_SHA256']
    result={}
    for r in rows(f):
        i=r['source_identity'];key=(i['capture_SHA256'],i['frame']);assert key not in result;result[key]=r
    assert len(result)==807;return p,result


def binding_native_metadata(before_native,case,cli_path):
    """Verify old CLI generated IDs name the same source as old native IDs.

    This is solely a binding proof. Existing field values remain the literal
    old CLI values, not values copied from native metadata.
    """
    rel=Path(case['relative']);producer=before_native/rel/'events-producer.json'
    meta=read(producer);export=producer.with_name('events.jsonl')
    assert meta['source']==BEFORE_SOURCE and meta['exit_code']==0
    assert meta['capture_SHA256']==case['capture_SHA256'] and sha(export)==meta['export_SHA256']
    def identity(e,native):
        return dict(id=e['id'],protocol=e['protocol'],status=e['status'],completeness=e['completeness'],
                    byte_source=e['source_bytes'if native else'byte_source'])
    ids={}
    for e in rows(export):
        identifier=e['id'];assert type(identifier)is int and identifier>0 and identifier not in ids
        ids[identifier]=hashlib.sha256(canonical(identity(e,True))).digest()
    used=collections.Counter()
    for row in rows(cli_path):
        for key in('pdus','published_pdus'):
            for e in row.get(key,[]):
                identifier=e['id'];assert identifier in ids
                assert ids[identifier]==hashlib.sha256(canonical(identity(e,False))).digest(),('old CLI/native identity disagreement',case['relative'],row.get('number'),identifier)
                assert e['field_values']['pdu']==identifier and same(e['byte_source'],e['field_values']['source'])
                used[identifier]+=1
    return dict(old_native_producer=str(producer),old_native_producer_SHA256=sha(producer),
                old_native_export_SHA256=sha(export),native_IDs=len(ids),CLI_unique_IDs=len(used),
                CLI_occurrences=sum(used.values()),all_CLI_IDs_bound_to_same_native_original_source=True)


def prepare(a):
    assert not a.out.exists();source_dir=Path(__file__).parent
    assert sha(a.native_plan/'plan.json')==NATIVE_PLAN_SHA
    native_plan=read(a.native_plan/'plan.json')
    _,grid=field_grid(a.field_plan)
    helpers=load('closed_CLI_schema',a.cli_helper)
    before,executions,before_sha=helpers.executions(a.before)
    assert before['source']==BEFORE_SOURCE and sum(x['rows']for x in executions.values())==384518
    a.out.mkdir();frozen=a.out/'frozen';frozen.mkdir()
    copyfiles=[(source_dir/'cli_source_plan.py','cli_source_plan.py'),
               (source_dir/'cli_source_expected.py','cli_source_expected.py'),
               (source_dir/'cli_pdu_references.py','cli_pdu_references.py'),
               (a.native_plan/'frozen/ospfv3_native_expected36.py','native_source_schema.py'),
               (a.cli_helper,'public_cli_literal_continuity.py')]
    copied={}
    for old,name in copyfiles:
        dest=frozen/name;shutil.copyfile(old,dest);assert sha(old)==sha(dest);copied[name]=sha(dest)
    sys.path.insert(0,str(frozen));constructor=load('CLI_source_constructor',frozen/'cli_source_expected.py')
    cases=[];targets=[];bindings=[];message_types=collections.Counter();corrections=0;max_pdu=0
    for rel,execution in sorted(executions.items()):
        receipt=execution['receipt'];capture=receipt['capture_SHA256']
        selected={frame:r for (digest,frame),r in grid.items()if digest==capture}
        path=a.before/rel/'yak.jsonl';found=set()
        case=dict(relative=rel,rows=execution['rows'],export_SHA256=execution['whole_export_SHA256'],
                  producer_SHA256=execution['receipt_SHA256'],capture=receipt['capture'],capture_SHA256=capture,
                  keylog_used=receipt['keylog_used'],arguments=receipt['arguments'][1:])
        if selected:
            bindings.append(dict(relative=rel,**binding_native_metadata(a.before_native,case,path)))
            for ordinal,line in enumerate(path.open('rb'),1):
                row=json.loads(line);frame=row.get('number')
                if frame not in selected:continue
                assert frame not in found;found.add(frame);source=selected[frame]
                expected=constructor.row(row,source,1)
                encoded=canonical(expected['pdus'][0]);max_pdu=max(max_pdu,len(encoded))
                assert len(encoded)<256*1024,'one source observation must fit existing per-packet scalar budget'
                corrections+=row['info']!='';message_types[str(source['expected_fields']['Header']['Type'])]+=1
                targets.append(dict(relative=rel,capture_SHA256=capture,frame=frame,baseline_ordinal=ordinal,
                    baseline_line_SHA256=hashlib.sha256(line).hexdigest(),baseline_whole_row=row,
                    original_frame_SHA256=source['original_frame_SHA256'],source_message_SHA256=source['source_message_SHA256'],
                    expected_whole_row_placeholder_ID1=expected,
                    expected_placeholder_canonical_SHA256=hashlib.sha256(canonical(expected)).hexdigest()))
            assert found==set(selected)
        case['source_OSPF_rows']=len(found);cases.append(case)
    assert len(targets)==807 and corrections==43 and len(cases)==37
    write(a.out/'cases.json',cases);write(a.out/'807-complete-source-rows.json',targets)
    write(a.out/'old-CLI-native-ID-bindings.json',bindings)
    old_format={}
    # Read immutable old public schemas only; no candidate decoder source.
    snapshots=frozen/'immutable-a82-public-schema';snapshots.mkdir()
    for name in('common/yak/cmd/yakcmds/shark-cli/packet_observation.go',
                'common/yak/cmd/yakcmds/shark-cli/command.go',
                'common/pcapx/pcaputil/protocol_field_registry.go'):
        raw=subprocess.check_output(['git','show',BEFORE_SOURCE+':'+name],cwd=a.repo)
        dest=snapshots/(Path(name).name+'.txt');dest.write_bytes(raw)
        old_format[name]=dict(snapshot=str(dest),SHA256=sha(dest))
    plan=dict(scope=__doc__,before_source=BEFORE_SOURCE,before=str(a.before.resolve()),before_runtime_SHA256=before_sha,
       field_plan=str(a.field_plan.resolve()),field_plan_SHA256=FIELD_PLAN_SHA,source_grid_SHA256=field_grid(a.field_plan)[0]['fields_session_grid_SHA256'],
       native_plan=str(a.native_plan.resolve()),native_plan_SHA256=NATIVE_PLAN_SHA,native_constructor_SHA256=native_plan['source_files']['ospfv3_native_expected36.py'],
       source_files=copied,immutable_a82_public_schema=old_format,cases_SHA256=sha(a.out/'cases.json'),
       expected_source_rows_SHA256=sha(a.out/'807-complete-source-rows.json'),old_ID_bindings_SHA256=sha(a.out/'old-CLI-native-ID-bindings.json'),
       cases=37,total_rows=384518,source_complete_OSPF_rows=807,legacy_decoder_error_rows_corrected=43,
       source_message_type_counts=dict(message_types),max_projected_OSPF_PDU_scalar_bytes=max_pdu,
       placeholder_policy='ID1 is only a constructor parameter witness, not an observed/runtime ID; resolve only with separately verified native publisher ledger',
       pure_presentation_contract=dict(protocol='OSPFv3',status='decoded',visibility='message',evidence=constructor.EVIDENCE,scope=constructor.SCOPE,
           summary='OSPFv3 {Packet Name} · router={Header.Router ID} area={Header.Area ID} instance={Header.Instance ID}',
           physical_layers='original bounded L2/IPv6/AH chain then OSPF; no DecodeFailure',display_stack='replace physical OSPF with OSPFv3; preserve every other header',
           PDU_schema='existing normal-error-free recordObservedPDU; no summary/error/expert keys'),
       candidate_decoder_read=False,candidate_CLI_output_read=False,
       preservation='Every non-target full row is literal except explicitly enumerated PDU references from the separate native ledger; no physical frame/flow/transaction/range remap; no generic normalization',
       pending='Closed native whole bridge summary/hash and original ledger bytes are mandatory before verifying any candidate CLI export')
    write(a.out/'plan.json',plan)
    print(json.dumps(dict(plan=str(a.out/'plan.json'),SHA256=sha(a.out/'plan.json'),cases=37,rows=384518,complete_source_OSPF_rows=807,legacy_error_corrections=43,max_PDU_scalar_bytes=max_pdu)),flush=True)


def load_ledgers(a,p,cases):
    summary_path=a.native_proof/'summary.json';assert sha(summary_path)==a.native_proof_sha256
    proof=read(summary_path)
    assert proof['source']==a.source and proof['plan_SHA256']==p['native_plan_SHA256']
    assert proof['inputs']==37 and proof['source_insertions']==807 and proof['event_objects']==307262 and proof['unplanned_differences']==0
    results={str(Path(r['export']).parent).replace('\\','/'):r for r in proof['results']if r['export'].endswith('events.jsonl')}
    assert len(results)==37
    bound={};ledgers={}
    for case in cases:
        rel=case['relative'];result=results[rel]
        for name in('OSPF-generated-PDU-map.json','OSPF-source-published-IDs.json'):
            path=a.native_proof/rel/name;bound[str(path)]=sha(path)
        mapping={int(k):v for k,v in read(a.native_proof/rel/'OSPF-generated-PDU-map.json').items()}
        inserted={int(k):v for k,v in read(a.native_proof/rel/'OSPF-source-published-IDs.json').items()}
        assert len(mapping)==result['old_events'] and len(set(mapping.values()))==len(mapping)
        assert all(type(k)is int and type(v)is int and k>0 and v>0 for k,v in mapping.items())
        assert len(inserted)==case['source_OSPF_rows']==result['source_insertions']
        assert all(type(k)is int and type(v)is int and k>0 and v>0 for k,v in inserted.items())
        assert len(set(inserted.values()))==len(inserted) and not set(inserted.values())&set(mapping.values())
        ledgers[rel]=(mapping,inserted)
    return proof,bound,ledgers


def verify(a):
    assert not a.out.exists();p=read(a.plan/'plan.json')
    assert sha(__file__)==p['source_files']['cli_source_plan.py'],'invoke source-frozen verifier'
    for name,digest in p['source_files'].items():assert sha(a.plan/'frozen'/name)==digest
    for name,key in(('cases.json','cases_SHA256'),('807-complete-source-rows.json','expected_source_rows_SHA256'),('old-CLI-native-ID-bindings.json','old_ID_bindings_SHA256')):
        assert sha(a.plan/name)==p[key]
    assert sha(a.before/'summary.json')==p['before_runtime_SHA256']
    sys.path.insert(0,str(a.plan/'frozen'))
    constructor=load('frozen_CLI_source_constructor',a.plan/'frozen/cli_source_expected.py')
    remapper=load('frozen_CLI_generated_reference_schema',a.plan/'frozen/cli_pdu_references.py')
    helper=load('closed_CLI_producers',a.plan/'frozen/public_cli_literal_continuity.py')
    _,grid=field_grid(a.field_plan)
    old,old_executions,_=helper.executions(a.before)
    after,new_executions,after_sha=helper.executions(a.after)
    assert old['source']==BEFORE_SOURCE and after['source']==a.source
    cases=read(a.plan/'cases.json');assert set(old_executions)==set(new_executions)=={c['relative']for c in cases}
    native,ledger_hashes,ledgers=load_ledgers(a,p,cases)
    source_rows={(r['relative'],r['baseline_ordinal']):r for r in read(a.plan/'807-complete-source-rows.json')}
    a.out.mkdir();checked=[];total=0;corrected=0;changed_id_rows=0
    for case in cases:
        rel=case['relative'];ob=old_executions[rel];nb=new_executions[rel]
        assert ob['receipt_SHA256']==case['producer_SHA256'] and ob['whole_export_SHA256']==case['export_SHA256']
        assert nb['rows']==case['rows']
        for k in('capture','capture_SHA256','keylog_used'):assert nb['receipt'][k]==ob['receipt'][k]
        assert nb['receipt']['arguments'][1:]==case['arguments']
        mapping,inserted=ledgers[rel];targets={f:r for (c,f),r in grid.items()if c==case['capture_SHA256']}
        assert set(inserted)==set(targets)
        count=0;id_rows=0;seen=set()
        old_path=a.before/rel/'yak.jsonl';new_path=a.after/rel/'yak.jsonl'
        with old_path.open('rb')as left,new_path.open('rb')as right:
            for ordinal,beforeline in enumerate(left,1):
                afterline=right.readline();assert afterline,('missing row',rel,ordinal)
                before=json.loads(beforeline);actual=json.loads(afterline);frozen=source_rows.get((rel,ordinal))
                if frozen is not None:
                    frame=frozen['frame'];assert frame not in seen;seen.add(frame)
                    assert hashlib.sha256(beforeline).hexdigest()==frozen['baseline_line_SHA256'] and same(before,frozen['baseline_whole_row'])
                    assert same(constructor.row(before,targets[frame],1),frozen['expected_whole_row_placeholder_ID1'])
                    expected=constructor.row(before,targets[frame],inserted[frame]);count+=1
                elif targets:
                    expected=remapper.remap_row(before,mapping)
                    id_rows+=not same(expected,before)
                else:
                    assert beforeline==afterline,('unaffected entire row bytes changed',rel,ordinal)
                    expected=before
                assert same(expected,actual),('whole CLI source expectation mismatch',rel,ordinal,before.get('number'))
            assert not right.readline(),('extra row',rel)
        assert seen==set(targets) and count==case['source_OSPF_rows']
        if not targets:assert ob['whole_export_SHA256']==nb['whole_export_SHA256']
        corrected+=count;total+=case['rows'];changed_id_rows+=id_rows
        checked.append(dict(relative=rel,rows=case['rows'],complete_source_OSPF_rows=count,existing_rows_with_explicit_generated_reference_changes=id_rows,
                            full_objects_checked=True,entire_file_byte_literal=not bool(targets),producer_SHA256=nb['receipt_SHA256'],export_SHA256=nb['whole_export_SHA256']))
    assert total==384518 and corrected==807
    for name,digest in ledger_hashes.items():assert sha(name)==digest
    assert sha(a.native_proof/'summary.json')==a.native_proof_sha256
    report=dict(scope=__doc__,source=a.source,cases=37,whole_rows=total,complete_source_OSPF_rows=corrected,
       legacy_decoder_error_rows_corrected=43,existing_rows_with_explicit_generated_reference_changes=changed_id_rows,
       unplanned_differences=0,plan_SHA256=sha(a.plan/'plan.json'),before_runtime_SHA256=p['before_runtime_SHA256'],
       after_runtime_SHA256=after_sha,independent_native_whole_proof=str(a.native_proof/'summary.json'),
       independent_native_whole_proof_SHA256=a.native_proof_sha256,native_ledger_SHA256=ledger_hashes,
       no_candidate_values_used_for_source_expectation=True,checks=checked)
    write(a.out/'summary.json',report)
    print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=sha(a.out/'summary.json'),whole_rows=total,complete_source_OSPF_rows=corrected,unplanned_differences=0)),flush=True)


def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('mode',choices=('prepare','verify'))
    for name in('before','field-plan','out'):p.add_argument('--'+name,type=Path,required=True)
    for name in('native-plan','before-native','cli-helper','repo','plan','after','native-proof'):p.add_argument('--'+name,type=Path)
    p.add_argument('--native-proof-sha256');p.add_argument('--source');a=p.parse_args()
    try:(prepare if a.mode=='prepare'else verify)(a)
    except Exception as e:
        if a.out.exists():
            failure=a.out/'failure.json'
            if not failure.exists():write(failure,dict(mode=a.mode,error=repr(e),traceback=traceback.format_exc(),driver_SHA256=sha(__file__)))
        raise


if __name__=='__main__':main()
