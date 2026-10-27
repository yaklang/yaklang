"""Compare every CLI row while certifying one real SSH-control diagnostic correction.
Bind the selected physical row and endpoints to the independent SDK/raw/refusal
proof. Permit only preserving its malformed outcome instead of overwriting it
with connection-associated missing-context text. All other row values stay exact.
"""
import argparse,copy,hashlib,itertools,json
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for name in ['before','after','manifest','SDK-proof','out']:ap.add_argument('--'+name,type=Path,required=True)
a=ap.parse_args()
for path in vars(a).values():assert not any((p/'.git').exists()for p in[path.resolve(),*path.resolve().parents])
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest();m=json.loads(a.manifest.read_text());proof=json.loads(a.SDK_proof.read_text());assert proof['manifest_sha256']==sha(a.manifest)and len(proof['changed_records'])==1;target=m['change'];assert proof['changed_records'][0]['physical_frame']==target['physical_frame']and proof['changed_records'][0]['raw_sha256']==target['mutated_PDU_sha256']
def unique(items):
 d={}
 for k,v in items:assert k not in d,'duplicate JSON key';d[k]=v
 return d
def invalid(v):raise ValueError('non-JSON constant: '+v)
def rows(path):
 for l in path.open(encoding='utf-8-sig'):yield json.loads(l,object_pairs_hook=unique,parse_constant=invalid)
def canonical(v):return json.dumps(v,sort_keys=True,separators=(',',':'),ensure_ascii=True)
count=0;changed=[]
for b,n in itertools.zip_longest(rows(a.before),rows(a.after)):
 assert b is not None and n is not None;count+=1;expected=copy.deepcopy(b)
 if canonical(b)!=canonical(n):
  assert b['number']==target['physical_frame'];assert b['protocol']==b['connection_protocol']=='SSH'and b['transport']=='tcp';assert f'{b["source"]}:{b["source_port"]}'==target['source']and f'{b["destination"]}:{b["destination_port"]}'==target['destination'];assert b['protocol_status']=='context-required'and b['protocol_scope']=='connection-associated bytes'and b['visibility']=='unassembled-stream';assert b['protocol_evidence']=='observed connection; this segment has no complete decoded message'
  # SDK and CLI identity namespaces are not assumed equal. Match literal
  # source SHA and physical ownership, then keep each namespace unchanged.
  pdus=[p for p in b['pdus']if p['byte_source']['SHA256']==target['mutated_PDU_sha256']];assert len(pdus)==1 and pdus[0]['protocol']=='ssh'and pdus[0]['status']=='malformed';assert pdus[0]['byte_source']['PacketRefs']==[{'Number':target['physical_frame'],'Domain':{'Section':0,'Interface':0,'Encapsulation':''}}]
  prefix='SSH stream bytes · no complete decoded message in this packet · ';assert b['info'].startswith(prefix);expected['info']=b['info'][len(prefix):];assert expected['info'].startswith('SSH · MalformedMessage · ssh-plaintext: Ed25519 Signature Bytes requires 64 bytes')
  expected['protocol_status']='malformed';expected['protocol_scope']='connection-associated diagnostic';expected['protocol_evidence']='protocol parser diagnostic';del expected['visibility'];changed.append({'physical_frame':b['number'],'status_before':'context-required','status_after':'malformed','source_and_protocol_identity_unchanged':True})
 assert canonical(expected)==canonical(n),('unexpected full row difference',count,b['number'])
assert count==m['physical_frames']and len(changed)==1
result={'scope':__doc__,'records_compared':count,'changed_records':changed,'before_sha256':sha(a.before),'after_sha256':sha(a.after),'manifest_sha256':sha(a.manifest),'SDK_projection_proof_sha256':sha(a.SDK_proof),'all_other_values_types_order_and_IDs_exact':True,'identity_or_field_normalization':False};a.out.parent.mkdir(parents=True,exist_ok=True);a.out.write_text(json.dumps(result,indent=2)+'\n');print(json.dumps(result))
