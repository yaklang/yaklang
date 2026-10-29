"""Author 20 small RSVP boundary scenes outside the repository.

This generates synthetic source witnesses, never real traffic, and no binary
process is started. Source field values come from raw octets/RFC-selected
layouts. The frozen nine-frame baseline is not changed. Complete facts/plans,
pcap, raw blobs and reference outputs must never enter the source-only bundle.
"""
import argparse
import base64
import hashlib
import importlib.util
import ipaddress
import json
from pathlib import Path
import struct


def dg(data):return hashlib.sha256(data).hexdigest()
def sha(path):return dg(Path(path).read_bytes())
def put(path,obj):Path(path).write_text(json.dumps(obj,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
def load(path,name):
 spec=importlib.util.spec_from_file_location(name,path)
 module=importlib.util.module_from_spec(spec)
 spec.loader.exec_module(module)
 return module

def object_wire(cls,ctype,body,length=None):
 return struct.pack('!HBB',4+len(body) if length is None else length,cls,ctype)+body

def message_wire(model,body,kind=1,length=None,omit_checksum=False):
 raw=struct.pack('!BBHBBH',16,kind,0,64,0,8+len(body) if length is None else length)+body
 if not omit_checksum:raw=raw[:2]+struct.pack('!H',model.checksum(raw))+raw[4:]
 return raw

def session(v6=False):
 addr=ipaddress.ip_address('2001:db8:10::a' if v6 else '203.0.113.9').packed
 return object_wire(1,2 if v6 else 1,addr+bytes([17,0])+struct.pack('!H',40000))

def sender(v6=False,unused=0):
 addr=ipaddress.ip_address('2001:db8:20::b' if v6 else '198.51.100.2').packed
 return object_wire(11,2 if v6 else 1,addr+struct.pack('!HH',unused,40000))

def time_values(period=1000):return object_wire(5,1,struct.pack('!I',period))

def framed(model,raw,number,outer='v4',src=None,dst=None,truncate=0):
 eth=bytes.fromhex('020000000002020000000001')
 if outer=='v4':
  src=src or '192.0.2.1';dst=dst or '192.0.2.2'
  ip=struct.pack('!BBHHHBBH4s4s',69,0,20+len(raw),number,0,64,46,0,ipaddress.ip_address(src).packed,ipaddress.ip_address(dst).packed)
  ip=ip[:10]+struct.pack('!H',model.checksum(ip))+ip[12:]
  wire=eth+struct.pack('!H',2048)+ip+raw
  start=34
 else:
  src=src or '2001:db8:ffff::1';dst=dst or '2001:db8:ffff::2'
  ip=struct.pack('!IHBB16s16s',6<<28,len(raw),46,64,ipaddress.ip_address(src).packed,ipaddress.ip_address(dst).packed)
  wire=eth+struct.pack('!H',34525)+ip+raw
  start=54
 declared_end=len(wire)
 wire+=bytes(max(0,60-len(wire)))
 original=len(wire)
 if truncate:
  assert declared_end==len(wire) and 0<truncate<len(raw)
  wire=wire[:-truncate]
 captured_pdu=wire[start:min(declared_end,len(wire))]
 return wire,captured_pdu,{'IP_version':4 if outer=='v4' else 6,'IP_protocol_or_next_header':46,
                           'IP_source':src,'IP_destination':dst,'IP_header_range':[14,start],
                           'IP_declared_end':declared_end,'IP_available_end':len(wire),
                           'PDU_physical_range':[start,min(declared_end,len(wire))],
                           'parent_complete':declared_end<=len(wire),
                           'captured_length':len(wire),'original_length':original,
                           'Ethernet_padding_bytes':max(0,len(wire)-declared_end)}

def write_pcap(path,frames):
 data=struct.pack('<IHHIIII',0xa1b2c3d4,2,4,0,0,65535,1)
 for i,(wire,anchor) in enumerate(frames,1):
  data+=struct.pack('<IIII',1700000000+i,0,len(wire),anchor['original_length'])+wire
 path.write_bytes(data)

def remove_reference_fields(leaf):
 return {k:v for k,v in leaf.items() if k!='reference_alias'}

def run(phase,out):
 assert not out.exists()
 out.mkdir();(out/'pdu').mkdir();(out/'physical').mkdir()
 model_path=phase/'rsvp_boundary_source45.py'
 model=load(model_path,'rsvp_boundary_source_independent')
 baseline=phase/'RSVP-baseline-source45-01'
 baseline_source=phase/'build_RSVP_baseline_source45.py'
 assert sha(baseline_source)=='ca6807985fad38fea24a8cdb48b2d2d14f8e601b272145d96afb4ba1de21ca79'
 base_grid=baseline/'source-grid.json'
 assert sha(base_grid)=='9d31302c649ded298b26474aa98d1e245e2116086ac839bd48355852606262ad'
 primary=phase/'RSVP-primary-download45-01'
 inputs=[Path(__file__),model_path,baseline_source,base_grid,baseline/'manifest.json']+[primary/f'rfc{n}.txt'for n in(2205,2961,2747)]
 baseline_frames=json.loads(base_grid.read_text(encoding='utf-8'))
 inputs += [Path(r['PDU_path'])for r in baseline_frames]
 before={str(p):sha(p)for p in inputs}
 bridge=[]
 for row in baseline_frames:
  original_leaves=[remove_reference_fields(x)for x in row['leaves']]
  new=model.decode_source(Path(row['PDU_path']).read_bytes())
  assert original_leaves==new['leaves']
  assert not new['issues']
  bridge.append({'frame':row['frame'],'PDU_SHA256':row['PDU_SHA256'],'literal_wire_leaves':sum(x['literal']for x in new['leaves']),'derived_bits':sum(not x['literal']for x in new['leaves'])})
 scenes=[]
 def add(scene,name,raw,budget=4096,**carrier):
  scenes.append({'scene':scene,'name':name,'raw':raw,'budget':budget,'carrier_options':carrier})
 # Fixed common-header partial; no Message Type/Length may be invented.
 add(1,'short-common-one-byte',message_wire(model,session())[:1])
 add(2,'declared-message-under-fixed-header',message_wire(model,b'',length=4))
 add(3,'true-snaplen-short-last-primitive',message_wire(model,session()+time_values()),truncate=1)
 add(4,'whole-declared-message-with-unselected-tail',message_wire(model,session())+time_values(0x01020304))
 add(5,'object-header-three-bytes',message_wire(model,struct.pack('!HB',12,1)))
 add(6,'object-length-under-four',message_wire(model,object_wire(1,1,b'',length=3)))
 add(7,'object-length-not-multiple-four',message_wire(model,object_wire(1,1,bytes([1,2]),length=6)))
 add(8,'object-length-crosses-rsvp-parent',message_wire(model,object_wire(1,1,ipaddress.ip_address('203.0.113.9').packed,length=12)))
 add(9,'complete-object-selected-body-too-short',message_wire(model,object_wire(1,1,ipaddress.ip_address('203.0.113.9').packed)))
 add(10,'unknown-class-then-known-sibling',message_wire(model,object_wire(250,1,bytes([1,2,3,4]))+time_values()))
 add(11,'known-class-unknown-ctype-then-known-sibling',message_wire(model,object_wire(1,99,bytes([5,6,7,8]))+time_values()))
 budget_wire=message_wire(model,session()+time_values())
 add(12,'object-budget-one',budget_wire,budget=1)
 bad=bytearray(budget_wire)
 oldsum=int.from_bytes(bad[2:4],'big')
 bad[2:4]=struct.pack('!H',oldsum^1 if oldsum^1 else oldsum^2)
 add(13,'bad-message-checksum-plus-budget-one',bytes(bad),budget=1)
 nested=message_wire(model,session())+message_wire(model,time_values())
 add(14,'unselected-Bundle-message-body-is-not-object-chain',message_wire(model,nested,kind=12))
 add(15,'checksum-omitted-and-unused-object-bits-nonzero',message_wire(model,session()+sender(unused=0xbeef),omit_checksum=True))
 v6hop=object_wire(3,2,ipaddress.ip_address('2001:db8::1').packed+struct.pack('!I',0x01020304))
 scope6=object_wire(7,2,ipaddress.ip_address('2001:db8::1').packed+ipaddress.ip_address('2001:db8::2').packed)
 add(16,'IPv6-baseline-Path-fields-and-SCOPE',message_wire(model,session(True)+v6hop+time_values()+sender(True)+scope6),outer='v6')
 error6=object_wire(6,2,ipaddress.ip_address('2001:db8:20::b').packed+struct.pack('!BBH',0,0,0))
 confirm6=object_wire(15,2,ipaddress.ip_address('2001:db8:10::a').packed)
 style=object_wire(8,1,struct.pack('!I',10))
 filter6=object_wire(10,2,ipaddress.ip_address('2001:db8:20::b').packed+struct.pack('!HH',0,40000))
 add(16,'IPv6-baseline-ResvConf-fields',message_wire(model,session(True)+error6+confirm6+style+filter6,kind=7),outer='v6',src='2001:db8:ffff::2',dst='2001:db8:ffff::1')
 scope4=object_wire(7,1,ipaddress.ip_address('198.51.100.1').packed+ipaddress.ip_address('198.51.100.2').packed)
 add(17,'SCOPE-shared-budget-includes-address-nodes',message_wire(model,scope4+time_values(),kind=4),budget=2)
 partial6=object_wire(7,2,ipaddress.ip_address('2001:db8::1').packed+ipaddress.ip_address('2001:db8::2').packed[:4])
 add(18,'IPv6-SCOPE-whole-address-plus-partial-address',message_wire(model,partial6,kind=4),outer='v6')
 for label,period in [('B-first',2000),('A-delayed',1000),('A-duplicate',1000)]:
  add(19,'authored-record-permutation-'+label,message_wire(model,session()+time_values(period)))
 selector=message_wire(model,session()+sender())
 add(20,'advertised-selector-same-peer-A',selector,src='192.0.2.11',dst='192.0.2.12')
 add(20,'advertised-selector-reuse-peer-B',selector,src='192.0.2.21',dst='192.0.2.22')
 assert len({r['scene']for r in scenes})==20 and len(scenes)==24
 frames=[];facts=[];probe=[]
 for number,row in enumerate(scenes,1):
  physical,raw,anchor=framed(model,row['raw'],number,**row['carrier_options'])
  pdu_path=out/'pdu'/f'{number:02d}.bin';pdu_path.write_bytes(raw)
  frame_path=out/'physical'/f'{number:02d}.bin';frame_path.write_bytes(physical)
  evaluation=model.decode_source(raw,row['budget'])
  item={'sourcecase':f'RSVP-authored-scene{row["scene"]:02d}-frame{number:02d}',
        'scene':row['scene'],'name':row['name'],'frame':number,'domain':{'section':0,'interface':0,'link':1},
        'kind':'authored synthetic minimal carrier; not captured traffic or full procedure proof',
        'PDU_path':str(pdu_path),'PDU_SHA256':dg(raw),'physical_path':str(frame_path),'physical_SHA256':dg(physical),
        'time_ns':(1700000000+number)*10**9,'carrier':anchor,'budget':row['budget'],
        'source':evaluation,'source_no_transport_connection_or_session_identity':True}
  if number==1:
   assert evaluation['fields']['Header'].keys()=={'Version and Flags',model.WR,'Version','Flags'}
  if row['scene']==3:
   assert not anchor['parent_complete'] and any(i['kind']=='need_more'for i in evaluation['issues']) and not any(i['kind']=='format'for i in evaluation['issues'])
  if row['scene']==4:
   assert evaluation['fields']['Checksum Verified'] and len(evaluation['fields']['Objects'])==1 and len(base64.b64decode(evaluation['fields']['Trailing Bytes']))==8 and model.checksum(raw)!=0
  if row['scene']in(10,11):
   assert len(evaluation['fields']['Objects'])==2 and evaluation['fields']['Objects'][1]['Body']['Refresh Period Milliseconds']==1000 and not evaluation['fields']['Field Layouts Complete']
  if row['scene']==13:assert {i['kind']for i in evaluation['issues']}=={'integrity','budget'}
  if row['scene']==14:assert 'Objects'not in evaluation['fields'] and 'Unparsed Body'in evaluation['fields']
  if row['scene']==15:assert evaluation['fields']['Checksum Omitted'] and not evaluation['fields']['Checksum Verified'] and not evaluation['issues']
  if row['scene']==17:
   addresses=evaluation['fields']['Objects'][0]['Body']['Addresses']
   assert len(addresses)==1 and evaluation['source_observation']['collection_tokens_used']==2 and any(i['kind']=='budget'for i in evaluation['issues'])
  if row['scene']==18:
   addresses=evaluation['fields']['Objects'][0]['Body']['Addresses']
   assert len(addresses)==1 and len(base64.b64decode(evaluation['fields']['Unparsed Body']))==4 and any(i['kind']=='format'for i in evaluation['issues'])
  facts.append(item);frames.append((physical,anchor))
  probe.append({'sourcecase':item['sourcecase'],'input_path':str(pdu_path),'byte_offset':0,'byte_length':len(raw),'raw_SHA256':dg(raw),'budget':row['budget'],'parent_complete':anchor['parent_complete']})
 # Only one extra evaluation, same physical input, to prove budget zero
 # preserves the common header and creates no object/default primitives.
 ordinary=next(r for r in facts if r['scene']==12)
 zero=model.decode_source(Path(ordinary['PDU_path']).read_bytes(),0)
 assert zero['fields']['Objects']is None and len(zero['fields']['Header'][model.WR])==6
 assert {i['kind']for i in zero['issues']}=={'budget'}
 extra={'sourcecase':ordinary['sourcecase']+'-budget0','same_physical_frame':ordinary['frame'],'PDU_path':ordinary['PDU_path'],'PDU_SHA256':ordinary['PDU_SHA256'],'budget':0,'source':zero}
 put(out/'extra-budget-evaluation.json',extra)
 probe.append({'sourcecase':extra['sourcecase'],'input_path':extra['PDU_path'],'byte_offset':0,'byte_length':len(Path(extra['PDU_path']).read_bytes()),'raw_SHA256':extra['PDU_SHA256'],'budget':0,'parent_complete':True})
 write_pcap(out/'authored-controls.pcap',frames)
 put(out/'source-facts.json',facts)
 put(out/'baseline-leaf-bridge.json',bridge)
 (out/'probe-input.jsonl').write_text(''.join(json.dumps(r)+'\n'for r in probe),encoding='utf-8')
 contract={'scope':'20 authored boundary scenes; 24 physical records, plus one same-wire zero-budget evaluation; not real traffic',
 'literal_ranges':'Wire Field Ranges on every complete literal owned field; all offsets are half-open logical RSVP-PDU offsets',
 'derived':'Version/Flags only derived; mask/shift source range exists outside WR, no fake literal range',
 'partial_common':'fixed common header: retain each complete captured primitive including Length even when Length<8; do not parse objects after underheader contradiction',
 'partial_object':'keep full captured object-header primitives even when Length<4; fewer than2bytes cannot manufacture an empty row',
 'partial_selected':'observe every complete primitive in selected layout within declared/captured intersection; no invented missing field; preserve exact remaining owned bytes',
 'parent_distinction':'true snaplen-short record has original length>captured and incomplete IP declared extent; complete-parent object extent contradiction is format evidence',
 'collection_budget':'one shared token per object node and SCOPE address node; common header/scalars use none; budget0 retains common header',
 'checksum':'only complete valid declared RSVP extent, no pseudo-header; zero omitted; bad nonzero checksum independent of budget/context/format; physical/trailing bytes outside declared PDU never enter sum',
 'selected_layouts':'IPv4 C-Type1 baseline8; IPv6 C-Type2 only SESSION/HOP/ERROR/FILTER/SENDER/CONFIRM; TIME/STYLE C-Type1; SCOPE C-Type1 IPv4 or2 IPv6',
 'unknown':'unknown class/C-Type body opaque and next independently declared sibling may be observed; Field Layouts Complete=false; observing is not accepting/forwarding unknown objects',
 'unknown_message':'outside profile1..7 including Bundle12: common header+owned Unparsed Body, Objects absent; do not parse body as generic object chain',
 'SCOPE':'addresses in authored full lists are numerically ascending; no new ordering refusal is prescribed; partial last address never invents an address',
 'unused':'unused sender/filter bits preserved literally and ignored on receipt (RFC2205 Appendix A); not automatic format',
 'permutation':'scene19 changes authored record order and duplicates wire; no real network reordering/loss causality is claimed because no selected RSVP sequence field exists',
 'selector_reuse':'scene20 repeats advertised SESSION+SENDER tuple across different physical IP peers; no captured UDP/TCP flow, authenticated adjacency, reservation or state is inferred',
 'not_proved':['full per-message mandatory object grammar','Intserv service semantics','Bundle/reduction processing','Integrity/MAC/replay','real live network fault recovery','native admission/CLI/typed snapshot correctness'],
 'primary_sections':{'common':'RFC2205 3.1.1','object':'RFC2205 3.1.2','unknown':'RFC2205 3.10','unused':'RFC2205 Appendix A','selected_IPv4_IPv6':'RFC2205 A.1/A.2/A.4/A.5/A.7/A.9/A.10/A.14','Scope':'RFC2205 A.6','Bundle_not_selected':'RFC2961 3','Integrity_not_selected':'RFC2747 1/2.1'}}
 put(out/'source-contract.json',contract)
 assert before=={p:sha(p)for p in before}
 outputs={str(p):sha(p)for p in out.rglob('*')if p.is_file()}
 manifest={'inputs':before,'outputs':outputs,'frozen_original_baseline_unchanged':True,'source_cases':25,'physical_frames':24,'scenes':20,'no_future_Go_source_or_candidate_output_read':True,'no_binary_Go_build_replay_or_TShark_started':True,'all_values_from_authored_raw_source':True,'helper':str(model_path),'helper_SHA256':sha(model_path),'builder':str(Path(__file__)),'builder_SHA256':sha(__file__)}
 put(out/'manifest.json',manifest)
 evaluations=[r['source']for r in facts]+[zero]
 summary={'manifest_SHA256':sha(out/'manifest.json'),'source_facts_SHA256':sha(out/'source-facts.json'),'capture_SHA256':sha(out/'authored-controls.pcap'),'probe_input_SHA256':sha(out/'probe-input.jsonl'),'scene_count':20,'physical_records':24,'evaluations':25,'baseline_bridge_frames':9,'baseline_literal_leaves_unchanged':332,'baseline_derived_leaves_unchanged':18,'evaluated_literal_wire_leaves':sum(sum(x['literal']for x in r['leaves'])for r in evaluations),'evaluated_derived_bit_fields':sum(sum(not x['literal']for x in r['leaves'])for r in evaluations),'source_only':True,'reference_or_product_correctness_not_yet_claimed':True}
 put(out/'summary.json',summary)
 print(json.dumps({'summary':str(out/'summary.json'),'SHA256':sha(out/'summary.json'),'counts':{k:v for k,v in summary.items()if isinstance(v,int)}}))


def main():
 parser=argparse.ArgumentParser(description=__doc__)
 parser.add_argument('--phase',required=True,type=Path)
 parser.add_argument('--out',required=True,type=Path)
 args=parser.parse_args();run(args.phase,args.out)
if __name__=='__main__':main()