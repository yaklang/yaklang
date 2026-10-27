"""Check every public CLI row against the guarded SSH recovery SDK export.
Use the separately certified exact-message PDU-ID bijection only for declared
identity/reference metadata. Physical row facts and every other connection stay
exact. New SSH scalar projections must match their exact SDK source evidence.
"""
import argparse,collections,hashlib,itertools,json
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in['before','after','sdk','identity-map','out']:ap.add_argument('--'+n,type=Path,required=True)
ap.add_argument('--target-flow',type=int,required=True);a=ap.parse_args();checks=0
for p in vars(a).values():
 if isinstance(p,Path):assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
def check(ok,why):
 global checks
 checks+=1;assert ok,why
def eq(x,y,why):check(x==y,why)
def rows(p):
 for l in p.open(encoding='utf-8-sig'):yield json.loads(l)
def sha(p):
 h=hashlib.sha256()
 with p.open('rb')as f:
  for b in iter(lambda:f.read(1<<20),b''):h.update(b)
 return h.hexdigest()
cert=json.loads(a.identity_map.read_text());eq(cert['after_sha256'],sha(a.sdk),'certified exact SDK export');mapping={int(k):v for k,v in cert['PDU_identity_map'].items()};ssh={e['id']:e for e in rows(a.sdk)if e['protocol']=='ssh'and e['flow']==a.target_flow};aliases={'pdu.id','pdu.response_to','ntp.control.request_pdu','http3.connection.error_pdu'}
def mid(v):
 if not v:return v
 check(type(v)is int and v in mapping,'exact previously certified PDU reference');return mapping[v]
def source(s):
 if not s:return
 s['ParentPDU']=mid(s['ParentPDU'])
 if s.get('ParentPDUs')is not None:s['ParentPDUs']=[mid(v)for v in s['ParentPDUs']]
def normalize(p):
 if p.get('id'):p['id']=mid(p['id'])
 if p.get('response_to'):p['response_to']=mid(p['response_to'])
 source(p.get('byte_source'));f=p.get('field_values')or{}
 if f:
  f['pdu']=mid(f['pdu']);source(f.get('source'))
  for v in f.get('values',[]):
   if v['name']in aliases:
    old=v['value'];v['value']=mid(old)
    if'exact_value'in v:eq(v['exact_value'],str(old),'literal exact ID metadata');v['exact_value']=str(v['value'])
 for name in aliases:
  if name in(p.get('fields')or{}):p['fields'][name]=[mid(v)for v in p['fields'][name]]
changed=[];counts=collections.Counter();physical=['number','time','source','destination','length','captured_length','source_port','destination_port','transport','capture_domain','tcp_flow_id','tcp_flow_domain','stream_id'];allowed={'pdus','info','protocol','protocol_stack','connection_protocol','connection_stack','connection_evidence','visibility','protocol_status','protocol_scope','protocol_evidence','message_source','message_destination','message_transport','message_domain','pdu_scope','layers'};total=0;new_projections=0;cli_flow=None;native_identity=next(e for e in ssh.values()if(e.get('session')or{}).get('Identification Conformance')=='nonconformant');endpoints={native_identity['source'],native_identity['destination']}
for old,new in itertools.zip_longest(rows(a.before),rows(a.after)):
 check(old is not None and new is not None,'same complete physical row count');total+=1
 for p in old.get('pdus',[]):normalize(p)
 if old==new:continue
 touched={k for k in old.keys()|new.keys()if old.get(k)!=new.get(k)}
 check(touched<=allowed,('only declared SSH presentation paths',new['number'],sorted(touched-allowed)))
 if cli_flow is None:
  check(any(p['id']in ssh for p in new.get('pdus',[])),'first changed row binds the separate CLI flow namespace to an exact SDK PDU');cli_flow=new.get('tcp_flow_id');check(cli_flow is not None,'observed CLI connection retained')
 eq(new.get('tcp_flow_id'),cli_flow,('only the original declared CLI SSH connection changes',new['number']));eq(new.get('tcp_flow_domain'),native_identity['domain'],'original captured domain retained')
 if new.get('message_source')or new.get('message_destination'):eq({new.get('message_source'),new.get('message_destination')},endpoints,('original exact inner SSH endpoints retained',new['number']))
 else:check(not new.get('pdus')and not old.get('message_source')and not old.get('message_destination'),'a physical ACK without published PDU does not fabricate message endpoints')
 for name in physical:eq(old.get(name),new.get(name),('literal physical/connection fact unchanged',new['number'],name))
 eq(new['protocol'],'SSH','guarded native identity is shown as SSH')
 for p in new.get('pdus',[]):
  check(p['id']in ssh,'new target scalar PDU has an exact SDK message');e=ssh[p['id']];check(new['number']in[r['Number']for r in e['source_bytes']['PacketRefs']],'physical row is an original contributor to this exact logical PDU')
  for name in['protocol','status','response_to']:eq(p.get(name),e.get(name),'scalar identity/status/association matches SDK')
  eq(p['byte_source'],e['source_bytes'],'exact SDK byte source and completion');eq(p['field_values'],e['field_values'],'exact SDK typed fields and evidence origins');expected=collections.defaultdict(list)
  for v in e['field_values']['values']:expected[v['name']].append(v['value'])
  eq(p['fields'],dict(expected),'compact query values agree with exact SDK projection')
  for name in['expert','error']:
   if name in p:eq(p[name],e.get(name)or'','literal SDK diagnostic survives publication')
  new_projections+=1
 for name in touched:counts[name]+=1
 changed.append({'physical_frame':new['number'],'presentation_paths':sorted(touched),'PDU_ids':[p['id']for p in new.get('pdus',[])]})
out={'scope':__doc__,'checks':checks,'physical_rows':total,'changed_original_SSH_rows':len(changed),'new_scalar_projections_compared':new_projections,'changed_presentation_paths':dict(counts),'changed_records':changed,'before_sha256':sha(a.before),'after_sha256':sha(a.after),'SDK_sha256':sha(a.sdk),'other_connection_differences':[]};a.out.parent.mkdir(parents=True,exist_ok=True);a.out.write_text(json.dumps(out,indent=2)+'\n');print(json.dumps({k:v for k,v in out.items()if k not in['scope','changed_records']}))
