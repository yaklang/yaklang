"""Certify one SSH refusal-only field-query correction in complete exports.

The declared actual-capture control changes one signature string length, not
its containing packet or byte count. Independently bind exact SDK raw bytes to
the selected physical Wireshark view and inspect the nested strings. Wireshark
may retain this signature as opaque; its lack of an expert warning is not an
RFC8709 length-conformance verdict. Every other complete record stays exact.
"""
import argparse,base64,copy,hashlib,itertools,json
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for name in ['before','after','manifest','oracle','out']:ap.add_argument('--'+name,type=Path,required=True)
a=ap.parse_args()
for path in vars(a).values():assert not any((p/'.git').exists()for p in[path.resolve(),*path.resolve().parents])
def sha(path):
 with path.open('rb')as f:return hashlib.file_digest(f,'sha256').hexdigest()
def canonical(v):return json.dumps(v,sort_keys=True,separators=(',',':'),ensure_ascii=True)
def unique(items):
 d={}
 for k,v in items:assert k not in d,'duplicate JSON key';d[k]=v
 return d
def invalid(v):raise ValueError('non-JSON constant: '+v)
def rows(path):
 for line in path.open(encoding='utf-8-sig'):yield json.loads(line,object_pairs_hook=unique,parse_constant=invalid)
def leaves(o,key):
 if isinstance(o,dict):
  for k,v in o.items():
   if k==key:yield from(v if isinstance(v,list)else[v])
   elif isinstance(v,(dict,list)):yield from leaves(v,key)
 elif isinstance(o,list):
  for v in o:yield from leaves(v,key)
m=json.loads(a.manifest.read_text());assert m['changed_frame_count']==1;target=m['change'];assert target['signature_string_length_before']==64 and target['signature_string_length_after']==4
oracle=json.loads(a.oracle.read_text(encoding='utf-8-sig'));reference=next(row['_source']['layers']for row in oracle if int(row['_source']['layers']['frame']['frame.number'])==target['physical_frame']);raw_views=reference['ssh_raw'];raw_views=[raw_views]if isinstance(raw_views[0],str)else raw_views
count=0;changes=[]
for before,after in itertools.zip_longest(rows(a.before),rows(a.after)):
 assert before is not None and after is not None,'ordered record count';count+=1;expected=copy.deepcopy(before)
 if canonical(before)!=canonical(after):
  assert before['protocol']=='ssh'and before['status']=='malformed'and before['entry']=='SSHPacket';raw=base64.b64decode(before['raw'],validate=True);assert hashlib.sha256(raw).hexdigest()==target['mutated_PDU_sha256'];assert len(raw)==int.from_bytes(raw[:4],'big')+4 and len(raw)%8==0 and raw[5]==31
  assert before['source']==target['source']and before['destination']==target['destination'];assert before['domain']=={'Section':0,'Interface':0,'Encapsulation':''};refs=before['source_bytes']['PacketRefs'];assert refs==[{'Number':target['physical_frame'],'Domain':before['domain']}]
  # CompletedIn is optional for refused observations. Bind the exact owned
  # contributors without manufacturing a successful completion marker.
  if before['source_bytes'].get('CompletedIn')is not None:assert before['source_bytes']['CompletedIn']==refs[-1]
  assert bytes.fromhex(next(v[0]for v in raw_views if bytes.fromhex(v[0])==raw))==raw,'exact selected reference PDU bytes'
  sport=int(next(leaves(reference['tcp'],'tcp.srcport')));dport=int(next(leaves(reference['tcp'],'tcp.dstport')));assert f'{next(leaves(reference["ip"],"ip.src"))}:{sport}'==target['source'];assert f'{next(leaves(reference["ip"],"ip.dst"))}:{dport}'==target['destination']
  pos=6;end=len(raw)-raw[4]
  def string():
   global pos
   assert pos+4<=end;n=int.from_bytes(raw[pos:pos+4],'big');pos+=4;assert n<=end-pos;v=raw[pos:pos+n];pos+=n;return v
  host=string();exchange=string();signature=string();assert pos==end
  n=int.from_bytes(signature[:4],'big');assert signature[4:4+n]==b'ssh-ed25519';assert int.from_bytes(signature[4+n:8+n],'big')==4;available=signature[8+n:];assert len(available)==64
  assert 'ssh-ed25519'in list(leaves(reference['ssh'],'ssh.host_sig.type'));assert (len(signature))in [int(v)for v in leaves(reference['ssh'],'ssh.host_sig.length')]
  opaque=signature[4+n:];assert opaque in [bytes.fromhex(v.replace(':',''))for v in leaves(reference['ssh'],'ssh.host_sig.data')],'opaque reference signature container agrees; not a fixed-length signature verdict'
  assert before['fields']['Payload']['Message Number']==31 and'Opaque Message Data'in before['fields']['Payload'];assert 'fields_error'not in before
  assert before['error'].startswith('MalformedMessage:')and'Ed25519 Signature Bytes requires 64 bytes'in before['error'];expected['fields']=None;expected['fields_error']=before['error']
  changes.append({'PDU':after['id'],'physical_frame':target['physical_frame'],'raw_sha256':target['mutated_PDU_sha256'],'declared_signature_bytes':4,'remaining_signature_octets':len(available),'reference_profile':'opaque signature container; no inner signature length conformance assertion'})
 assert canonical(expected)==canonical(after),('unexpected complete-record difference',count,after.get('id'))
assert len(changes)==1,'one exact refusal projection correction required'
result={'scope':__doc__,'records_compared':count,'changed_records':changes,'before_sha256':sha(a.before),'after_sha256':sha(a.after),'manifest_sha256':sha(a.manifest),'oracle_sha256':sha(a.oracle),'all_other_values_types_order_and_IDs_exact':True,'identity_or_field_normalization':False,'normative_length_source':'https://www.rfc-editor.org/rfc/rfc8709.html#section-6'};a.out.parent.mkdir(parents=True,exist_ok=True);a.out.write_text(json.dumps(result,indent=2)+'\n');print(json.dumps(result))
