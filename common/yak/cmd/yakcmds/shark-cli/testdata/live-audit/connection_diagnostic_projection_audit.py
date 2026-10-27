"""Certify complete CLI exports with only native TCP failure-status restoration.

Connection identity must not replace a native malformed/resource refusal with
missing-context text. Bind each changed row to unchanged CLI PDU metadata and
an exact SDK raw SHA/contributor/domain/endpoint witness, keeping namespaces
independent. This verifies honest projection of the native result; it does not
certify that each underlying parser refusal is correct under its protocol RFC.
"""
import argparse,base64,collections,copy,hashlib,itertools,json
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for name in ['before','after','SDK','out']:ap.add_argument('--'+name,type=Path,required=True)
a=ap.parse_args()
for path in vars(a).values():assert not any((p/'.git').exists()for p in[path.resolve(),*path.resolve().parents])
def sha(path):
 with path.open('rb')as f:return hashlib.file_digest(f,'sha256').hexdigest()
def unique(items):
 d={}
 for k,v in items:assert k not in d,'duplicate JSON key';d[k]=v
 return d
def invalid(v):raise ValueError('non-JSON constant: '+v)
def rows(path):
 for l in path.open(encoding='utf-8-sig'):yield json.loads(l,object_pairs_hook=unique,parse_constant=invalid)
def canonical(v):return json.dumps(v,sort_keys=True,separators=(',',':'),ensure_ascii=True)
def evidence_key(proto,source):return proto,source['SHA256'],canonical(source.get('PacketRefs',[]))
def endpoint(host,port):return f'[{host}]:{port}'if ':'in host else f'{host}:{port}'
native=collections.defaultdict(list)
for e in rows(a.SDK):
 if e['transport']!='tcp'or e['status']not in ['malformed','limited']or not e.get('error'):continue
 raw=base64.b64decode(e['raw'],validate=True);source=e['source_bytes'];assert source['SHA256']==hashlib.sha256(raw).hexdigest(),'independent complete SDK raw identity'
 native[evidence_key(e['protocol'],source)].append(e)
count=0;changes=[];labels=collections.Counter();refusals=collections.Counter()
for b,n in itertools.zip_longest(rows(a.before),rows(a.after)):
 assert b is not None and n is not None,'ordered physical row count';count+=1;labels[n['protocol']]+=1;expected=copy.deepcopy(b)
 if canonical(b)!=canonical(n):
  assert b['protocol']==b['connection_protocol']and b['transport']=='tcp';assert b['protocol_status']=='context-required'and b['protocol_scope']=='connection-associated bytes';assert b['protocol_evidence']=='observed connection; this segment has no complete decoded message';assert b['visibility']in ['unassembled-stream','encrypted']
  prefix=b['connection_protocol']+' stream bytes · no complete decoded message in this packet · ';assert b['info'].startswith(prefix);expected['info']=b['info'][len(prefix):];expected['protocol_scope']='connection-associated diagnostic';expected['protocol_evidence']='protocol parser diagnostic';del expected['visibility']
  assert n['protocol_status']in ['malformed','limited'];expected['protocol_status']=n['protocol_status'];assert 'visibility'not in n,'this selected restoration does not manufacture an encryption claim'
  candidates=[];native_cli_pdus=[]
  for pdu in b['pdus']:
   if pdu['status']!=n['protocol_status']or not pdu.get('error'):continue
   source=pdu['byte_source'];assert any(r['Number']==b['number']and r['Domain']==b['capture_domain']for r in source['PacketRefs']),'exact physical row domain and byte contribution'
   for e in native.get(evidence_key(pdu['protocol'],source),[]):
    if e['status']==pdu['status']and e['error']==pdu['error']and e['source']==endpoint(b['source'],b['source_port'])and e['destination']==endpoint(b['destination'],b['destination_port']):candidates.append(e);native_cli_pdus.append(pdu)
  assert candidates,('native refusal witness required',b['number']);refusals[b['connection_protocol']]+=1
  changes.append({'physical_frame':b['number'],'capture_domain':b['capture_domain'],'protocol':b['protocol'],'status_before':b['protocol_status'],'status_after':n['protocol_status'],'SDK_PDU_witnesses':[e['id']for e in candidates],'CLI_PDU_witnesses':[e['id']for e in native_cli_pdus],'raw_sha256_witnesses':sorted({e['source_bytes']['SHA256']for e in candidates}),'native_errors':sorted({e['error']for e in candidates}),'independent_namespace_numeric_equality_required':False})
 assert canonical(expected)==canonical(n),('non-certified complete row change',count,b['number'])
assert count,'empty export cannot certify equality'
result={'scope':__doc__,'physical_rows_compared':count,'changed_rows':changes,'changed_rows_by_protocol':dict(refusals),'before_sha256':sha(a.before),'after_sha256':sha(a.after),'SDK_sha256':sha(a.SDK),'literal_protocol_label_counts':dict(sorted(labels.items())),'all_other_values_types_order_and_IDs_exact':True,'identity_or_field_normalization':False};a.out.parent.mkdir(parents=True,exist_ok=True);a.out.write_text(json.dumps(result,indent=2)+'\n');print(json.dumps({k:v for k,v in result.items()if k not in ['scope','changed_rows','literal_protocol_label_counts']}))
