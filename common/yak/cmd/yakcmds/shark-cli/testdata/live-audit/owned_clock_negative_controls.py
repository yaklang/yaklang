"""Require independent owned TLS/QUIC auditors to reject forged evidence.

Usage: owned_clock_negative_controls.py EXTERNAL_SHARED_CLOCK_ROOT OUTPUT_DIR
All deliberately corrupted copies and original capture secrets stay outside Git.
"""
import base64,copy,hashlib,json,sys
from pathlib import Path
from owned_quic_clock_audit import audit as quic,vals,many
from owned_tls_clock_audit import audit as tls


def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def change_field(tree,key,value):
    if isinstance(tree,dict):
        if key in tree:tree[key]=value;return True
        return any(change_field(v,key,value)for v in tree.values())
    if isinstance(tree,list):return any(change_field(v,key,value)for v in tree)
    return False


def run(root,out):
    for p in[root,out]:assert not any((v/'.git').exists()for v in[p.resolve(),*p.resolve().parents])
    out.mkdir(parents=True,exist_ok=False);results=[]
    for kind,auditor in[('TLS',tls),('QUIC',quic)]:
        case='fresh-'+kind;capture=root/(case+'-01')/'live.pcap';manifest=root/(case+'-01')/'manifest.json';events=root/'replay-after-02'/case/'events.jsonl';oracle=root/'oracles-02'/case/'oracle.json'
        auditor(capture,events,manifest,oracle)
        original_events=[json.loads(l)for l in events.open(encoding='utf8')];original_reference=json.loads(oracle.read_text('utf8'))
        names=['wire-byte','capture-domain','source-frame','header-alias']
        names+=['plaintext-byte','parent-authentication']if kind=='TLS'else['STREAM-offset','HTTP3-header','padding-authentication','coalesced-connection']
        for name in names:
            es=copy.deepcopy(original_events);rs=copy.deepcopy(original_reference)
            e=next(e for e in es if e['protocol']==kind.lower()and e['status']=='decoded'and(e.get('session')or{}).get('Authentication Verified')is True)
            if name=='wire-byte':
                w=bytearray(base64.b64decode(e['raw']));w[-1]^=1;e['raw']=base64.b64encode(w).decode();e['source_bytes']['SHA256']=hashlib.sha256(w).hexdigest()
            elif name=='capture-domain':e['domain']['Interface']+=1
            elif name=='source-frame':e['source_bytes']['PacketRefs'][0]['Number']+=1
            elif name=='header-alias':
                field='tls.record.length'if kind=='TLS'else'quic.packet_number'
                v=next(v for v in e['field_values']['values']if v['name']==field);v['value']+=1
            elif name=='plaintext-byte':
                h=next(e for e in es if e['protocol']=='http'and e['direction']==1);w=bytearray(base64.b64decode(h['raw']));w[-1]^=1;h['raw']=base64.b64encode(w).decode();h['source_bytes']['SHA256']=hashlib.sha256(w).hexdigest()
            elif name=='parent-authentication':
                h=next(e for e in es if e['protocol']=='http');parent=h['source_bytes']['ParentPDUs'][0];next(e for e in es if e['id']==parent)['session']['Authentication Verified']=False
            elif name=='STREAM-offset':
                f=next(f for e in es for f in(e.get('session')or{}).get('Frames',[])if f.get('Frame Type')=='STREAM'and not f['Stream ID']&2);f['Offset']+=1
            elif name=='HTTP3-header':
                f=next(h for e in es for f in(e.get('session')or{}).get('Frames',[])for h in f.get('HTTP3 Frames',[])if h.get('Headers'));f['Headers'][0]['Value']+='-forged'
            elif name=='padding-authentication':next(e for e in es if e['completeness']=='datagram-padding')['session']['Authentication Verified']=True
            elif name=='coalesced-connection':
                qs=next(many(r['_source']['layers']['quic'])for r in rs if len(many(r['_source']['layers'].get('quic',[])))>1 and any(vals(q,'quic.connection.number')for q in many(r['_source']['layers'].get('quic',[])))and any(vals(q,'quic.stream.stream_id')and not vals(q,'quic.connection.number')for q in many(r['_source']['layers'].get('quic',[]))))
                q=next(q for q in qs if vals(q,'quic.stream.stream_id')and not vals(q,'quic.connection.number'));q['QUIC Connection information']={'quic.connection.number':'999999'}
            dest=out/(kind+'-'+name);dest.mkdir();ep=dest/'events.jsonl';rp=dest/'oracle.json'
            with ep.open('w',encoding='utf8',newline='\n')as f:
                for e in es:f.write(json.dumps(e)+'\n')
            rp.write_text(json.dumps(rs),encoding='utf8')
            try:auditor(capture,ep,manifest,rp)
            except(AssertionError,KeyError,ValueError)as error:reason=type(error).__name__+': '+str(error)
            else:raise AssertionError((kind,name,'forged evidence was accepted'))
            results.append(dict(protocol=kind,control=name,rejected=True,reason=reason,mutated_SDK_SHA256=sha(ep),mutated_reference_SHA256=sha(rp)))
    proof=dict(scope=__doc__,auditor_SHA256=sha(Path(__file__)),rejected=len(results),controls=results)
    (out/'proof.json').write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8')
    print(json.dumps(dict(rejected=len(results),all_rejected=True)),flush=True)


if __name__=='__main__':run(*map(Path,sys.argv[1:3]))
