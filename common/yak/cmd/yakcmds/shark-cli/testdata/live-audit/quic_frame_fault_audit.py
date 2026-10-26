"""Bind actual controlled UDP frame faults to independent AEAD and native refusal.
Injected controls are drained before endpoint processing; subsequent byte-exact
requests prove capture/parser recovery, not that an endpoint accepts bad frames.
Only these selected 1-RTT constraints are certified; 0-RTT/ticket/full endpoint
conformance are not established by this capture.
"""
import argparse,base64,collections,hashlib,json
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in['manifest','events','before','oracle','crypto-audit','out']:ap.add_argument('--'+n,type=Path,required=True)
a=ap.parse_args();checks=0
for p in vars(a).values():assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
def check(ok,why):
 global checks
 checks+=1;assert ok,why
def eq(x,y,why):check(x==y,(why,x,y))
def sha(w):return hashlib.sha256(w).hexdigest()
def load(p):return json.loads(p.read_text(encoding='utf-8-sig'))
m=load(a.manifest);proof=load(a.crypto_audit);eq(proof['events_sha256'],sha(a.events.read_bytes()),'independently certified native export');eq(proof['declared_manifest_sha256'],sha(a.manifest.read_bytes()),'exact declared control manifest');eq(proof['oracle_sha256'],sha(a.oracle.read_bytes()),'exact reference export')
reference=collections.defaultdict(list)
def leaves(o,key):
 if isinstance(o,dict):
  for k,v in o.items():
   if k==key:
    yield from(v if isinstance(v,list)else[v])
   elif isinstance(v,(dict,list)):yield from leaves(v,key)
 elif isinstance(o,list):
  for v in o:yield from leaves(v,key)
for row in load(a.oracle):
 l=row['_source']['layers'];vs=l.get('quic')or[];vs=vs if isinstance(vs,list)else[vs];rs=l.get('quic_raw')or[];rs=[rs]if rs and isinstance(rs[0],str)else rs
 for view,raw in zip(vs,rs):reference[sha(bytes.fromhex(raw[0]))].append((int(l['frame']['frame.number']),view))
# RFC9000 sections12.4,19.7,19.20: the known invalid cases and positive
# contrast are explicit here, rather than trusting an expected label in input.
specs={'client-handshake-done':(0,'1e',0,'MalformedMessage'),'client-new-token':(0,'070178',0,'MalformedMessage'),'server-empty-new-token':(1,'0700',0,'MalformedMessage'),'reserved-bits':(0,'01',8,'MalformedMessage'),'nonminimal-ping':(0,'4001',0,'MalformedMessage'),'unselected-frame':(0,'1f',0,'UnsupportedFeature'),'wide-unselected-frame':(0,'4040',0,'UnsupportedFeature'),'empty-payload':(0,'',0,'MalformedMessage'),'allowed-server-token':(1,'070176',0,None)}
def by_wire(path):
 out=collections.defaultdict(list)
 for line in path.open(encoding='utf-8-sig'):
  e=json.loads(line)
  if e['protocol']in['quic','http3','doq']:out[sha(base64.b64decode(e['raw']or''))].append(e)
 return out
events=by_wire(a.events);before=by_wire(a.before);cert={p['wire_sha256']:p for p in proof['packets']};flows={};by_epoch=collections.defaultdict(list);records=[];old_false_accepts=0;old_protection_fact_omissions=0;old_unsupported_malformed=0
eq(len(m['profiles']),3,'three observed generations');eq(len(m['transport_faults']),3*len(specs),'all cases emitted in each generation')
for control in m['transport_faults']:
 name=control['case'];check(name in specs,'known declared constraint');direction,hex_body,reserved,error=specs[name];eq((control['direction'],control['plaintext_hex'],control['reserved_bits'],control['expected_native_error']),(direction,hex_body,reserved,error),'independent declared packet layout and reason');check(control['receiver_observed_and_drained'],'actual socket receiver drained the declared injected control');wire=control['wire_sha256'];check(wire in cert,'control packet independently decrypted from capture');c=cert[wire];eq(c['plaintext_sha256'],sha(bytes.fromhex(hex_body)),'independent literal decrypted payload');eq(c['packet_number'],control['packet_number'],'independent packet number');eq(c['reserved_bits'],reserved,'independent reserved bits')
 eq(len(events[wire]),1,'single native control observation');eq(len(before[wire]),1,'single baseline observation');e,old=events[wire][0],before[wire][0];s=e['session'];eq(e['protocol'],'quic','transport refusal cannot become an application label');eq(s.get('Authentication Verified'),True,'packet AEAD verification survives protocol refusal');eq(s.get('Peer Identity Verified'),False,'no peer identity claim');eq(s['Packet Number'],control['packet_number'],'native packet number');eq([r['Number']for r in e['source_bytes']['PacketRefs']],[c['frame']],'exact physical source frame');check(e['flow'],'captured connection retained');epoch=control['epoch'];flows.setdefault(epoch,e['flow']);eq(flows[epoch],e['flow'],'one generation across valid/refused controls');by_epoch[epoch].append(e['id'])
 if error:
  eq(e['expert'],error,'specific native refusal class');eq(e['status'],'malformed'if error=='MalformedMessage'else'context-required','protocol violation and unsupported profile are separate');check(e.get('error'),'visible refusal diagnostic');check('Frames'not in s,'no delivered frames in refused packet');eq(base64.b64decode(s['Rejected Packet Payload']or''),bytes.fromhex(hex_body),'inspectable refused payload exact bytes');eq(s['Rejected Packet Payload SHA256'],c['plaintext_sha256'],'inspectable payload digest');check('not delivered'in s['Payload Delivery'],'explicit non-delivery scope')
  old_false_accepts+=old['status']in['decoded','deferred']and not old.get('error');old_unsupported_malformed+=error=='UnsupportedFeature'and old['status']=='malformed'
 else:
  eq(e['status'],'decoded','allowed server token contrast remains accepted');check(not e.get('error'),'valid control has no refusal');eq(s['Frames'][0]['Frame Type'],'NEW_TOKEN','actual valid frame');eq(base64.b64decode(s['Frames'][0]['Token']),'v'.encode(),'actual valid opaque token');check('Rejected Packet Payload'not in s,'positive control not misrepresented as refused')
 old_protection_fact_omissions+=not(old.get('session')or{}).get('Authentication Verified',False)
 refs=reference[wire];eq(len(refs),1,'one exact literal reference packet view');frame,view=refs[0];eq(frame,c['frame'],'reference physical frame');types=list(leaves(view,'quic.frame_type'));pns=list(leaves(view,'quic.packet_number'))
 if pns:eq([int(v)for v in pns],[c['packet_number']],'reference and independent packet number')
 if name!='empty-payload':eq([int(v,0)for v in types],[{'client-handshake-done':30,'client-new-token':7,'server-empty-new-token':7,'reserved-bits':1,'nonminimal-ping':1,'unselected-frame':31,'wide-unselected-frame':64,'allowed-server-token':7}[name]],'reference literal decrypted frame type')
 records.append({'case':name,'epoch':epoch,'physical_frame':c['frame'],'PDU':e['id'],'flow':e['flow'],'packet_number':c['packet_number'],'native_status':e['status'],'native_error_class':e['expert'],'before_status':old['status'],'before_error_class':old['expert'],'reference_frame_types':types,'reference_packet_number_available':bool(pns),'reference_expert_messages':list(leaves(view,'_ws.expert.message')),'wire_sha256':wire,'payload_sha256':c['plaintext_sha256']})
eq(len(set(flows.values())),3,'independent native generations');seen=set()
for values in events.values():
 for e in values:
  if e['protocol']!='http3':continue
  for fr in(e.get('session')or{}).get('Frames',[]):
   for h in fr.get('HTTP3 Frames',[]):
    if h['Frame Type']!='HEADERS'or h['Direction']!=0:continue
    fields={v['Name']:v['Value']for v in h.get('Headers')or[]}
    if fields.get(':path')!='/after-transport-faults':continue
    epoch=next(k for k,v in flows.items()if v==e['flow']);check(e['id']>max(by_epoch[epoch]),'recovery request observed after every control in the same generation');check(epoch not in seen,'one recovery request per generation');seen.add(epoch)
eq(seen,set(range(3)),'all live generations continue into a real recovery request')
out={'scope':__doc__,'checks':checks,'controlled_wire_packets':len(records),'native_refusals':sum(bool(r['native_error_class'])for r in records),'allowed_contrasts':sum(not r['native_error_class']for r in records),'baseline_false_successes':old_false_accepts,'baseline_AEAD_fact_omissions':old_protection_fact_omissions,'baseline_unsupported_misclassified_malformed':old_unsupported_malformed,'recovered_live_generations':len(seen),'manifest_sha256':sha(a.manifest.read_bytes()),'events_sha256':sha(a.events.read_bytes()),'before_sha256':sha(a.before.read_bytes()),'crypto_certificate_sha256':sha(a.crypto_audit.read_bytes()),'records':records};a.out.parent.mkdir(parents=True,exist_ok=True);a.out.write_text(json.dumps(out,indent=2)+'\n',encoding='utf8');print(json.dumps({k:v for k,v in out.items()if k not in['scope','records']}))
