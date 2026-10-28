"""Negative controls for exact TLS omissions, retained values and peer ownership."""
import argparse,base64,copy,hashlib,json,shutil,struct
from pathlib import Path
from tls_resumption_control_audit import audit


def run(root,out):
    out.mkdir(parents=True,exist_ok=False);name='tls13-reordered-server-records';case=root/'controls-01'/name;replay=root/'control-replay-01'/name;reference=root/'reference-reorder-01'/name
    source=root/'capture-01/live.pcap';peer=root/'capture-01/manifest.json';baseline=root/'replay-01'
    positive=audit(source,peer,baseline,case,replay,reference);assert positive['display_projection_correct']
    original=dict(events=[json.loads(l)for l in(replay/'events.jsonl').open(encoding='utf8')],flows=json.loads((replay/'flows.json').read_text('utf8')),oracle=json.loads((reference/'oracle.json').read_text('utf8')),manifest=json.loads((case/'manifest.json').read_text('utf8')))
    def e(v,protocol='http',cid=6,direction=1):return next(e for e in v if e['protocol']==protocol and e['tcp_flow_id']==cid and e['direction']==direction)
    def recursive_set(tree,key,value):
        if isinstance(tree,dict):
            if key in tree:tree[key]=value;return True
            return any(recursive_set(v,key,value)for v in tree.values())
        if isinstance(tree,list):return any(recursive_set(v,key,value)for v in tree)
        return False
    cases=[
        ('unaffected-native-ALPN-forgery','events',lambda v:e(v,'tls',cid=1)['session'].update(ALPN='h2')),
        ('target-foreign-generation-flow','events',lambda v:e(v).update(tcp_flow_id=5)),
        ('target-boolean-direction','events',lambda v:e(v).update(direction=True)),
        ('target-wrong-HTTP-transaction','events',lambda v:e(v).update(transaction=15)),
        ('target-response-to-old-generation','events',lambda v:e(v).update(response_to=15)),
        ('target-false-HTTP-field-path','events',lambda v:e(v,direction=0)['fields']['Message']['HTTP Request']['FirstLine'].update(Path='/foreign')),
        ('target-source-ref-splice','events',lambda v:e(v)['source_bytes']['PacketRefs'][0].update(Number=6)),
        ('target-source-domain-boolean','events',lambda v:e(v)['source_bytes']['PacketRefs'][0]['Domain'].update(Interface=False)),
        ('flow-observer-old-generation-owner','flows',lambda v:next(p for p in v['packets']if p['flow']==6).update(flow=5)),
        ('forged-ledger-source-frame','manifest',lambda v:v['ledger'][0].update(source_frame=2)),
        ('forged-target-generation','manifest',lambda v:v.update(target_original_connection=5)),
        ('reference-false-request-path','oracle',lambda v:recursive_set(v,'http.request.uri','/foreign')),
        ('reference-false-response-code','oracle',lambda v:recursive_set(v,'http.response.code','500')),
    ]
    def false_parent(v):
        child=e(v);pdu=child['source_bytes']['ParentPDUs'][0];next(p for p in v if p['id']==pdu)['session']['Authentication Verified']=False
    cases.append(('target-plaintext-from-unauthenticated-record','events',false_parent))
    def wrong_parent(v):
        child=e(v);child['source_bytes']['ParentPDUs']=[next(p['id']for p in v if p['protocol']=='tls'and p['tcp_flow_id']==5)]
    cases.append(('target-foreign-authenticated-parent','events',wrong_parent))
    cases.extend([('self-consistent-altered-retained-wire','wire',None),('self-consistent-altered-retained-time','time',None)])
    results=[]
    for title,kind,mutate in cases:
        dest=out/title;dest.mkdir();candidate_case=dest/'case';candidate_case.mkdir();candidate_replay=dest/'replay';candidate_replay.mkdir();candidate_reference=dest/'reference';candidate_reference.mkdir()
        for filename in['manifest.json','observation.pcap']:shutil.copyfile(case/filename,candidate_case/filename)
        for filename in['events.jsonl','yak.jsonl','flows.json']:shutil.copyfile(replay/filename,candidate_replay/filename)
        for filename in['oracle.json','oracle-producer.json']:shutil.copyfile(reference/filename,candidate_reference/filename)
        if kind in original:
            value=copy.deepcopy(original[kind]);mutate(value);assert json.dumps(value,sort_keys=True)!=json.dumps(original[kind],sort_keys=True)
            path=candidate_case/'manifest.json'if kind=='manifest'else candidate_reference/'oracle.json'if kind=='oracle'else candidate_replay/(kind+('.jsonl'if kind=='events'else'.json'))
            path.write_text(''.join(json.dumps(e)+'\n'for e in value)if kind=='events'else json.dumps(value),encoding='utf8')
        else:
            path=candidate_case/'observation.pcap';wire=bytearray(path.read_bytes())
            if kind=='wire':wire[24+16+4+20+7]^=1
            else:struct.pack_into('<I',wire,28,struct.unpack_from('<I',wire,28)[0]+1)
            path.write_bytes(wire);doc=json.loads((candidate_case/'manifest.json').read_text('utf8'));doc['capture_SHA256']=hashlib.sha256(wire).hexdigest();(candidate_case/'manifest.json').write_text(json.dumps(doc),encoding='utf8')
        try:audit(source,peer,baseline,candidate_case,candidate_replay,candidate_reference)
        except(AssertionError,KeyError,ValueError,StopIteration)as exc:results.append(dict(case=title,kind=kind,rejected=True,rejection=type(exc).__name__))
        else:raise AssertionError('forgery accepted: '+title)
    proof=dict(scope=__doc__,positive_source_original_passed=True,positive_scope=positive['verification_result'],auditor_SHA256=positive['auditor_SHA256'],negative_driver_SHA256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),negative_controls=len(results),all_forgeries_rejected=True,cases=results)
    (out/'summary.json').write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8');print(json.dumps(dict(positive_original_passed=True,negative_controls=len(results),all_forgeries_rejected=True)),flush=True)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('root',type=Path);ap.add_argument('out',type=Path);a=ap.parse_args()
    for p in[a.root,a.out]:assert not any((v/'.git').exists()for v in[p.resolve(),*p.resolve().parents])
    run(a.root,a.out)
