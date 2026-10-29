"""Independently bind RSVP parent-refusal events to original PCAPNG headers.

No Go, binaries, TShark or repository source is read/executed. Runtime labels
select audit targets but never prove their IP/RSVP validity. Full original wire
is not copied; original file offsets/SHA and bounded header facts are retained
outside the repository. No generated-ID or arbitrary field normalization.
"""
import argparse,collections,hashlib,ipaddress,json,struct
from pathlib import Path

def dg(w):return hashlib.sha256(w).hexdigest()
def sha(p):
 h=hashlib.sha256()
 with Path(p).open('rb')as f:
  for chunk in iter(lambda:f.read(1048576),b''):h.update(chunk)
 return h.hexdigest()
def read(p):return json.loads(Path(p).read_text(encoding='utf-8-sig'))
def put(p,x):Path(p).write_text(json.dumps(x,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
def checksum(w):
 w+=bytes(len(w)%2);s=sum(int.from_bytes(w[i:i+2],'big')for i in range(0,len(w),2))
 while s>>16:s=(s&65535)+(s>>16)
 return(~s)&65535

def events(path,targets=None,expert=None):
 h=hashlib.sha256();rows=[];total=0
 with Path(path).open('rb')as f:
  for line in f:
   h.update(line)
   if not line.strip():continue
   total+=1
   if expert and expert.encode()not in line:continue
   e=json.loads(line)
   refs=e['source_bytes'].get('PacketRefs')or[]
   if targets is not None and not any(r['Number']in targets for r in refs):continue
   rows.append(e)
 return rows,h.hexdigest(),total

def capture_sources(path,wanted):
 h=hashlib.sha256();endian=None;section=-1;interfaces=[];number=0;at=0;out={};blocks=collections.Counter()
 with Path(path).open('rb')as f:
  while True:
   offset=at;head=f.read(8)
   if not head:break
   assert len(head)==8
   if head[:4]==bytes.fromhex('0a0d0d0a'):
    magic=f.read(4);assert len(magic)==4
    endian='<'if magic==bytes.fromhex('4d3c2b1a')else'>'if magic==bytes.fromhex('1a2b3c4d')else None;assert endian
    length=struct.unpack(endian+'I',head[4:])[0];assert length>=28 and length%4==0
    rest=f.read(length-12);raw=head+magic+rest;section+=1;interfaces=[]
   else:
    assert endian;length=struct.unpack(endian+'I',head[4:])[0];assert length>=12 and length%4==0
    raw=head+f.read(length-8)
   assert len(raw)==length and struct.unpack_from(endian+'I',raw,length-4)[0]==length
   h.update(raw);at+=length;typ=struct.unpack_from(endian+'I',raw,0)[0];blocks[typ]+=1
   if typ==1:
    link,reserved,snap=struct.unpack_from(endian+'HHI',raw,8);interfaces.append({'link':link,'snaplen':snap})
   elif typ in(6,2,3):
    number+=1
    if typ in(6,2):
     interface=struct.unpack_from(endian+('I'if typ==6 else'H'),raw,8)[0];cap,orig=struct.unpack_from(endian+'II',raw,20);start=28
    else:
     interface=0;orig=struct.unpack_from(endian+'I',raw,8)[0];cap=min(orig,interfaces[0]['snaplen']);start=12
    assert interface<len(interfaces)and start+cap<=length-4
    if number not in wanted:continue
    wire=raw[start:start+cap];config=interfaces[interface]
    assert config['link']==1 and wire[12:14]==bytes.fromhex('0800')and len(wire)>=34
    version=wire[14]>>4;ihl=(wire[14]&15)*4;iplen=int.from_bytes(wire[16:18],'big');frag=int.from_bytes(wire[20:22],'big');proto=wire[23]
    assert version==4 and ihl>=20 and 14+ihl<=cap
    src=str(ipaddress.IPv4Address(wire[26:30]));dst=str(ipaddress.IPv4Address(wire[30:34]));headerend=14+ihl;ipend=14+iplen
    reasons=[]
    if iplen<ihl:reasons.append('declared-IP-length-below-captured-header')
    if ipend>cap:reasons.append('declared-IP-end-exceeds-captured-frame')
    out[number]={'frame':number,'section':section,'interface':interface,'link':config['link'],'original_capture_file_offset':offset+start,'captured_length':cap,'original_length':orig,'original_frame_SHA256':dg(wire),'IP_version':version,'IP_header_range':[14,headerend],'IP_header_SHA256':dg(wire[14:headerend]),'IP_header_checksum_valid':checksum(wire[14:headerend])==0,'IP_total_length':iplen,'IP_declared_end':ipend,'IP_header_bytes':ihl,'IP_protocol':proto,'literal_direct_IP46':proto==46,'as_if_IPv6_extension_under_IPv4':({'extension_kind':'IPv6Routing'if proto==43 else'IPv6Destination'if proto==60 else'IPv6HopByHop','next_header':wire[headerend]if headerend<cap else None,'header_length_octet':wire[headerend+1]if headerend+1<cap else None,'first_fixed_extension_range':[headerend,min(headerend+8,cap)],'wrong_IP_family_for_this_extension':True}if proto in(0,43,60)else None),'IP_source_range':[26,30],'IP_destination_range':[30,34],'IP_source':src,'IP_destination':dst,'fragment_field':frag,'fragment_offset_bytes':(frag&8191)*8,'more_fragments':bool(frag&8192),'available_after_IP_header':cap-headerend,'complete_capture_record':cap==orig,'independent_parent_invalid_reasons':reasons}
 assert set(out)==wanted,'missing target physical records'
 return out,h.hexdigest(),{'physical_records':number,'block_counts':dict(blocks),'capture_bytes':at}

def run(phase,out):
 assert not out.exists();out.mkdir()
 olddir=phase/'runtime-public-native-45/preservation/sharon';newdir=phase/'runtime-public-native-46/preservation/sharon'
 op=read(olddir/'events-producer.json');np=read(newdir/'events-producer.json');capture=Path(np['capture']);assert op['capture']==np['capture']and op['capture_SHA256']==np['capture_SHA256']
 new,newhash,newtotal=events(newdir/'events.jsonl',expert='RSVPParentLengthInvalid');assert newhash==np['export_SHA256']and len(new)==937
 newby={e['source_bytes']['PacketRefs'][0]['Number']:e for e in new};assert len(newby)==937
 oldcensuspath=phase/'RSVP-independent-source45-02/input-35.json';oldcensus=read(oldcensuspath);oldframes={r['frame']for r in oldcensus};assert len(oldframes)==904
 union=set(newby)|oldframes;physical,caphash,capcounts=capture_sources(capture,union);assert caphash==np['capture_SHA256']=='aa4fcc81af216e2b788171c64271b43681974d9156aa9ddbdc1a7a25e730714c'
 old,oldhash,oldtotal=events(olddir/'events.jsonl',targets=union);assert oldhash==op['export_SHA256'];oldby=collections.defaultdict(list)
 for e in old:
  for ref in e['source_bytes'].get('PacketRefs')or[]:
   if ref['Number']in union:oldby[ref['Number']].append(e)
 stats=collections.Counter();groups={'intersection':set(newby)&oldframes,'new_only':set(newby)-oldframes,'old_census_only':oldframes-set(newby)};groupstats={};endpoints=[]
 for label,numbers in groups.items():
  counter=collections.Counter()
  for number in sorted(numbers):
   row=physical[number];counter['frames']+=1;counter['literal_direct_IP46']+=row['literal_direct_IP46'];counter['wrong_family_IPv6_extension']+=row['as_if_IPv6_extension_under_IPv4']is not None;counter['IP_length_zero']+=row['IP_total_length']==0;counter['IP_length_below_header']+=row['IP_total_length']<row['IP_header_bytes'];counter['IP_end_exceeds_capture']+=row['IP_declared_end']>row['captured_length'];counter['has_fragment_offset_or_MF']+=bool(row['fragment_offset_bytes']or row['more_fragments']);counter['payload_less_than_common8']+=row['available_after_IP_header']<8;counter['bad_IP_header_checksum']+=not row['IP_header_checksum_valid'];counter['snaplen_short']+=not row['complete_capture_record']
  groupstats[label]=dict(counter)
 for number,e in sorted(newby.items()):
  row=physical[number];assert row['independent_parent_invalid_reasons']and row['complete_capture_record']and row['fragment_offset_bytes']==0 and not row['more_fragments']
  assert e['protocol']=='ip'and e['status']=='malformed'and e['profile']=='network-header-refusal'and e['expert']=='RSVPParentLengthInvalid'
  if e['source']or e['destination']:
   assert e['source']==row['IP_source']and e['destination']==row['IP_destination'];stats['provided_endpoint_pairs_source_proved']+=1
  else:stats['empty_endpoint_pairs_not_fabricated']+=1
  domain={'Section':row['section'],'Interface':row['interface'],'Encapsulation':''}
  assert e['domain']==domain and e['source_bytes']['PacketRefs']==[{'Number':number,'Domain':domain}]
  assert not e.get('raw')and e['source_bytes']['SHA256']==''and all(e.get(key,0)==0 for key in('flow','tcp_flow_id','transaction','response_to','direction'))
  assert e['session']['Child Header Identity']=='unconfirmed'and not any(v['name'].startswith('rsvp.')for v in e['field_values']['values'])
  pairs=oldby[number];assert len(pairs)<=1
  if pairs:
   previous=pairs[0];assert previous['protocol']=='ip'and previous['status']=='unrecognized'and previous['expert']=='UnsupportedOrMalformedNetworkLayer';assert previous['source']==previous['destination']=='';stats['old_callback_endpoint_promotion_source_proved']+=1
   oldid=previous['id']
  else:stats['new_callback_with_no_old_callback']+=1;oldid=None
  endpoints.append({'frame':number,'old_generated_ID':oldid,'new_generated_ID':e['id'],'source_IP_observation':row['IP_source'],'destination_IP_observation':row['IP_destination'],'actual_event_source':e['source'],'actual_event_destination':e['destination'],'source_range':row['IP_source_range'],'destination_range':row['IP_destination_range'],'source_parent_invalid_reasons':row['independent_parent_invalid_reasons'],'all_nonempty_endpoint_values_independently_source_proved':True,'empty_values_not_backfilled':True,'literal_direct_IP46':row['literal_direct_IP46'],'RSVP_specific_refusal_source_selected':row['literal_direct_IP46']})
 assert stats['old_callback_endpoint_promotion_source_proved']==935 and stats['new_callback_with_no_old_callback']==2
 newmissing=[number for number in sorted(newby)if not oldby[number]];assert newmissing==[218724,279605]
 put(out/'physical-source-facts.json',[physical[n]for n in sorted(physical)])
 put(out/'new-refusal-endpoint-proof.json',endpoints)
 put(out/'set-ledger.json',{'old_census904':sorted(oldframes),'new_refusals937':sorted(newby),'groups':{label:sorted(numbers)for label,numbers in groups.items()},'new_callbacks_without_old':[{'frame':n,'source':physical[n]}for n in newmissing],'old_census_only_old_native_outcomes':[{ 'frame':n,'old_events':[{'id':e['id'],'protocol':e['protocol'],'status':e['status'],'expert':e['expert'],'profile':e['profile']}for e in oldby[n]]}for n in sorted(groups['old_census_only'])]})
 inputs={str(path):sha(path)for path in [Path(__file__),oldcensuspath,olddir/'events-producer.json',newdir/'events-producer.json']}
 inputs.update({str(olddir/'events.jsonl'):oldhash,str(newdir/'events.jsonl'):newhash,str(capture):caphash})
 put(out/'summary.json',{'inputs':inputs,'capture':capcounts,'old_event_rows':oldtotal,'new_event_rows':newtotal,'old_census_frames':904,'new_parent_refusals':937,'union_physical_frames':len(union),'set_counts':{label:len(numbers)for label,numbers in groups.items()},'group_source_facts':groupstats,'source_proved_counts':dict(stats),'net_new_physical_callbacks':newmissing,'outputs':{str(path):sha(path)for path in out.iterdir()if path.is_file()},'new_all937_parent_invalid_and_no_RSVP_identity':True,'new_RSVP_specific_refusal_wrong_IP_family_frames':[n for n in sorted(newby)if not physical[n]['literal_direct_IP46']],'RSVP_source_scope_cannot_authorize_all937_as_literalIP46':True,'only_target935_old_endpoint_changes_source_authorized':True,'unaffected_event_or_ID_mapping_not_claimed':True,'capture_decoder_error_count_change_not_yet_proved':True,'no_Go_runtime_TShark_or_repo_source_read':True})
 print(json.dumps({'summary':str(out/'summary.json'),'SHA256':sha(out/'summary.json'),'set_counts':{label:len(numbers)for label,numbers in groups.items()},'groupfacts':groupstats,'new_missing':newmissing,'source_counts':dict(stats)}))

def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--phase',type=Path,required=True);p.add_argument('--out',type=Path,required=True);a=p.parse_args();run(a.phase,a.out)
if __name__=='__main__':main()