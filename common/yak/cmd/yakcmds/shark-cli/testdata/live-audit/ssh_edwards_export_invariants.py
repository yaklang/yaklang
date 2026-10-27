"""Permit only independently certified SSH Edwards inner-field expansion.

Keep every ordered SDK record, type, identity, source, connection, session and
wire byte exact. The sole allowed transformation replaces a selected initial
host/signature opaque SSH string with its exact RFC8709 typed length/value.
An independently decoded current field certificate must bind every changed PDU.
"""
import argparse,base64,copy,hashlib,itertools,json
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for name in ['before','after','field_certificate','out']:ap.add_argument('--'+name.replace('_','-'),dest=name,type=Path,required=True)
a=ap.parse_args()
for p in vars(a).values():assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
def sha(path):
 with path.open('rb')as f:return hashlib.file_digest(f,'sha256').hexdigest()
def canonical(v):return json.dumps(v,sort_keys=True,separators=(',',':'),ensure_ascii=True).encode()
def unique(items):
 d={}
 for k,v in items:
  assert k not in d,'duplicate object key';d[k]=v
 return d
def invalid(v):raise ValueError('non-JSON constant: '+v)
def rows(path):
 for line in path.open(encoding='utf-8-sig'):yield json.loads(line,object_pairs_hook=unique,parse_constant=invalid)
proof=json.loads(a.field_certificate.read_text());assert proof['events_sha256']==sha(a.after),'field certificate binds the entire current export'
certified={v['PDU']:v for v in proof['compared_PDUs']};assert len(certified)==len(proof['compared_PDUs'])
def expand(tree):
 changed=[]
 if not isinstance(tree,dict):return changed
 payload=tree.get('Payload')
 if not isinstance(payload,dict):return changed
 for container,format_key,old_key,suffix in [('Host Key','Host Key Format','Opaque Host Key Data',' Public Key'),('Signature','Signature Format','Opaque Signature Data',' Signature Bytes')]:
  inner=payload.get(container)
  if not isinstance(inner,dict)or old_key not in inner:continue
  fmt=inner.get(format_key)
  if fmt not in ['ssh-ed25519','ssh-ed448']:continue
  name='Ed25519'if fmt=='ssh-ed25519'else'Ed448';size=(32 if name=='Ed25519'else 57)if container=='Host Key'else(64 if name=='Ed25519'else 114)
  raw=base64.b64decode(inner[old_key],validate=True);assert len(raw)==4+size and int.from_bytes(raw[:4],'big')==size,'exact nested RFC8709 string'
  assert name+suffix not in inner and name+suffix+' Length'not in inner,'no overwriting existing fields'
  del inner[old_key];inner[name+suffix+' Length']=size;inner[name+suffix]=base64.b64encode(raw[4:]).decode();changed.append(container)
 return changed
count=0;changes=[];protocols={}
for index,(old,new)in enumerate(itertools.zip_longest(rows(a.before),rows(a.after)),1):
 assert old is not None and new is not None,'complete ordered record count'
 count+=1;protocols[new['protocol']]=protocols.get(new['protocol'],0)+1
 expected=copy.deepcopy(old);paths=[]
 if old.get('protocol')=='ssh'and old.get('entry')=='SSHPacket'and old.get('status')in['decoded','deferred']and isinstance(old.get('session'),dict)and isinstance(old['session'].get('Initial Packet Fields'),dict):
  for label,tree in [('fields',expected.get('fields')),('session/Initial Packet Fields',expected['session']['Initial Packet Fields'])]:
   paths.extend(label+'/'+v for v in expand(tree))
 if paths:
  assert new['id']in certified,'every changed method tree independently certified';cert=certified[new['id']];raw=base64.b64decode(new['raw'],validate=True)
  assert hashlib.sha256(raw).hexdigest()==cert['raw_sha256'],'literal complete wire identity'
  assert hashlib.sha256(canonical(new['fields'])).hexdigest()==cert['field_tree_sha256'],'literal independently decoded field identity'
  assert sorted(r['Number']for r in new['source_bytes']['PacketRefs'])==cert['physical_frames'],'exact contributors'
  assert [new['source'],new['destination'],new['domain']]==[cert['source'],cert['destination'],cert['capture_domain']],'exact connection direction and capture scope'
  changes.append({'row':index,'PDU':new['id'],'expanded':paths,'raw_sha256':cert['raw_sha256']})
 assert canonical(expected)==canonical(new),('non-certified full-record change',index,new.get('id'))
assert count,'empty input cannot certify invariants'
result={'scope':__doc__,'records_compared':count,'changed_records':changes,'before_sha256':sha(a.before),'after_sha256':sha(a.after),'field_certificate_sha256':sha(a.field_certificate),'protocol_record_counts':dict(sorted(protocols.items())),'all_other_values_types_order_and_IDs_exact':True,'identity_or_field_normalization':False}
a.out.parent.mkdir(parents=True,exist_ok=True);a.out.write_text(json.dumps(result,indent=2)+'\n');print(json.dumps({'records_compared':count,'changed_records':len(changes),'all_other_values_types_order_and_IDs_exact':True}))
