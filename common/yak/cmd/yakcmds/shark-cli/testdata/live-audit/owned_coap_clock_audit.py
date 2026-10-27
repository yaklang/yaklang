"""Check six actual owned CoAP UDP messages, fields and endpoint receipts.

This covers the declared CON GET / ACK Content profile and its three tokens;
blockwise transfers, Observe and complete CoAP feature parity remain separate.
"""
import argparse,base64,collections,hashlib,json
from pathlib import Path
from owned_quic_clock_audit import datagrams,vals,number


def sha(w):return hashlib.sha256(w).hexdigest()
def audit(capture,events,manifest,oracle):
    peer=json.loads(manifest.read_text('utf8'));assert peer['capture_ready_observed']and len(peer['operations'])==3
    expected={}
    for op in peer['operations']:
        for direction,name in[(0,'request'),(1,'reply')]:
            raw=bytes.fromhex(op[name+'_hex']);assert sha(raw)==op[name+'_SHA256'];expected[sha(raw)]=(direction,op,raw)
    packets,by_ref=datagrams(capture);native=collections.Counter();reference=collections.Counter();flows=set();checks=0;references=[]
    ws={number(r['_source']['layers']['frame']['frame.number']):r['_source']['layers']for r in json.loads(oracle.read_text('utf8'))if'coap'in r['_source']['layers']}
    for line in events.open(encoding='utf8'):
        e=json.loads(line)
        if e['protocol']!='coap':continue
        assert e['status']=='decoded'and e['source_bytes']['SHA256']in expected
        direction,op,raw=expected[e['source_bytes']['SHA256']];assert e['direction']==direction and base64.b64decode(e['raw'])==raw
        refs=e['source_bytes']['PacketRefs'];assert len(refs)==1;p=by_ref[refs[0]['Number']]
        assert p['payload']==raw and e['domain']==refs[0]['Domain']==dict(Section=p['domain'][0],Interface=p['domain'][1],Encapsulation='')
        assert p['source_port']==peer['client'if direction==0 else'server'][1]and p['destination_port']==peer['server'if direction==0 else'client'][1]
        fields=e['fields'];s=e['session'];q=ws[p['frame']]['coap'];assert fields['Version']==number(q['coap.version'])==raw[0]>>6==1
        for native_name,ref_name,value in[('Type','coap.type',(raw[0]>>4)&3),('Code','coap.code',raw[1]),('Message ID','coap.mid',int.from_bytes(raw[2:4],'big')),('Token Length','coap.token_len',raw[0]&15)]:
            assert fields[native_name]==number(q[ref_name])==value;checks+=1
        token=raw[4:4+(raw[0]&15)];assert base64.b64decode(fields['Token'])==base64.b64decode(s['Token'])==bytes.fromhex(q['coap.token'].replace(':',''))==bytes.fromhex(op['token_hex'])==token
        assert s['Code Value']==raw[1]and s['Type']==fields['Type']and s['Message ID']==op['message_id']
        if direction==0:assert '/'+s['Uri-Path']==q['coap.opt.uri_path_recon']==op['URI']
        else:
            body=raw.split(b'\xff',1)[1];assert sha(body)==op['body_SHA256']and s['Payload Bytes']==len(body)and s['In Reply To']=='GET'
        native[sha(raw)]+=1;flows.add(e['flow']);references.append(dict(PDU=e['id'],frame=p['frame'],raw_SHA256=sha(raw)));checks+=5
    for p in packets:
        if p['frame']in ws:reference[sha(p['payload'])]+=1
    assert native==reference==collections.Counter({k:1 for k in expected})and len(flows)==1
    return dict(scope=__doc__,capture_SHA256=sha(capture.read_bytes()),SDK_SHA256=sha(events.read_bytes()),manifest_SHA256=sha(manifest.read_bytes()),oracle_SHA256=sha(oracle.read_bytes()),auditor_SHA256=sha(Path(__file__).read_bytes()),messages=sum(native.values()),connections=len(flows),literal_field_checks=checks,all_wire_bytes_tokens_URI_and_payloads_exact=True,source_bound_observations=references,numeric_response_PDU_link_coverage=False,full_CoAP_feature_parity_claim=False)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for n in['capture','events','manifest','oracle','out']:ap.add_argument('--'+n,type=Path,required=True)
    a=ap.parse_args()
    for path in vars(a).values():assert not any((p/'.git').exists()for p in[path.resolve(),*path.resolve().parents])
    proof=audit(a.capture,a.events,a.manifest,a.oracle)
    with a.out.open('x',encoding='utf8',newline='\n')as out:json.dump(proof,out,indent=2);out.write('\n')
    print(json.dumps(proof),flush=True)
