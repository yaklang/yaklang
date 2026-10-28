"""Frozen whole Native15->membership bridge; no heuristic ID normalization.

Prepare reads only baseline/original sources. Verify is a separate invocation
after binaries and closed producers exist. Retained+MLD events pair in actual
publisher order after removing only source-proved new IGMP events. All values
outside the explicit generated-PDU schema are compared as entire typed objects.
"""
import argparse
import collections
import copy
import hashlib
import importlib.util
import itertools
import json
from pathlib import Path
import sys


def sha(p):
    h=hashlib.sha256()
    with Path(p).open('rb')as f:
        for b in iter(lambda:f.read(1048576),b''):h.update(b)
    return h.hexdigest()


def rows(p):
    with Path(p).open(encoding='utf-8-sig')as f:
        for line in f:yield json.loads(line)


def same(a,b):
    if type(a)is not type(b):return False
    if isinstance(a,dict):return a.keys()==b.keys()and all(same(a[k],b[k])for k in a)
    if isinstance(a,list):return len(a)==len(b)and all(same(x,y)for x,y in zip(a,b))
    return a==b


def canonical(v):return json.dumps(v,sort_keys=True,separators=(',',':'),ensure_ascii=False)
def digest(v):return hashlib.sha256(canonical(v).encode()).hexdigest()
def write(p,v):Path(p).write_text(json.dumps(v,sort_keys=True,indent=2)+'\n',encoding='utf8')


def load_module(name,p):
    s=importlib.util.spec_from_file_location(name,p);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m


def setup(a):
    sys.path[:0]=[str(a.audit_modules),str(a.igmp_modules),str(a.mld_modules)]
    from vxlan_endpoint_projection_audit import capture_records
    igmp=load_module('igmp_native_oracle',a.igmp_modules/'igmp_native_expected.py')
    mld=load_module('mld_native_oracle',a.mld_modules/'mld_native_expected.py')
    carrier=load_module('source_carrier',a.carrier)
    refs=load_module('publisher_reference_schema',a.audit_modules/'known_pdu_reference_paths_v2.py')
    files=[a.igmp_modules/name for name in('igmp_native_expected.py','igmp_expected.py','igmp_carrier_source.py')]
    files += [a.mld_modules/name for name in('mld_native_expected.py','mld_expected.py')]
    files += [a.carrier,a.audit_modules/'known_pdu_reference_paths.py',a.audit_modules/'known_pdu_reference_paths_v2.py']
    return capture_records,igmp,mld,carrier,refs,{str(p):sha(p)for p in files}


def identity(e):
    s=e['source_bytes']
    return digest({k:e[k]for k in('protocol','transport','profile','status','raw','domain','flow','source','destination')}|
                  {'kind':s['Kind'],'SHA256':s['SHA256'],'PacketRefs':s.get('PacketRefs'),'CompletedIn':s.get('CompletedIn')})


def physical_frame(e):
    s=e['source_bytes'];rs=s.get('PacketRefs')or[]
    return rs[0]['Number']if s['Kind']=='captured'and len(rs)==1 and type(rs[0]['Number'])is int else None


def valid_id(e,seen):
    v=e['id'];assert type(v)is int and v>0 and v not in seen,'duplicate/invalid publisher ID';seen.add(v)
    assert e['field_values']['pdu']==v and type(e['field_values']['pdu'])is int
    ids=[x for x in e['field_values']['values']if x['name']=='pdu.id']
    assert len(ids)==1 and type(ids[0]['value'])is int and ids[0]['value']==v and ids[0]['exact_value']==str(v)
    return v


class PublisherSourceOrder:
    """Necessary packet-ingress causality, independent of wall clocks/peers."""
    def __init__(self,targets):
        self.targets=sorted(targets);self.next=0;self.maximum_old_contributor=0

    def added(self,frame):
        assert self.next<len(self.targets)and frame==self.targets[self.next], 'new IGMP source order changed'
        assert frame>=self.maximum_old_contributor, 'new IGMP delayed past an already-published later physical contributor'
        self.next+=1

    def existing(self,e):
        refs=e['source_bytes'].get('PacketRefs')or[]
        assert all(type(x['Number'])is int and x['Number']>=0 for x in refs)
        current=max((x['Number']for x in refs),default=0)
        assert self.next==len(self.targets)or self.targets[self.next]>=current, 'pending earlier IGMP source skipped before later contributor'
        self.maximum_old_contributor=max(self.maximum_old_contributor,current)


def source_match(e,r,s,family):
    f=physical_frame(e);assert f==s['frame']
    domain=e['domain'];assert domain['Section']==r['section']and domain['Interface']==r['interface']
    assert e['source_bytes']['PacketRefs']==[dict(Number=f,Domain=domain)]
    if family=='igmp':
        from igmp_native_expected import literal_capture_encapsulation
        assert domain['Encapsulation']==literal_capture_encapsulation(r,s['location'])and e['protocol']=='igmp'and e['transport']=='network'and e['flow']==0
        recipe=s['source1_recipe'];raw=r['wire'][recipe['raw_start']:recipe['raw_end']]
    else:
        old=s['old_event'];assert same(domain,old['domain'])and e['protocol']=='icmpv6'and e['transport']=='network'and e['flow']==0
        recipe=s['source_profile'];raw=r['wire'][recipe['raw_start']:recipe['raw_end']]
    import base64
    assert base64.b64decode(e['raw'])==raw and hashlib.sha256(raw).hexdigest()==e['source_bytes']['SHA256']


def changes(x,y,path=''):
    if type(x)is not type(y):return [path+' [type]']
    if isinstance(x,dict):
        out=[]
        for k in sorted(x.keys()|y.keys()):out += [path+'/'+k+' [missing]']if k not in x or k not in y else changes(x[k],y[k],path+'/'+k)
        return out
    if isinstance(x,list):
        out=[]if len(x)==len(y)else[path+' [length]']
        for i,(a,b)in enumerate(zip(x,y)):out+=changes(a,b,path+'/'+str(i))
        return out
    return []if x==y else[path]


def prepare(a):
    read,igmp,mld,carrier,refs,helpers=setup(a);a.out.mkdir(exist_ok=False)
    mldplan=json.loads((a.mld_plan/'summary.json').read_text());igmpp=json.loads((a.igmp_plan/'summary.json').read_text())
    targets={};records={};caps={'weber':a.capture_weber,'sharon':a.capture_sharon}
    for corpus,cap in caps.items():
        it={x['frame']:x for x in rows(a.igmp_plan/corpus/'sources.jsonl')if x['selected_membership_profile']}
        mt={x['frame']:x for x in rows(a.mld_source/(corpus+'-source.jsonl'))}
        assert not(it.keys()&mt.keys());targets[corpus]=dict(igmp=it,mld=mt)
        records[corpus]={i:r for i,r in enumerate(read(cap),1)if i in it or i in mt}
        for n,s in it.items():igmp.event(1000000+n,records[corpus][n],s)
        for n,s in mt.items():r=records[corpus][n];r['frame']=n;mld.event(s['id'],r,s,carrier.locate)
    cases=[];eventcounts=collections.Counter();old_counts={};target_seen=collections.Counter();small=[]
    prods=sorted(a.before.glob('*/*/events-producer.json'));assert len(prods)==37
    for p in prods:
        rel=p.relative_to(a.before);case=p.parent.name;pr=json.loads(p.read_text());data=p.with_name('events.jsonl')
        assert pr['exit_code']==0 and pr['export_SHA256']==sha(data)and pr['capture_SHA256']==sha(Path(pr['capture']))
        assert pr['source']=='4e664ebaed542ffb80024ad70d797695975cee77'
        order=[];seen=set();ml=[];igmp_old=[];unplanned=[]
        for e in rows(data):
            identifier=valid_id(e,seen);order.append(identifier);f=physical_frame(e)
            if case in targets and f in targets[case]['igmp']:igmp_old.append(e)
            if case in targets and f in targets[case]['mld']:
                src=targets[case]['mld'][f];assert same(e,src['old_event']);ml.append(dict(frame=f,old_id=e['id']));target_seen[case]+=1
            elif e['protocol']=='igmp'or e['protocol']=='icmpv6'and(e.get('session')or{}).get('Type')in(130,131,132,143):unplanned.append(e['id'])
        assert not igmp_old,'planned new IGMP source already owns an old native event'
        assert not unplanned,'an existing membership event lacks source plan'
        entry=dict(relative_producer=str(rel).replace('\\','/'),case=case,source=pr['source'],producer_SHA256=sha(p),capture_SHA256=pr['capture_SHA256'],
                   export_SHA256=sha(data),old_rows=len(order),publisher_ID_order_SHA256=digest(order),MLD_replacements=ml,
                   expected_new_IGMP_frames=sorted(targets[case]['igmp'])if case in targets else[],all_other_events='ordered whole-value equality with explicit PDU-reference schema only')
        if case not in targets:
            literals=[]
            # No actual source protocol2 or MLD in the other35 inputs may be
            # assumed from native labels alone. Re-read every original record.
            import igmp_carrier_source
            for n,r in enumerate(read(Path(pr['capture'])),1):
                for family,walker in [('igmp',igmp_carrier_source.locate),('mld',carrier.locate)]:
                    try:l=walker(r)
                    except AssertionError:continue
                    b=r['wire'][l['offset']:l['end']]
                    if family=='igmp'and l['protocol']=='igmp'or family=='mld'and l['protocol']=='icmpv6'and b and b[0]in(130,131,132,143):literals.append(dict(frame=n,family=family,type=b[0]if b else None,location=l))
            assert not literals,'expand the pre-candidate source plan for an additional input'
            entry['all_original_records_scanned_no_membership_source']=True;small.append(case)
        else:assert target_seen[case]==len(targets[case]['mld'])
        cases.append(entry);eventcounts[case]+=len(order)
    write(a.out/'cases.json',cases)
    source_targets={c:{family:list(v.values())for family,v in groups.items()}for c,groups in targets.items()}
    for c,x in source_targets.items():
        folder=a.out/c;folder.mkdir();write(folder/'IGMP-source-plan.json',x['igmp']);write(folder/'MLD-source-plan.json',x['mld'])
    summary=dict(scope=__doc__,phase='pre-candidate exact change plan; no candidate read',cases=len(cases),old_rows=sum(eventcounts.values()),
                 IGMP_additions={c:len(v['igmp'])for c,v in targets.items()},old_IGMP_source_events=0,MLD_replacements={c:len(v['mld'])for c,v in targets.items()},
                 other35_no_native_or_original_membership_sources=small,cases_SHA256=sha(a.out/'cases.json'),source_plans={str(p.relative_to(a.out)):sha(p)for p in a.out.glob('*/*-source-plan.json')},
                 IGMP_plan_SHA256=sha(a.igmp_plan/'summary.json'),MLD_plan_SHA256=sha(a.mld_plan/'summary.json'),MLD_source_SHA256=sha(a.mld_source/'summary.json'),helpers=helpers,auditor_SHA256=sha(__file__),
                 mapping_policy='All old rows retain their relative publication order, including237 MLD replacements. Only151 exact original-source newIGMP rows are removed before pairing. Added IGMP follows original source-frame order and cannot be delayed past an already-published later physical contributor. Full source/raw identity precedes injective IDs; remaining values then compare entire objects with explicit known reference rewriting. No time/endpoint-window or arbitrary integer normalization.')
    write(a.out/'summary.json',summary);print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=sha(a.out/'summary.json'),old_rows=summary['old_rows'],IGMP=summary['IGMP_additions'],MLD=summary['MLD_replacements'])))


def validate_literal_identity_files(oldfile,newfile,mapping):
    """Only a proved byte-identical whole export may bypass a partial mapper."""
    assert all(type(k)is type(v)is int and k==v for k,v in mapping.items()), 'unplanned scope has nonidentity publisher IDs'
    assert sha(oldfile)==sha(newfile), 'unplanned scope is not whole-byte literal'
    return True


def verify(a):
    read,igmp,mld,carrier,refs,helpers=setup(a);plan=json.loads((a.plan/'summary.json').read_text());assert same(helpers,plan['helpers'])and plan['auditor_SHA256']==sha(a.frozen_auditor)
    a.out.mkdir(exist_ok=False);assert plan['cases_SHA256']==sha(a.plan/'cases.json')
    cases=json.loads((a.plan/'cases.json').read_text());caps={'weber':a.capture_weber,'sharon':a.capture_sharon};inputs={};records={}
    for corpus,cap in caps.items():
        groups={f:{x['frame']:x for x in json.loads((a.plan/corpus/(f.upper()+'-source-plan.json')).read_text())}for f in('igmp','mld')}
        for family in groups:assert plan['source_plans'][str(Path(corpus)/(family.upper()+'-source-plan.json'))]==sha(a.plan/corpus/(family.upper()+'-source-plan.json'))
        inputs[corpus]=groups;records[corpus]={i:r for i,r in enumerate(read(cap),1)if i in groups['igmp']or i in groups['mld']}
        for n,r in records[corpus].items():r['frame']=n
    binary=json.loads((a.bins/'events-receipt.json').read_text());assert binary['source']==a.source_after and binary['exit_code']==0 and binary['binary_SHA256']==sha(a.bins/'events.exe')
    results=[];controls=[]
    for case in cases:
        name=case['case'];rel=case['relative_producer'];oldpr=a.before/rel;newpr=a.after/rel;oldfile=oldpr.with_name('events.jsonl');newfile=newpr.with_name('events.jsonl')
        op,np=json.loads(oldpr.read_text()),json.loads(newpr.read_text());assert sha(oldpr)==case['producer_SHA256']and sha(oldfile)==case['export_SHA256']
        assert np['exit_code']==0 and np['source']==a.source_after and np['binary_SHA256']==binary['binary_SHA256']and np['export_SHA256']==sha(newfile)
        assert op['capture_SHA256']==np['capture_SHA256']==case['capture_SHA256']and op['keylog_used']==np['keylog_used']and op['arguments'][1:]==np['arguments'][1:]
        targets=inputs.get(name,{'igmp':{},'mld':{}});mapping={};seen=set();old_seen=set();added={};mld_seen=set();left=iter(rows(oldfile));last_igmp_frame=0
        publisher=PublisherSourceOrder(targets['igmp'])
        for ne in rows(newfile):
            valid_id(ne,seen);f=physical_frame(ne)
            if ne['protocol']=='igmp':
                assert f in targets['igmp']and f not in added and f>last_igmp_frame,'unexpected/duplicate/reordered new IGMP source'
                publisher.added(f)
                source_match(ne,records[name][f],targets['igmp'][f],'igmp');added[f]=ne['id'];last_igmp_frame=f;continue
            oe=next(left,None);assert oe is not None,'unexpected extra non-IGMP event';valid_id(oe,old_seen)
            of=physical_frame(oe)
            if of in targets['mld']:
                assert of not in mld_seen;source_match(ne,records[name][of],targets['mld'][of],'mld');mld_seen.add(of)
            else:assert identity(oe)==identity(ne),'ordered retained event/source identity changed'
            publisher.existing(ne)
            # valid_id(ne,seen) has already proved uniqueness across every new
            # event, including additions; avoid a quadratic mapping.values scan.
            assert oe['id']not in mapping;mapping[oe['id']]=ne['id']
        assert next(left,None)is None and len(mapping)==case['old_rows']and len(set(mapping.values()))==len(mapping)
        assert set(added)==set(targets['igmp'])and mld_seen==set(targets['mld'])
        literal_identity = name not in inputs
        if literal_identity:validate_literal_identity_files(oldfile,newfile,mapping)
        checked=collections.Counter();differences=[];old_stream=iter(rows(oldfile));positive=None
        for ne in rows(newfile):
            f=physical_frame(ne)
            if ne['protocol']=='igmp':want=igmp.event(ne['id'],records[name][f],targets['igmp'][f]);kind='new-IGMP'
            else:
                oe=next(old_stream);of=physical_frame(oe)
                if of in targets['mld']:want,_,_=mld.event(ne['id'],records[name][of],targets['mld'][of],carrier.locate);kind='source-MLD'
                else:want=copy.deepcopy(oe)if literal_identity else refs.remap_known_pdu_references(oe,mapping);kind='retained-reference-only'
            if not same(want,ne):differences.append(dict(id=ne['id'],frame=f,kind=kind,paths=changes(want,ne),expected=want,actual=ne))
            checked[kind]+=1
            if positive is None and kind=='retained-reference-only':positive=(want,ne)
        folder=a.out/name;folder.mkdir();write(folder/'pdu-map.json',mapping);write(folder/'differences.json',differences)
        assert not differences,str(folder/'differences.json')
        if positive is not None:
            want,original=positive
            for label,fn in [('flow',lambda x:x.update(flow=x['flow']+1)),('domain',lambda x:x['domain'].update(Interface=x['domain']['Interface']+1)),('raw',lambda x:x.update(raw='AA==')),('ID-type',lambda x:x.update(id=float(x['id']))),('wire-integer',lambda x:x['source_bytes']['PacketRefs'][0].update(Number=x['source_bytes']['PacketRefs'][0]['Number']+1))]:
                c=copy.deepcopy(original);fn(c);assert not same(want,c);controls.append(name+'/'+label)
        results.append(dict(case=name,old_rows=case['old_rows'],new_rows=sum(checked.values()),checks=dict(checked),mapping_injective=True,whole_byte_literal_identity=literal_identity,map_SHA256=sha(folder/'pdu-map.json'),before_SHA256=sha(oldfile),after_SHA256=sha(newfile),producer_SHA256=sha(newpr)))
    summary=dict(scope=__doc__,source_after=a.source_after,source_plan_SHA256=sha(a.plan/'summary.json'),binary_receipt_SHA256=sha(a.bins/'events-receipt.json'),files=results,
                 old_rows=sum(v['old_rows']for v in results),new_rows=sum(v['new_rows']for v in results),negative_controls=controls,whole_object_differences=0,auditor_SHA256=sha(__file__),frozen_auditor_SHA256=sha(a.frozen_auditor))
    write(a.out/'summary.json',summary);print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=sha(a.out/'summary.json'),old_rows=summary['old_rows'],new_rows=summary['new_rows'])))


def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('mode',choices=['prepare','verify'])
    for n in('before','capture-weber','capture-sharon','audit-modules','igmp-modules','mld-modules','carrier','out'):p.add_argument('--'+n,type=Path,required=True)
    for n in('igmp-plan','mld-source','mld-plan','plan','after','bins'):p.add_argument('--'+n,type=Path)
    p.add_argument('--frozen-auditor',type=Path);p.add_argument('--source-after');a=p.parse_args()
    (prepare if a.mode=='prepare'else verify)(a)


if __name__=='__main__':main()
