"""Pre-runtime bridge controls using real sources and authored transformed rows.

No row generated here is a native runtime export. Controls exercise the same
source predicate, strict typed-object comparator, publisher-ID checker and
explicit reference schema used by bridge.py.
"""
import argparse
import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import sys

from bridge import same,source_match,valid_id,identity,PublisherSourceOrder


def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()


def mod(name,p):
    s=importlib.util.spec_from_file_location(name,p);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m


def main():
    ap=argparse.ArgumentParser(description=__doc__)
    for n in('plan','before','igmp-modules','mld-modules','audit-modules','carrier','capture-weber','capture-sharon','out'):ap.add_argument('--'+n,type=Path,required=True)
    a=ap.parse_args();a.out.mkdir(exist_ok=False)
    sys.path[:0]=[str(a.igmp_modules),str(a.mld_modules),str(a.audit_modules)]
    from vxlan_endpoint_projection_audit import capture_records
    igmp=mod('new_igmp_oracle',a.igmp_modules/'igmp_native_expected.py');mld=mod('new_mld_oracle',a.mld_modules/'mld_native_expected.py');carrier=mod('source_carrier',a.carrier)
    reference=mod('reference_schema',a.audit_modules/'known_pdu_reference_paths_v2.py')
    negative=[];positive=[]
    def rejects(name,fn):
        try:fn()
        except (AssertionError,TypeError,KeyError,ValueError):negative.append(name);return
        raise AssertionError('negative accepted '+name)
    def contributor(frame):return {'source_bytes':{'PacketRefs':[{'Number':frame}]}}
    order=PublisherSourceOrder([10,20]);order.existing(contributor(5));order.added(10);order.existing(contributor(15));order.added(20);order.existing(contributor(30));positive.append('publisher-causality-ordered')
    order=PublisherSourceOrder([10,20]);rejects('publisher-causality-all-additions-at-end',lambda:order.existing(contributor(15)))
    order=PublisherSourceOrder([10,20]);rejects('publisher-causality-added-order',lambda:order.added(20))
    for corpus,cap in [('weber',a.capture_weber),('sharon',a.capture_sharon)]:
        groups={f:json.loads((a.plan/corpus/(f.upper()+'-source-plan.json')).read_text())for f in('igmp','mld')}
        chosen={f:next(x for x in rs if x.get('whole_codec_expected',{}).get('status','decoded')=='decoded')for f,rs in groups.items()}
        needed={x['frame']for x in chosen.values()};records={i:r for i,r in enumerate(capture_records(cap),1)if i in needed}
        for family,s in chosen.items():
            r=records[s['frame']];r['frame']=s['frame'];identifier=1000000+s['frame']
            e=igmp.event(identifier,r,s)if family=='igmp'else mld.event(identifier,r,s,carrier.locate)[0]
            source_match(e,r,s,family);valid_id(e,set());positive.append(corpus+'/'+family+'/whole-source-contract')
            for name,mutate in [('raw',lambda v:v.update(raw='AA==')),('hash',lambda v:v['source_bytes'].update(SHA256='0'*64)),('physical-frame',lambda v:v['source_bytes']['PacketRefs'][0].update(Number=s['frame']+1)),
                                ('domain-interface',lambda v:v['domain'].update(Interface=v['domain']['Interface']+1)),('flow',lambda v:v.update(flow=1)),('declared-family',lambda v:v.update(protocol='tls')),
                                ('frame-number-bool',lambda v:v['source_bytes']['PacketRefs'][0].update(Number=True))]:
                v=copy.deepcopy(e);mutate(v);rejects(corpus+'/'+family+'/source-'+name,lambda v=v:source_match(v,r,s,family))
            for name,mutate in [('status',lambda v:v.update(status='unrecognized')),('metadata-extra',lambda v:v.update(unplanned=True)),('ID-scalar-type',lambda v:v.update(id=float(v['id']))),
                                ('range-source',lambda v:v['field_values']['values'].append(dict(name='invented',value=0,origin='pdu',pdu_range=dict(offset=0,length=1)))),
                                ('raw-field-tree',lambda v:v.update(fields={'Type':0})),('snapshot-source',lambda v:v['field_values']['source'].update(Kind='decrypted'))]:
                v=copy.deepcopy(e);mutate(v)
                rejects(corpus+'/'+family+'/whole-'+name,lambda v=v:(_ for _ in ()).throw(AssertionError())if not same(e,v)else None)
            seen=set();valid_id(e,seen);rejects(corpus+'/'+family+'/duplicate-publisher-ID',lambda:valid_id(e,seen))
            forged=copy.deepcopy(e);forged['id']=True;rejects(corpus+'/'+family+'/bool-publisher-ID',lambda:valid_id(forged,set()))
        # Real baseline event references are tested under a nonidentity map;
        # flow IDs, physical frame numbers, Raw and domains remain literal.
        data=a.before/'preservation'/corpus/'events.jsonl';mapping={};examples=[]
        for line in data.open(encoding='utf8'):
            e=json.loads(line);mapping[e['id']]=e['id']+1000000
            if len(examples)<4:examples.append(e)
        reverse={v:k for k,v in mapping.items()};assert len(reverse)==len(mapping)
        for index,e in enumerate(examples):
            translated=reference.remap_known_pdu_references(e,mapping);restored=reference.remap_known_pdu_references(translated,reverse)
            assert same(restored,e)
            for key in('raw','domain','flow','source','destination'):assert same(e[key],translated[key])
            assert same(e['source_bytes']['PacketRefs'],translated['source_bytes']['PacketRefs'])
            positive.append(corpus+'/actual-baseline-reference-nonidentity-'+str(index))
        e=examples[0];bad=dict(mapping);bad[e['id']]=True;rejects(corpus+'/mapped-ID-bool',lambda:reference.remap_known_pdu_references(e,bad))
        bad=dict(mapping);del bad[e['id']];rejects(corpus+'/dangling-ID',lambda:reference.remap_known_pdu_references(e,bad))
        assert identity(examples[0])!=identity(examples[1]);positive.append(corpus+'/distinct-publisher-source-identity')
    summary=dict(scope=__doc__,positive_controls=positive,negative_controls=negative,plan_SHA256=sha(a.plan/'summary.json'),bridge_SHA256=sha(Path(__file__).with_name('bridge.py')),auditor_SHA256=sha(__file__),
                 IGMP_constructor_SHA256=sha(a.igmp_modules/'igmp_native_expected.py'),MLD_constructor_SHA256=sha(a.mld_modules/'mld_native_expected.py'))
    f=a.out/'summary.json';f.write_text(json.dumps(summary,indent=2,sort_keys=True)+'\n');print(json.dumps(dict(summary=str(f),SHA256=sha(f),positive=len(positive),negative=len(negative))))


if __name__=='__main__':main()
