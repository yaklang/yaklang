"""Reject source/flow/handshake/plaintext forgeries in the TLS resumption audit."""
import argparse,base64,copy,hashlib,json
from pathlib import Path
from tls_resumption_audit import audit


def run(root,out):
    out.mkdir(parents=True,exist_ok=False)
    original=dict(capture=root/'capture-01/live.pcap',events=root/'replay-01/events.jsonl',cli=root/'replay-01/yak.jsonl',flows=root/'replay-01/flows.json',manifest=root/'capture-01/manifest.json',oracle=root/'replay-01/oracle.json',labels=root/'replay-01/labels.tsv')
    positive=audit(**original)
    events=[json.loads(l)for l in original['events'].open(encoding='utf8')];cli=[json.loads(l)for l in original['cli'].open(encoding='utf8')];flows=json.loads(original['flows'].read_text('utf8'));peer=json.loads(original['manifest'].read_text('utf8'));reference=json.loads(original['oracle'].read_text('utf8'))
    cases=[]
    def add(name,kind,mutate):cases.append((name,kind,mutate))
    def tls(v):return next(e for e in v if e['protocol']=='tls'and e['tcp_flow_id']==2 and(e['session']or{}).get('Authentication Verified')is True)
    def request(v):return next(e for e in v if e['protocol']=='http'and e['tcp_flow_id']==2 and e['direction']==0)
    def response(v):return next(e for e in v if e['protocol']=='http'and e['tcp_flow_id']==2 and e['direction']==1)
    def setkey(tree,name,value):
        if isinstance(tree,dict):
            if name in tree:tree[name]=value;return True
            return any(setkey(v,name,value)for v in tree.values())
        if isinstance(tree,list):return any(setkey(v,name,value)for v in tree)
        return False
    add('native-foreign-generation-reference','events',lambda v:tls(v)['source_bytes']['PacketRefs'][0].update(Number=6))
    add('native-foreign-source-domain','events',lambda v:tls(v)['domain'].update(Interface=1))
    add('native-boolean-direction','events',lambda v:tls(v).update(direction=True))
    add('native-boolean-flow','events',lambda v:tls(v).update(tcp_flow_id=True))
    add('native-wrong-negotiated-version','events',lambda v:tls(v)['session'].update({'Negotiated Version':772}))
    add('native-wrong-cipher','events',lambda v:tls(v)['session'].update({'Cipher Suite':4865}))
    add('native-unoffered-ALPN','events',lambda v:tls(v)['session'].update(ALPN='h2'))
    def unauthenticated_parent(v):
        e=response(v);parent=next(p for p in v if p['id']==e['source_bytes']['ParentPDUs'][0]);parent['session']['Authentication Verified']=False
    add('native-plaintext-from-unauthenticated-parent','events',unauthenticated_parent)
    add('native-response-to-previous-generation','events',lambda v:response(v).update(response_to=next(e['id']for e in v if e['protocol']=='http'and e['tcp_flow_id']==1 and e['direction']==0)))
    add('native-request-field-uri-forgery','events',lambda v:request(v)['fields']['Message']['HTTP Request']['FirstLine'].update(Path='/foreign'))
    add('native-response-body-field-forgery','events',lambda v:response(v)['fields']['Message']['HTTP Response']['Body'].update(Octets='changed'))
    def changed_raw(v):
        e=tls(v);raw=bytearray(base64.b64decode(e['raw']));raw[-1]^=1;e['raw']=base64.b64encode(raw).decode();e['source_bytes']['SHA256']=hashlib.sha256(raw).hexdigest()
    add('native-self-consistent-wrong-record-bytes','events',changed_raw)
    add('peer-false-resumption-receipt','manifest',lambda v:next(r for r in v['peer_receipts']if r['profile']=='tls13'and r['generation']==1 and r['peer']=='server').update(session_reused=False))
    add('native-flow-packet-owned-by-old-generation','flows',lambda v:next(p for p in v['packets']if p['flow']==2).update(flow=1))
    add('reference-wrong-TCP-generation','oracle',lambda v:setkey(v[35]['_source']['layers'],'tcp.stream','0'))
    add('reference-wrong-selected-PSK','oracle',lambda v:setkey(v[145]['_source']['layers'],'tls.handshake.extensions.psk.identity.selected','1'))
    add('reference-frame-source-forgery','oracle',lambda v:v[35]['_source']['layers']['frame_raw'].__setitem__(0,'00'+v[35]['_source']['layers']['frame_raw'][0][2:]))
    add('CLI-wrong-wire-length','cli',lambda v:v[35].update(length=v[35]['length']+1))
    add('CLI-false-negotiated-label-on-ClientHello','cli',lambda v:v[5].update(protocol='TLSv1.3'))
    results=[]
    originals=dict(events=events,cli=cli,flows=flows,manifest=peer,oracle=reference)
    for name,kind,mutate in cases:
        candidate=copy.deepcopy(originals[kind]);changed=mutate(candidate);assert json.dumps(candidate,sort_keys=True)!=json.dumps(originals[kind],sort_keys=True),name
        case=out/name;case.mkdir();target=case/(kind+('.jsonl'if kind in['events','cli']else'.json'))
        target.write_text((''.join(json.dumps(v)+'\n'for v in candidate)if kind in['events','cli']else json.dumps(candidate)),encoding='utf8')
        args=dict(original);args[kind]=target
        try:audit(**args)
        except(AssertionError,KeyError,ValueError)as exc:results.append(dict(case=name,kind=kind,rejected=True,rejection=type(exc).__name__,forgery_SHA256=hashlib.sha256(target.read_bytes()).hexdigest()))
        else:raise AssertionError('forgery accepted: '+name)
    proof=dict(scope=__doc__,auditor_SHA256=hashlib.sha256((Path(__file__).parent/'tls_resumption_audit.py').read_bytes()).hexdigest(),negative_driver_SHA256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),baseline_capture_SHA256=positive['capture_SHA256'],positive_original_passed=True,negative_controls=len(results),all_forgeries_rejected=True,cases=results)
    (out/'summary.json').write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8');(out/'positive-proof.json').write_text(json.dumps(positive,indent=2)+'\n',encoding='utf8')
    print(json.dumps(dict(positive_original_passed=True,negative_controls=len(results),all_forgeries_rejected=True)),flush=True)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('root',type=Path);ap.add_argument('out',type=Path);a=ap.parse_args()
    for p in[a.root,a.out]:assert not any((v/'.git').exists()for v in[p.resolve(),*p.resolve().parents])
    run(a.root,a.out)
