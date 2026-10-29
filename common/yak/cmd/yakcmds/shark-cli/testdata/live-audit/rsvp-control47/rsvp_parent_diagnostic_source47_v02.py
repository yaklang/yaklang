"""Source-built RSVP parent diagnostic contract after the IP-family fix.

Scans original PCAPNG independently; no candidate47 output is read. Generated
PDU IDs are an explicit caller argument for an independently proved ledger,
never inferred/mapped from labels. Complete expected JSON objects stay external.
"""
import argparse,collections,copy,hashlib,ipaddress,json,struct
from pathlib import Path

def dg(w):return hashlib.sha256(w).hexdigest()
def sha(path):
 h=hashlib.sha256()
 with Path(path).open('rb')as f:
  for w in iter(lambda:f.read(1048576),b''):h.update(w)
 return h.hexdigest()
def read(p):return json.loads(Path(p).read_text(encoding='utf-8-sig'))
def put(p,v):Path(p).write_text(json.dumps(v,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
def options(header):
 pos=20;trace=[]
 while pos<len(header):
  kind=header[pos]
  if kind==0:trace.append({'kind':0,'range':[14+pos,14+pos+1]});return trace,None
  if kind==1:trace.append({'kind':1,'range':[14+pos,14+pos+1]});pos+=1;continue
  if pos+2>len(header):return trace,{'reason':'option length byte unavailable','offset':14+pos,'kind':kind}
  length=header[pos+1]
  if length<2:return trace,{'reason':'option length below2','offset':14+pos,'kind':kind,'length':length}
  if pos+length>len(header):return trace,{'reason':'option crosses IPv4 IHL','offset':14+pos,'kind':kind,'length':length,'available':len(header)-pos}
  trace.append({'kind':kind,'range':[14+pos,14+pos+length]});pos+=length
 return trace,None

def scan(path):
 h=hashlib.sha256();endian=None;section=-1;interfaces=[];number=0;at=0;selected=[];badoptions=[];counts=collections.Counter()
 with Path(path).open('rb')as f:
  while True:
   offset=at;head=f.read(8)
   if not head:break
   assert len(head)==8
   if head[:4]==bytes.fromhex('0a0d0d0a'):
    magic=f.read(4);endian='<'if magic==bytes.fromhex('4d3c2b1a')else'>'if magic==bytes.fromhex('1a2b3c4d')else None;assert endian
    size=struct.unpack(endian+'I',head[4:])[0];assert size>=28 and size%4==0;block=head+magic+f.read(size-12);section+=1;interfaces=[]
   else:
    assert endian;size=struct.unpack(endian+'I',head[4:])[0];assert size>=12 and size%4==0;block=head+f.read(size-8)
   assert len(block)==size and struct.unpack_from(endian+'I',block,size-4)[0]==size;h.update(block);at+=size;typ=struct.unpack_from(endian+'I',block)[0]
   if typ==1:
    link,reserved,snap=struct.unpack_from(endian+'HHI',block,8);interfaces.append({'link':link,'snap':snap})
   elif typ in(6,2,3):
    number+=1
    if typ in(6,2):iface=struct.unpack_from(endian+('I'if typ==6 else'H'),block,8)[0];cap,orig=struct.unpack_from(endian+'II',block,20);start=28
    else:iface=0;orig=struct.unpack_from(endian+'I',block,8)[0];cap=min(orig,interfaces[0]['snap']);start=12
    assert iface<len(interfaces)and start+cap<=size-4
    wire=block[start:start+cap]
    if interfaces[iface]['link']!=1 or len(wire)<34 or wire[12:14]!=bytes.fromhex('0800')or wire[23]!=46:continue
    counts['all_direct_Ethernet_IPv4_literal_protocol46']+=1
    version=wire[14]>>4;ihl=(wire[14]&15)*4
    if version!=4 or ihl<20 or 14+ihl>len(wire):counts['fixed_header_unselected_or_short']+=1;continue
    iplen=int.from_bytes(wire[16:18],'big');frag=int.from_bytes(wire[20:22],'big');trace,error=options(wire[14:14+ihl])
    base={'frame':number,'section':section,'interface':iface,'link':1,'original_file_offset':offset+start,'captured_length':cap,'original_length':orig,'frame_SHA256':dg(wire),'IP_header_range':[14,14+ihl],'IP_header_SHA256':dg(wire[14:14+ihl]),'IP_protocol':46,'IP_total_length':iplen,'IP_declared_end':14+iplen,'IP_header_bytes':ihl,'IP_source':str(ipaddress.IPv4Address(wire[26:30])),'IP_destination':str(ipaddress.IPv4Address(wire[30:34])),'IP_source_range':[26,30],'IP_destination_range':[30,34],'fragment_offset_bytes':(frag&8191)*8,'more_fragments':bool(frag&8192),'available_payload_bytes':cap-14-ihl,'options_trace':trace}
    if error:badoptions.append(base|{'options_error':error});counts['invalid_IPv4_options']+=1;continue
    if frag&0x3fff:counts['fragment_not_admitted_to_direct_RSVP']+=1;continue
    invalid=iplen<ihl or 14+iplen>cap
    if not invalid:counts['complete_bounded_direct_IP_parent']+=1;continue
    if cap!=orig:counts['real_snaplen_short_not_this_refusal_contract']+=1;continue
    # The old public endpoint extractor excludes a declared IP length below
    # the fixed/IHL header. Retain empty endpoints rather than promote raw
    # address bytes into an admitted network identity for those records.
    visible=iplen>=ihl
    selected.append(base|{'endpoint_policy':'literal-header-addresses'if visible else'empty-unselected-declared-IP-length','expected_event_source':base['IP_source']if visible else'','expected_event_destination':base['IP_destination']if visible else'','source_parent_invalid_reason':'declared-IP-length-below-header'if iplen<ihl else'declared-IP-end-exceeds-complete-capture'})
 return selected,badoptions,h.hexdigest(),{'physical_records':number,'bytes':at,'census':dict(counts)}

def expected_diagnostic(case,pdu_id):
 """Complete exported DTO using old metadata schema and a caller-owned ID."""
 assert type(pdu_id)is int and pdu_id>0
 domain={'Section':case['section'],'Interface':case['interface'],'Encapsulation':''};ref={'Number':case['frame'],'Domain':domain};source=case['expected_event_source'];destination=case['expected_event_destination']
 byte_source={'Kind':'captured','SHA256':'','ParentPDU':0,'ParentPDUs':None,'PacketRefs':[ref]}
 values=[]
 if destination:values.append({'name':'destination','value':destination,'origin':'event'})
 values.append({'name':'flow.id','value':0,'exact_value':'0','origin':'event'})
 if source:
  values.extend([{'name':'ip.addr','value':source,'origin':'event'},{'name':'ip.addr','value':destination,'origin':'event'}])
 values.extend([{'name':'pdu.id','value':pdu_id,'exact_value':str(pdu_id),'origin':'event'},{'name':'pdu.length','value':0,'exact_value':'0','origin':'event'},{'name':'protocol','value':'ip','origin':'event'}])
 if source:values.append({'name':'source','value':source,'origin':'event'})
 values.append({'name':'status','value':'malformed','origin':'event'})
 session={'Child Header Identity':'unconfirmed','Header Observation Scope':'validated network prefix before refused child header','Observed Header Layers':['Ethernet','IPv4'],'Parent Header Refusal':'RSVPParentLengthInvalid'}
 return {'admission':'','carrier_layers':None,'completeness':'malformed','destination':destination,'direction':0,'domain':domain,'entry':'','error':'RSVP parent extent invalid; control body not admitted','expert':'RSVPParentLengthInvalid','field_values':{'schema_version':1,'pdu':pdu_id,'source':copy.deepcopy(byte_source),'values':values,'completeness':'malformed'},'fields':None,'fields_error':'protocol parser: no exact rule for this message','flow':0,'id':pdu_id,'profile':'network-header-refusal','protocol':'ip','raw':None,'response_to':0,'session':session,'source':source,'source_bytes':byte_source,'status':'malformed','summary':'RSVP parent extent invalid; control body not admitted; child identity not admitted','transaction':0,'transport':'network'}

def prepare(phase,out):
 assert not out.exists();out.mkdir()
 original=phase/'RSVP-independent-source45-02/input-35.json';oldcensus=read(original);capture=Path(r'C:\tmp\yak-shark-public-corpus-20261024\sharon-20220213\ultimate_wireshark_protocols_pcap_220213.pcap')
 cases,badopts,caphash,stats=scan(capture);assert caphash=='aa4fcc81af216e2b788171c64271b43681974d9156aa9ddbdc1a7a25e730714c'
 by={case['frame']:case for case in cases};assert len(by)==len(cases)==936 and 218724 not in by and 279605 in by
 oldframes={row['frame']for row in oldcensus};badframes={row['frame']for row in badopts}
 assert len(oldframes)==904 and len(oldframes-set(by))==233 and oldframes-set(by)<=badframes
 assert len(set(by)-oldframes)==265
 assert all(case['section']==0 and case['interface']==226 for case in cases)
 assert sum(bool(case['expected_event_source'])for case in cases)==671
 assert sum(not case['expected_event_source']for case in cases)==265
 # A previous closed generation supplies only the public DTO schema and
 # generated identity slots for this concordance. No future47 export is read.
 previous=phase/'runtime-public-native-46/preservation/sharon/events.jsonl';h=hashlib.sha256();seen=set();concordance=0
 with previous.open('rb')as f:
  for line in f:
   h.update(line)
   if b'RSVPParentLengthInvalid'not in line:continue
   event=json.loads(line);frame=event['source_bytes']['PacketRefs'][0]['Number']
   if frame==218724:continue
   assert frame in by and frame not in seen;seen.add(frame)
   expected=expected_diagnostic(by[frame],event['id'])
   assert event==expected,'old schema source-concordance differs at '+str(frame)
   concordance+=1
 assert seen==set(by)and concordance==936
 prod=read(phase/'runtime-public-native-46/preservation/sharon/events-producer.json');assert h.hexdigest()==prod['export_SHA256']
 # No generated IDs are saved in source cases. A future full ledger must
 # provide IDs from its independent arrival mapping, not from this check.
 put(out/'source-cases.json',cases);put(out/'bad-IPv4-options-source-facts.json',badopts)
 put(out/'metadata-schema-and-scope.json',{'expected_DTO_constructor':'expected_diagnostic(source_case,pdu_id)','ID_policy':'explicit identity slot supplied by an independently proved whole ledger; no arbitrary ID map or prefilled candidate47 IDs','source_scope':'original PCAPNG direct Ethernet/IPv4 protocol46 only, valid captured fixed/options header, no fragment, complete CI record, invalid declared IP extent','source_endpoints':'671 literal header endpoint pairs;265 empty for declared IP length0; no backfill from unadmitted bytes; 670 old endpoint changes plus true new279605 nonempty pair','old_callback_policy':'935 prior ip/unrecognized targets may be reclassified to exact source-built parent diagnostic; other retained objects remain strict;279605 genuine new empty-payload parent diagnostic;218724 wrong-family child selection excluded','no_RSVP_identity':'protocol ip/malformed;child unconfirmed,no rsvp aliases,no flow/tcp/transaction/response,no raw child/body','unknown_metadata':'no optional fields accepted by normalization; full exported old-schema dictionary is literal compared','capture_decoder_error_counts':'not certified by this source contract; parent/child rejection versus capture ErrorLayer delta still requires separate evidence'})
 inputs={str(Path(__file__)):sha(__file__),str(original):sha(original),str(capture):caphash,str(previous):h.hexdigest(),str(phase/'runtime-public-native-46/preservation/sharon/events-producer.json'):sha(phase/'runtime-public-native-46/preservation/sharon/events-producer.json')}
 put(out/'manifest.json',{'inputs':inputs,'outputs':{str(path):sha(path)for path in out.iterdir()if path.is_file()},'no_candidate47_output_or_Go_source_read':True,'no_Go_binary_TShark_runtime_or_replay':True,'source_cases':936,'previous_schema_concordance_not_future_producer_proof':True})
 put(out/'summary.json',{'manifest_SHA256':sha(out/'manifest.json'),'source_cases_SHA256':sha(out/'source-cases.json'),'original_capture_scan':stats,'source_selected_parent_diagnostics':936,'excluded_wrong_family_frame':218724,'new_literal46_empty_payload_frame':279605,'old_census_only_invalid_options':dict(collections.Counter(row['options_error']['reason']for row in badopts if row['frame']in oldframes-set(by))),'all_direct_IP46_invalid_options':dict(collections.Counter(row['options_error']['reason']for row in badopts)),'endpoint_pairs_literal':671,'endpoint_pairs_empty':265,'prior_callback_target_count':935,'actual_old_endpoint_value_changes':670,'source_concordance_with_previous_schema_only':concordance,'generated_ID_expectations_not_saved':True,'whole47_actual_not_yet_read_or_passed':True})
 print(json.dumps({'summary':str(out/'summary.json'),'SHA256':sha(out/'summary.json'),'manifest_SHA256':sha(out/'manifest.json'),'source_cases_SHA256':sha(out/'source-cases.json'),'census':stats}))

def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--phase',type=Path,required=True);p.add_argument('--out',type=Path,required=True);a=p.parse_args();prepare(a.phase,a.out)
if __name__=='__main__':main()