"""Strict 37-input verification: seven whole source-built objects, all else literal.

No PDU-ID mapping, changed-path allowance, alias normalization, current-output
expectation generation or partial CLI result may produce this certificate.
"""
import argparse,hashlib,importlib.util,itertools,json,traceback
from pathlib import Path
from natt_expected import same,canonical,sha_bytes

def sha(p):
    h=hashlib.sha256()
    with Path(p).open('rb') as f:
        for b in iter(lambda:f.read(1<<20),b''):h.update(b)
    return h.hexdigest()
def read(p):return json.loads(Path(p).read_text(encoding='utf-8-sig'))
def module(name,p):
    spec=importlib.util.spec_from_file_location(name,p);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m
def duplicate_guard(pairs):
    out={}
    for k,v in pairs:assert k not in out,('duplicate JSON key',k);out[k]=v
    return out
def parse(raw):return json.loads(raw,object_pairs_hook=duplicate_guard)
def write(p,value):p.write_text(json.dumps(value,ensure_ascii=False,indent=2,allow_nan=False)+'\n',encoding='utf8')

def run(a):
    plan=read(a.plan);base=a.plan.parent
    for name,value in plan['outputs'].items():assert sha(base/name)==value,('frozen plan member changed',name)
    source_module=Path(__file__).with_name('natt_expected.py')
    assert sha(source_module)==plan['source_file_bindings'][str(source_module)]
    frozen=read(base/'seven-complete-native-expectations.json')
    assert len(frozen)==7
    if a.kind=='native':
        helper_name='public_native_literal_continuity.py';old_entries=plan['native']
        helper=a.audit_modules/helper_name
        assert sha(helper)==plan['source_file_bindings'][str(helper)]
        runtime,new=module('closed_native',helper).executions(a.after)
        new={Path(k).as_posix():v for k,v in new.items()}
        before_source=plan['native_baseline_source']
        expected_key='expected_native_event';changed_relative='preservation/weber/events.jsonl'
        replacements={r['baseline_native_ordinal']:r for r in frozen}
        complete_cli_sha=None
    else:
        assert a.complete_cli_plan is not None,'Complete source-frozen CLI rows are required; the partial template cannot close a whole-CLI proof'
        cp=read(a.complete_cli_plan)
        assert cp['complete_expected_available']is True,'Incomplete CLI template cannot produce a whole-CLI certificate'
        assert cp['after_output_used']is False,'Current CLI output cannot be used to fill its expected objects'
        assert cp['baseline_CLI_runtime_SHA256']==plan['CLI_runtime_SHA256']
        assert cp['seven_native_plan_SHA256']==sha(base/'seven-complete-native-expectations.json')
        assert cp['source_contract_bindings'], 'Complete CLI expectation must identify independent frozen presentation/source inputs'
        old_by_frame={r['frame']:r for r in frozen};replacements={}
        for row in cp['required_complete_rows']:
            source=old_by_frame[row['frame']]
            assert same(row['baseline_whole_row'],source['frozen_CLI_row'])
            assert row['baseline_ordinal']==source['baseline_CLI_ordinal']
            assert row['baseline_line_SHA256']==source['frozen_CLI_line_SHA256']
            assert type(row['expected_whole_row'])is dict
            assert row['expected_whole_row']['number']==row['frame']
            replacements[row['baseline_ordinal']]=dict(frame=row['frame'],expected_CLI_row=row['expected_whole_row'],
                baseline_CLI_line_SHA256=row['baseline_line_SHA256'])
        assert len(replacements)==7
        helper_name='public_cli_literal_continuity.py';helper=a.audit_modules/helper_name
        assert sha(helper)==plan['source_file_bindings'][str(helper)]
        runtime,new,_=module('closed_cli',helper).executions(a.after)
        new={(Path(k)/'yak.jsonl').as_posix():v for k,v in new.items()}
        old_entries=plan['CLI'];before_source=plan['CLI_baseline_source']
        expected_key='expected_CLI_row';changed_relative='preservation/weber/yak.jsonl'
        complete_cli_sha=sha(a.complete_cli_plan)
    assert runtime['source']==a.after_source
    assert {r['relative_file'] for r in old_entries}==new.keys()
    checks=[];total=0;changed=0
    for old in old_entries:
        key=old['relative_file'];fresh=new[key]
        before_path=Path(old['baseline_file']);after_path=a.after/Path(key)
        assert sha(before_path)==old['baseline_SHA256'],('baseline changed',key)
        assert sha(old['baseline_producer'])==old['baseline_producer_SHA256']
        producer=fresh['producer'] if a.kind=='native' else fresh['receipt']
        assert producer['arguments'][1:]==old['arguments_tail'],('runtime options changed',key)
        assert producer['capture']==old['capture'] and producer['capture_SHA256']==old['capture_SHA256']
        assert producer['keylog_used']==old['keylog_used_recorded']
        assert fresh['rows']==old['baseline_rows']
        item=dict(export=key,rows=fresh['rows'],new_producer_SHA256=fresh['receipt_SHA256'],new_export_SHA256=sha(after_path))
        if key!=changed_relative:
            assert sha(after_path)==old['baseline_SHA256'],('unrelated complete export changed',key)
            item.update(all_bytes_literal=True,changed_objects=0)
        else:
            seen=set();canonical_hash=hashlib.sha256();count=0
            with before_path.open('rb') as left,after_path.open('rb') as right:
                for ordinal,(b,c) in enumerate(itertools.zip_longest(left,right),1):
                    assert b is not None and c is not None,('row count changed',key,ordinal)
                    expected=replacements.get(ordinal)
                    if expected is None:
                        assert b==c,('unrelated row bytes changed',key,ordinal)
                        if a.kind=='native':canonical_hash.update(canonical(parse(c))+b'\n')
                    else:
                        before=parse(b);now=parse(c)
                        if a.kind=='native':
                            assert sha_bytes(b)==expected['baseline_native_line_SHA256']
                            assert same(before,expected['baseline_native_event'])
                            assert now['id']==before['id']==expected['expected_native_event']['id']
                            assert same(now,expected[expected_key]),('whole source-built event differs',key,ordinal,before['id'])
                            canonical_hash.update(canonical(now)+b'\n')
                        else:
                            assert sha_bytes(b)==expected['baseline_CLI_line_SHA256']
                            assert before['number']==now['number']==expected['frame']
                            assert same(now,expected[expected_key]),('whole source-built CLI row differs',key,ordinal,before['number'])
                        seen.add(ordinal)
                    count+=1
            assert seen==replacements.keys() and len(seen)==7 and count==old['baseline_rows']
            if a.kind=='native':assert canonical_hash.hexdigest()==old['expected_whole_export_canonical_SHA256']
            item.update(changed_objects=7,all_other_row_bytes_literal=True,seven_complete_objects_equal_frozen_source=True)
            if a.kind=='native':item['whole_export_canonical_SHA256']=canonical_hash.hexdigest()
            changed+=7
        total+=fresh['rows'];checks.append(item)
    assert changed==7
    summary=dict(scope=__doc__,kind=a.kind,before_source=before_source,after_source=a.after_source,inputs=37,
        closed_exports=len(checks),whole_rows=total,exact_source_corrections=7,unplanned_differences=0,
        ID_mapping=False,protocol_name_normalization=False,all_unchanged_bytes_literal=True,
        plan_SHA256=sha(a.plan),complete_CLI_plan_SHA256=complete_cli_sha,
        auditor_SHA256=sha(Path(__file__)),constructor_SHA256=sha(source_module),after_runtime_SHA256=sha(a.after/'summary.json'),
        ProtocolStats_scope='not exported by the 39 native runtime helpers; independent stats/API probe required',checks=checks)
    write(a.out/'summary.json',summary)
    print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=sha(a.out/'summary.json'),kind=a.kind,inputs=37,whole_rows=total,source_corrections=7,unplanned_differences=0)))

def main():
    p=argparse.ArgumentParser(description=__doc__)
    for name in ['plan','after','audit-modules','out']:p.add_argument('--'+name,type=Path,required=True)
    p.add_argument('--after-source',required=True);p.add_argument('--kind',choices=['native','CLI'],required=True)
    p.add_argument('--complete-cli-plan',type=Path)
    a=p.parse_args();a.out.mkdir(exist_ok=False)
    try:run(a)
    except Exception:
        write(a.out/'failure.json',dict(auditor_SHA256=sha(__file__),arguments={k:str(v) for k,v in vars(a).items()},failure=traceback.format_exc(),no_success_certificate=True))
        raise
if __name__=='__main__':main()
