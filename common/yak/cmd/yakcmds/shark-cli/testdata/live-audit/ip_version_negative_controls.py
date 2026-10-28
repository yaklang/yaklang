"""Require the original-wire IP-version auditor to reject forged evidence.

Usage: ip_version_negative_controls.py --source SOURCE --capture CAPTURE
--manifest MANIFEST --events SDK --cli CLI --oracle JSON --labels TSV --out DIR
Only external copies are corrupted. Original bytes, metadata and receipts stay
intact. These controls validate source/type/projection checks, not every parser.
"""
import argparse,copy,hashlib,json
from pathlib import Path
from ip_version_source_audit import audit


def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def replace(tree,key,value):
    if isinstance(tree,dict):
        if key in tree:tree[key]=value;return True
        return any(replace(v,key,value)for v in tree.values())
    if isinstance(tree,list):return any(replace(v,key,value)for v in tree)
    return False
def run(a):
    for p in vars(a).values():assert not any((v/'.git').exists()for v in[p.resolve(),*p.resolve().parents])
    audit(a.source,a.capture,a.manifest,a.events,a.cli,a.oracle,a.labels)
    originals=dict(events=[json.loads(l)for l in a.events.open(encoding='utf8')],cli=[json.loads(l)for l in a.cli.open(encoding='utf8')],oracle=json.loads(a.oracle.read_text('utf8')),manifest=json.loads(a.manifest.read_text('utf8')))
    a.out.mkdir(parents=True,exist_ok=False);results=[]
    for name in['wire-byte-with-updated-hash','source-time','source-domain','duplicate-source-frame','boolean-PDU-ID','observed-version','child-header-splice','false-flow','CLI-fragment-label','CLI-application-stack','CLI-connection-identity','reference-version','reference-expert']:
        d=copy.deepcopy(originals);wire=bytearray(a.capture.read_bytes());e=d['events'][0];row=next(r for r in d['cli']if r.get('record_type')!='protocol_publication');changed=e['source_bytes']['PacketRefs'][0]['Number']
        if name=='wire-byte-with-updated-hash':wire[24+16+14]^=0x10;d['manifest']['capture_SHA256']=hashlib.sha256(wire).hexdigest()
        elif name=='source-time':d['manifest']['ledger'][0]['time_ns']+=1
        elif name=='source-domain':e['domain']['Interface']+=1
        elif name=='duplicate-source-frame':
            e['source_bytes']['PacketRefs'][0]['Number']=changed+1;e['field_values']['source']=copy.deepcopy(e['source_bytes'])
        elif name=='boolean-PDU-ID':
            e['id']=True;e['field_values']['pdu']=True
            for v in e['field_values']['values']:
                if v['name']=='pdu.id':v['value']=True
        elif name=='observed-version':e['session']['Observed IP Version']=4
        elif name=='child-header-splice':e['session']['Observed Header Layers']+=['TCP','TLS']
        elif name=='false-flow':e['flow']=1;e['tcp_flow_id']=1
        elif name=='CLI-fragment-label':row['protocol']='Fragment'
        elif name=='CLI-application-stack':row['protocol_stack']+=['TCP','TLS']
        elif name=='CLI-connection-identity':row['connection_protocol']='TLS'
        elif name=='reference-version':assert replace(d['oracle'][0],'ip.version','4')
        elif name=='reference-expert':
            def strip(v):
                if isinstance(v,dict):
                    if'_ws.expert.message'in v:v['_ws.expert.message']='forged: no version diagnostic'
                    for w in v.values():strip(w)
                elif isinstance(v,list):
                    for w in v:strip(w)
            strip(d['oracle'][0])
        dest=a.out/name;dest.mkdir();cp=dest/'observation.pcap';cp.write_bytes(wire)
        paths={}
        for kind in['events','cli','oracle','manifest']:
            path=dest/(kind+('.jsonl'if kind in['events','cli']else'.json'));paths[kind]=path
            with path.open('w',encoding='utf8',newline='\n')as f:
                if kind in['events','cli']:
                    for value in d[kind]:f.write(json.dumps(value)+'\n')
                else:json.dump(d[kind],f)
        try:audit(a.source,cp,paths['manifest'],paths['events'],paths['cli'],paths['oracle'],a.labels)
        except(AssertionError,KeyError,ValueError)as error:reason=type(error).__name__+': '+str(error)
        else:raise AssertionError((name,'forged evidence accepted'))
        results.append(dict(control=name,rejected=True,reason=reason,mutated_capture_SHA256=sha(cp),mutated_inputs_SHA256={k:sha(v)for k,v in paths.items()}))
    proof=dict(scope=__doc__,auditor_SHA256=sha(Path(__file__)),rejected=len(results),controls=results)
    (a.out/'proof.json').write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8');print(json.dumps(dict(rejected=len(results),all_rejected=True)),flush=True)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for n in['source','capture','manifest','events','cli','oracle','labels','out']:ap.add_argument('--'+n,type=Path,required=True)
    run(ap.parse_args())
