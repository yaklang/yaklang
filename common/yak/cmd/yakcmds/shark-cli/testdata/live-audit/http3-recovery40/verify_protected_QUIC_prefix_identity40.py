"""Bind every changed protected QUIC PDU to pre-candidate independent identity slots.

Build each permitted whole object from its old object plus the minimal public
identity-format contract and independent packet/STREAM/EE facts. Every other key,
array order, source, association, frame/body and typed value must remain literal.
Source slot evidence was frozen before reading candidate details. This verifier
was prepared after inspecting changed paths; it is not a pre-runtime whole plan.
"""
import argparse,base64,collections,copy,hashlib,itertools,json
from pathlib import Path

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def digest(w):return hashlib.sha256(w).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def rows(p):return[json.loads(s)for s in Path(p).read_text('utf8').splitlines()]
def write(p,x):
 with Path(p).open('x',encoding='utf8')as f:json.dump(x,f,indent=2);f.write('\n')
def canonical(x):return digest(json.dumps(x,sort_keys=True,separators=(',',':')).encode())

def source_binding(event,fact,slots,flow):
 refs=event['source_bytes']['PacketRefs'];assert len(refs)==1;assert refs[0]=={'Number':fact['frame'],'Domain':fact['domain']};assert event['domain']==fact['domain'];raw=base64.b64decode(event['raw']);assert len(raw)==fact['packet_bytes']and digest(raw)==fact['packet_SHA256'];assert event['source_bytes']['SHA256']==fact['packet_SHA256']
 assert event['direction']==fact['direction'];assert event['source']==fact['source_IP']+':'+str(fact['source_port']);assert event['destination']==fact['destination_IP']+':'+str(fact['destination_port']);s=event['session'];assert s['Authentication Verified']is fact['authenticated']
 if fact['authenticated']:
  assert s['Packet Number']==fact['PN']and s['Packet Number Length']==fact['PN_bytes'];assert s['Packet Number Space']==fact['space']
  if fact['key_phase']is not None:assert s['Key Phase']==fact['key_phase']
  assert len(s['Frames'])==len(fact['frames'])
  for af,sf in zip(s['Frames'],fact['frames']):
   assert af['Frame Type']==sf['name']
   if sf['name']=='STREAM':assert af['Stream ID']==sf['stream']and af['Offset']==sf['offset']and af['Length']==sf['length']and af['FIN']==sf['FIN'];assert {'bytes':len(base64.b64decode(af['Stream Data'])),'SHA256':digest(base64.b64decode(af['Stream Data']))}==sf['data']
  conn=fact['connection'];assert event['flow']not in flow or flow[event['flow']]==conn;flow[event['flow']]=conn
 else:assert'Packet Number'not in s and not s.get('Frames')and not s.get('Decrypted')
 for slot in slots:
  assert fact['authenticated']and fact['connection']==slot['source_connection']and fact['destination_CID']==slot['destination_CID'];assert fact['PN']==slot['PN']and fact['epoch']==slot['epoch'];assert fact['ALPN_confirmed_before_packet']==slot['source_ALPN']=='h3';assert slot['Stream_ID']%4==0 and slot['Stream_Length']>0 and slot['captured_contiguous_prefix_before']==slot['captured_contiguous_prefix_after']==0

def expected_identity(old,fact,slots):
 assert slots and old['protocol']=='quic';new=copy.deepcopy(old);s=new['session'];sid=[]
 for slot in slots:
  fi=slot['STREAM_frame_index'];fr=s['Frames'][fi];assert fr['Frame Type']=='STREAM'and fr['Stream ID']==slot['Stream_ID']and fr['Offset']==slot['Stream_Offset']and fr['Length']==slot['Stream_Length'];assert fr['Delivered Bytes']==0 and not fr.get('HTTP3 Frames')
  for key in('HTTP3','HTTP3 Stream Kind','HTTP3 Wire Observation'):assert key not in fr
  fr.update({'HTTP3':True,'HTTP3 Stream Kind':'client-bidirectional','HTTP3 Wire Observation':'prefix-missing'});sid.append(slot['Stream_ID'])
 assert old['transaction']==old['response_to']==0
 for key in('HTTP3','HTTP3 Stream Kind','HTTP3 Stream ID','Protocol Transition','HTTP3 Wire Only','HTTP3 Wire Observation'):assert key not in s
 s.update({'HTTP3':True,'HTTP3 Stream Kind':'client-bidirectional','HTTP3 Stream ID':sid[0],'Protocol Transition':'quic->http3','HTTP3 Wire Only':True,'HTTP3 Wire Observation':'prefix-missing'})
 new.update(protocol='http3',profile='http3-native',summary='http3')
 values=new['field_values']['values'];existing_names={v['name']for v in values};assert not existing_names&{'http3.streamid','http3.wire_only','http3.wire_observation'}
 for v in values:
  if v['name']=='protocol':assert v['value']=='quic';v['value']='http3'
 # The existing API emits event facts first, then other bindings in name order;
 # preserve all existing values/order and merge only the newly enabled aliases.
 prefix=[v for v in values if v['origin']=='event'];rest=[v for v in values if v['origin']!='event'];assert values==prefix+rest;assert [v['name']for v in rest]==sorted(v['name']for v in rest)
 additions=[{'name':'http3.streamid','value':v,'exact_value':str(v),'origin':'pdu'}for v in sid]+[{'name':'http3.wire_observation','value':'prefix-missing','origin':'derived'},{'name':'http3.wire_only','value':True,'origin':'derived'}]
 assert [v['name']for v in additions]==sorted(v['name']for v in additions);merged=[];at=0
 for v in rest:
  while at<len(additions)and additions[at]['name']<v['name']:merged.append(additions[at]);at+=1
  merged.append(v)
 merged.extend(additions[at:]);new['field_values']['values']=prefix+merged;return new

def verify_one(old,new,fact,slots,flow):
 source_binding(new,fact,slots,flow)
 if old==new:return False,None
 expected=expected_identity(old,fact,slots);assert expected==new,'whole object differs beyond exact source-built identity and typed aliases';return True,expected

def main():
 ap=argparse.ArgumentParser(description=__doc__)
 for n in('phase','source','before','after','out'):ap.add_argument('--'+n,type=Path,required=True)
 a=ap.parse_args();assert not a.out.exists();a.out.mkdir();proof=read(a.source/'summary.json');assert proof['no_candidate_or_native_or_WS_output_read'];binding={str(p):sha(p)for p in(Path(__file__),a.source/'summary.json',a.source/'plan-before.json',a.before/'summary.json',a.after/'summary.json')};results=[];anchors=[];fail=[];negative=[];first=None
 contract={'scope':__doc__,'candidate_read_scope_before_verifier':['old and new metadata receipts','changed path names and PDU/source references','one old PDU schema sample'],'pre_candidate_source_slots_summary_SHA256':sha(a.source/'summary.json'),'expected_identity_literals':{'Protocol':'http3','Profile':'http3-native','Summary':'http3','HTTP3 Stream Kind':'client-bidirectional','HTTP3 Wire Observation':'prefix-missing','HTTP3 Wire Only':True,'Protocol Transition':'quic->http3'},'typed_aliases':{'http3.streamid':{'bits':62,'origin':'pdu','exact_value':'decimal source SID','pdu_range':None},'http3.wire_only':{'origin':'derived','value':True},'http3.wire_observation':{'origin':'derived','value':'prefix-missing'}},'no_candidate_values_used_to_define_source_ID_PN_CID_direction_STREAM_or_ALPN':True,'all_other_object_values_and_array_order_literal':True,'broader_source_eligible_slots_do_not_all_have_to_change':'Existing previously recognized HTTP3 context can already carry the same source identity; source slots are independent lower-bound evidence, not an exact native h3 parser-state emulator.'};write(a.out/'comparison-policy-before.json',contract)
 for case in proof['results']:
  name=case['case'];group='QUIC-sparse-epoch-plan-before-01'if name in('original-real-network-loss','all-client-middle-epoch-omitted')else'QUIC-sparse-recovery-controls-before-01';rel=Path(group)/name;src=a.source/name;facts=rows(src/'packet-source.jsonl');slots=rows(src/'prefix-identity-slots.jsonl');by={(f['frame'],f['packet_SHA256']):f for f in facts};slotby=collections.defaultdict(list)
  for s in slots:slotby[s['frame'],s['packet_SHA256']].append(s)
  tails={(r['frame'],r['SHA256']):r for r in read(src/'unprotected-tail-source.json')};oldp=a.before/rel/'events.jsonl';newp=a.after/rel/'events.jsonl';flow={};count=collections.Counter();changed=[]
  for path in(oldp,newp,src/'packet-source.jsonl',src/'prefix-identity-slots.jsonl',src/'connections.json',src/'handshake-source.jsonl',src/'unprotected-tail-source.json'):binding[str(path)]=sha(path)
  for root,ep in((a.before,oldp),(a.after,newp)):
   pp=ep.parent/'producer.json';p=read(pp);binding[str(pp)]=sha(pp);assert p['exit_code']==0 and p['capture_SHA256']==case['capture_SHA256']and p['keylog_SHA256']==case['keylog_SHA256'];assert p['export_SHA256']==sha(ep)and p['stderr_SHA256']==sha(ep.parent/'stderr');assert p['source']==read(root/'summary.json')['source'];binary=Path(p['arguments'][0]);build=binary.parent/'events-receipt.json';b=read(build);assert sha(build)==p['build_receipt_SHA256']and sha(binary)==p['binary_SHA256']==b['binary_SHA256']and b['source']==p['source'];binding.update({str(path):sha(path)for path in(binary,build)})
  for no,(old,new)in enumerate(itertools.zip_longest(rows(oldp),rows(newp)),1):
   assert old is not None and new is not None;assert old['id']==new['id'];count['all_events']+=1;refs=new['source_bytes']['PacketRefs'];assert len(refs)==1;key=(refs[0]['Number'],new['source_bytes']['SHA256'])
   if key in tails:
    assert old==new and digest(base64.b64decode(new['raw']))==key[1];assert new['session']['Authentication Verified']is False and 'Packet Number'not in new['session'];count['unprotected_tail_objects_literal']+=1;continue
   assert key in by;fact=by[key]
   try:
    change,expected=verify_one(old,new,fact,slotby[key],flow);count['source_AEAD_authenticated_objects'if fact['authenticated']else'source_unopened_objects_no_PN_or_plaintext']+=1
    if change:
     changed.append(new['id']);count['changed_source_bound_whole_objects']+=1;anchor={'case':name,'PDU':new['id'],'flow':new['flow'],'physical_frame':key[0],'packet_SHA256':key[1],'expected_whole_object_canonical_SHA256':canonical(expected),'actual_whole_object_canonical_SHA256':canonical(new),'source_slots':slotby[key]};anchors.append(anchor)
     if first is None:first=(copy.deepcopy(old),copy.deepcopy(new),fact,slotby[key])
    else:count['all_other_objects_literal']+=1
   except Exception as e:fail.append({'case':name,'line':no,'PDU':new['id'],'physical_frame':key[0],'packet_SHA256':key[1],'reason':repr(e)})
  assert len(set(flow.values()))==len(flow)==3;results.append({'case':name,'counts':dict(count),'changed_PDU_IDs':changed,'literal_flow_to_independent_source_connection':flow,'before_events_SHA256':sha(oldp),'after_events_SHA256':sha(newp),'source_eligible_slots':case['prefix_identity_packet_slots']})
 assert first is not None
 if not fail:
  old,new,fact,slots=first
  for label in('raw','direction','physical-reference','CID-binding','STREAM-ID','inherited-transaction','invented-HEADERS','invented-method','missing-marker'):
   bad=copy.deepcopy(new);f=copy.deepcopy(fact)
   if label=='raw':raw=base64.b64decode(bad['raw']);bad['raw']=base64.b64encode(raw[:-1]+bytes([raw[-1]^1])).decode()
   elif label=='direction':bad['direction']=1-bad['direction']
   elif label=='physical-reference':bad['source_bytes']['PacketRefs'][0]['Number']+=1
   elif label=='CID-binding':f['destination_CID']='00'+f['destination_CID'][2:]
   elif label=='STREAM-ID':bad['session']['Frames'][slots[0]['STREAM_frame_index']]['Stream ID']+=4
   elif label=='inherited-transaction':bad['transaction']=1
   elif label=='invented-HEADERS':bad['session']['Frames'][slots[0]['STREAM_frame_index']]['HTTP3 Frames']=[{'Frame Type Value':1}]
   elif label=='invented-method':bad['field_values']['values'].append({'name':'http.request.method','value':'POST','origin':'pdu'})
   else:bad['session'].pop('HTTP3 Wire Observation')
   rejected=False
   try:verify_one(old,bad,f,slots,{})
   except AssertionError:rejected=True
   assert rejected,('negative accepted',label);negative.append(label)
 assert binding=={p:sha(p)for p in binding};write(a.out/'changed-PDU-source-anchors.json',anchors);write(a.out/'failures.json',fail)
 report={'scope':__doc__,'source_summary_SHA256':sha(a.source/'summary.json'),'comparison_policy_before_SHA256':sha(a.out/'comparison-policy-before.json'),'results':results,'changed_source_bound_whole_objects':len(anchors),'unchanged_TLS_scope':'not rechecked here; root owns literal TLS360 check','source_bad_AEAD_no_identity_or_plaintext_inheritance':True,'required_negative_controls_rejected':negative,'failure_count':len(fail),'failures_SHA256':sha(a.out/'failures.json'),'source_anchors_SHA256':sha(a.out/'changed-PDU-source-anchors.json'),'inputs':binding,'inputs_unchanged':True,'new_live_capture_claim':False,'full_HTTP3_message_recovery_or_TOP30_claim':False};write(a.out/'summary.json',report);print(json.dumps({'summary':str(a.out/'summary.json'),'SHA256':sha(a.out/'summary.json'),'changed_objects':len(anchors),'failures':len(fail),'negative_controls':negative,'results':[{'case':r['case'],'counts':r['counts']}for r in results]}))
 if fail:raise SystemExit(1)
if __name__=='__main__':main()
