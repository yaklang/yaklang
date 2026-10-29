"""Source-frozen807 OSPF insertions and complete37-input native preservation.

Prepare reads closed a82 producers, original sources and an independent field
plan. Verify pairs unchanged non-OSPF publications in their original order.
One injective generated-PDU ledger may change only the explicitly inventoried
reference schema; physical frames, wire integers, flows and all other fields
remain literal. New rows are whole source-constructed objects, not label fixes.
"""
import argparse,base64,collections,copy,hashlib,importlib.util,json,shutil,sys
from pathlib import Path


def sha(p):
    h=hashlib.sha256()
    with Path(p).open('rb')as f:
        for b in iter(lambda:f.read(1<<20),b''):h.update(b)
    return h.hexdigest()


def rows(p):
    with Path(p).open(encoding='utf-8-sig')as f:
        for line in f:yield json.loads(line)


def same(a,b):
    if type(a)is not type(b):return False
    if isinstance(a,dict):return a.keys()==b.keys() and all(same(a[k],b[k])for k in a)
    if isinstance(a,list):return len(a)==len(b) and all(same(x,y)for x,y in zip(a,b))
    return a==b


def write(p,data):
    with Path(p).open('x',encoding='utf8')as f:json.dump(data,f,indent=2);f.write('\n')


def load(name,p):
    s=importlib.util.spec_from_file_location(name,p);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m


def grid(plan):
    p=json.loads(plan.read_text('utf8'))
    assert p['complete_field_session_pairs']==807 and p['no_candidate_implementation_or_output_read']is True
    path=Path(p['fields_session_grid']);assert sha(path)==p['fields_session_grid_SHA256']
    data={}
    for r in rows(path):
        identity=r['source_identity'];key=(identity['capture_SHA256'],identity['frame'])
        assert key not in data;data[key]=r
    assert len(data)==807
    return p,data


def prepare(a):
    assert not a.out.exists()
    source_plan,targets=grid(a.field_plan)
    assert sha(a.field_plan)=='0f7caa92c06cf6971fe13cae804810b6cab4bbda38235d1f7cd78dd7a3326036'
    constructor=load('ospfv3_source_expected',a.constructor)
    cases=[];event_total=0;hashes={}
    def bound(p):
        name=str(p.resolve())
        if name not in hashes:hashes[name]=sha(p)
        return hashes[name]
    for producer in sorted(a.before.rglob('*-producer.json')):
        kind=producer.name.split('-producer.json')[0]
        assert kind in('events','flows')
        p=json.loads(producer.read_text('utf8'))
        assert p['source']=='a82acd0fff97ae50dfc1bbb403c16af359d7bf8f' and p['exit_code']==0
        export=producer.with_name('events.jsonl'if kind=='events'else'flows.json')
        assert bound(export)==p['export_SHA256'] and bound(Path(p['capture']))==p['capture_SHA256']
        assert bound(Path(p['arguments'][0]))==p['binary_SHA256']
        count=0
        if kind=='events':
            for e in rows(export):
                assert e['protocol']!='ospf','this before checkpoint has no native OSPF events'
                count+=1
            event_total+=count
        cases.append(dict(kind=kind,producer=producer.relative_to(a.before).as_posix(),export=export.relative_to(a.before).as_posix(),producer_SHA256=bound(producer),export_SHA256=bound(export),rows=count,capture_SHA256=p['capture_SHA256'],capture=p['capture'],arguments=p['arguments'][1:],keylog_used=p['keylog_used']))
    assert len(cases)==39 and sum(c['kind']=='events'for c in cases)==37 and event_total==306455
    # Prove every whole-source construction is defined now. The placeholder1
    # is never a claimed runtime ID; a separate publisher ledger supplies it.
    witnesses=[]
    for key,r in sorted(targets.items()):
        e=constructor.event(1,r)
        assert e['source_bytes']['PacketRefs'][0]['Number']==key[1]
        witnesses.append(dict(capture_SHA256=key[0],frame=key[1],whole_with_explicit_placeholder_ID1_SHA256=hashlib.sha256(json.dumps(e,sort_keys=True,separators=(',',':')).encode()).hexdigest()))
    a.out.mkdir();frozen=a.out/'frozen';frozen.mkdir()
    files=[(Path(__file__),'bridge.py'),(a.constructor,'ospfv3_native_expected36.py')]
    files.extend((a.audit_modules/n,n)for n in['known_pdu_reference_paths.py','known_pdu_reference_paths_v2.py'])
    copied={}
    for src,name in files:
        dest=frozen/name;shutil.copyfile(src,dest);assert sha(src)==sha(dest);copied[name]=sha(dest)
    write(a.out/'cases.json',cases);write(a.out/'source-construction-witnesses.json',witnesses)
    plan=dict(scope=__doc__,before_source='a82acd0fff97ae50dfc1bbb403c16af359d7bf8f',before=str(a.before.resolve()),before_runtime_SHA256=bound(a.before/'summary.json'),field_plan=str(a.field_plan.resolve()),field_plan_SHA256=sha(a.field_plan),source_grid_SHA256=source_plan['fields_session_grid_SHA256'],cases_SHA256=sha(a.out/'cases.json'),constructor_witnesses_SHA256=sha(a.out/'source-construction-witnesses.json'),source_files=copied,event_objects_before=event_total,source_insertions=807,expected_event_objects=event_total+807,flow_objects=2,normalization='Only one explicit injective generated-PDU ledger and separately source-proved capture DecodeErrors deltas; no arbitrary integer/path allowance, type coercion, label aliases or array sorting')
    write(a.out/'plan.json',plan);print(json.dumps(dict(plan=str(a.out/'plan.json'),SHA256=sha(a.out/'plan.json'),closed_before_files=len(cases),independent_source_constructions=len(witnesses))))


def identity(e):
    s=e['source_bytes']
    return {k:e[k]for k in('protocol','transport','profile','status','raw','domain','flow','source','destination')}|{'source':{k:s.get(k)for k in('Kind','SHA256','PacketRefs','CompletedIn')}}


def valid_id(e,seen):
    n=e['id'];assert type(n)is int and n>0 and n not in seen,'invalid/duplicate PDU'
    seen.add(n);assert e['field_values']['pdu']==n and type(e['field_values']['pdu'])is int
    values=[x for x in e['field_values']['values']if x['name']=='pdu.id']
    assert len(values)==1 and type(values[0]['value'])is int and values[0]['value']==n and values[0]['exact_value']==str(n)


def verify(a):
    assert not a.out.exists();p=json.loads((a.plan/'plan.json').read_text('utf8'))
    assert sha(a.plan/'cases.json')==p['cases_SHA256'] and sha(a.field_plan)==p['field_plan_SHA256']
    for name,digest in p['source_files'].items():assert sha(a.plan/'frozen'/name)==digest
    assert sha(a.before/'summary.json')==p['before_runtime_SHA256']
    _,all_targets=grid(a.field_plan)
    constructor=load('ospfv3_source_constructor',a.plan/'frozen'/'ospfv3_native_expected36.py')
    refs=load('ospfv3_known_generated_references',a.plan/'frozen'/'known_pdu_reference_paths_v2.py')
    delta=json.loads(a.flow_deltas.read_text('utf8'))
    assert delta['source_bound']is True and delta['candidate_output_read']is False
    a.out.mkdir();results=[];new_total=0
    for case in json.loads((a.plan/'cases.json').read_text('utf8')):
        opath=a.before/case['producer'];npath=a.after/case['producer'];old=a.before/case['export'];new=a.after/case['export']
        assert sha(opath)==case['producer_SHA256'] and sha(old)==case['export_SHA256']
        op=json.loads(opath.read_text('utf8'));np=json.loads(npath.read_text('utf8'))
        assert np['exit_code']==0 and np['source']==a.source and sha(new)==np['export_SHA256']
        assert np['capture_SHA256']==case['capture_SHA256']==sha(Path(np['capture']))
        assert np['arguments'][1:]==case['arguments'] and np['keylog_used']==case['keylog_used']
        binary=Path(np['arguments'][0]);assert sha(binary)==np['binary_SHA256']
        digest=case['capture_SHA256'];targets={f:r for (c,f),r in all_targets.items()if c==digest}
        folder=a.out/Path(case['export']).parent;folder.mkdir(parents=True,exist_ok=True)
        if case['kind']=='flows':
            want=json.loads(old.read_text('utf8'));actual=json.loads(new.read_text('utf8'))
            change=delta['captures'][digest]
            assert want['stats']['DecodeErrors']==change['old_stats_DecodeErrors']
            want['stats']['DecodeErrors']-=change['source_proved_legacy_failures']
            assert same(want,actual),'complete flow object differs outside exact source DecodeErrors delta'
            results.append(dict(export=case['export'],whole_object=True,source_DecodeErrors_delta=-change['source_proved_legacy_failures']))
            continue
        mapping={};seen=set();old_seen=set();added={};old_iter=iter(rows(old));pending=sorted(targets);cursor=0;max_old_frame=0
        for ne in rows(new):
            valid_id(ne,seen)
            if ne['protocol']=='ospf':
                rs=ne['source_bytes'].get('PacketRefs')or[];assert len(rs)==1
                f=rs[0]['Number'];assert type(f)is int and f in targets and f not in added
                assert cursor<len(pending) and f==pending[cursor] and f>=max_old_frame,'OSPF source publication order/causality'
                cursor+=1;added[f]=ne['id'];assert same(constructor.event(ne['id'],targets[f]),ne),'whole source-built OSPF object differs'
                continue
            oe=next(old_iter,None);assert oe is not None,'extra retained publication';valid_id(oe,old_seen)
            assert same(identity(oe),identity(ne)),'retained full source identity/order changed'
            frame=max((v['Number']for v in ne['source_bytes'].get('PacketRefs')or[]),default=0)
            assert cursor==len(pending)or pending[cursor]>=frame,'pending OSPF skipped before later contributor'
            max_old_frame=max(max_old_frame,frame);assert oe['id']not in mapping;mapping[oe['id']]=ne['id']
        assert next(old_iter,None)is None and len(mapping)==case['rows'] and len(set(mapping.values()))==len(mapping) and set(added)==set(targets)
        if not targets:
            assert all(k==v for k,v in mapping.items()) and sha(old)==sha(new),'unplanned scope must remain entire-file byte literal'
        else:
            old_iter=iter(rows(old))
            for ne in rows(new):
                if ne['protocol']=='ospf':continue
                oe=next(old_iter);assert same(refs.remap_known_pdu_references(oe,mapping),ne),'retained whole object differs outside explicit generated-PDU schema'
        write(folder/'OSPF-generated-PDU-map.json',mapping)
        write(folder/'OSPF-source-published-IDs.json',added)
        count=case['rows']+len(added);new_total+=count
        results.append(dict(export=case['export'],old_events=case['rows'],new_events=count,source_insertions=len(added),whole_byte_literal=not bool(targets),injective_generated_ID_ledger=True,complete_objects_checked=True,producer_SHA256=sha(npath),export_SHA256=sha(new)))
    assert new_total==307262
    report=dict(scope=__doc__,source=a.source,inputs=37,exports=39,event_objects=new_total,flow_objects=2,source_insertions=807,unplanned_differences=0,plan_SHA256=sha(a.plan/'plan.json'),flow_source_delta_SHA256=sha(a.flow_deltas),results=results)
    write(a.out/'summary.json',report);print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=sha(a.out/'summary.json'),event_objects=new_total,flow_objects=2,unplanned_differences=0)))


def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('mode',choices=['prepare','verify'])
    for name in('before','field-plan','out'):p.add_argument('--'+name,type=Path,required=True)
    for name in('constructor','audit-modules','plan','after','flow-deltas'):p.add_argument('--'+name,type=Path)
    p.add_argument('--source');a=p.parse_args();(prepare if a.mode=='prepare'else verify)(a)


if __name__=='__main__':main()
