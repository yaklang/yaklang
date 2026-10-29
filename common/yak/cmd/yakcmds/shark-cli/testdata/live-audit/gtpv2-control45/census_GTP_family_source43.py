"""Bounded original-wire GTPv1/v2 candidate census for the closed37 inputs.

No native/CLI candidate is read. UDP ports are observed hints only, not protocol
proof. Each selected message has literal version/type/declared extent, optional
header fields and bounded v2 IE headers. Bodies/identifiers are not authenticated;
NAS/subscriber data stays opaque length/hash. RSVP/RMCP are inventory hints only.
"""
import argparse,collections,hashlib,importlib.util,ipaddress,json,shutil,sys
from pathlib import Path

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def digest(w):return hashlib.sha256(w).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def write(p,x):
 with Path(p).open('x',encoding='utf8')as f:json.dump(x,f,indent=2);f.write('\n')

def v2ies(w,start,end):
 rows=[];at=start
 while at<end:
  assert len(rows)<4096 and at+4<=end,'IE header exceeds declared message'
  typ=w[at];size=int.from_bytes(w[at+1:at+3],'big');finish=at+4+size;assert finish<=end,'IE value exceeds declared message'
  row={'type':typ,'length':size,'instance':w[at+3]&15,'CR_or_spare_nibble':w[at+3]>>4,'header_PDU_span':[at,at+4],'value_PDU_span':[at+4,finish],'value_SHA256':digest(w[at+4:finish])}
  if typ==2 and size>=1:row['Cause_Code']=w[at+4];row['Cause_Code_PDU_span']=[at+4,at+5]
  if typ==3 and size==1:row['Recovery_Counter']=w[at+4];row['Recovery_Counter_PDU_span']=[at+4,at+5]
  if typ==73 and size==1:row['EPS_Bearer_ID']=w[at+4]&15;row['EBI_octet_PDU_span']=[at+4,at+5]
  if typ==87 and size>=5:
   flags=w[at+4];v4=bool(flags&128);v6=bool(flags&64);expected=5+4*v4+16*v6;row['F_TEID_expected_value_bytes']=expected;row['F_TEID_declared_extent_matches']=size==expected
   if size==expected:
    row['F_TEID_flags']=flags;row['F_TEID_interface']=flags&63;row['F_TEID_TEID']=int.from_bytes(w[at+5:at+9],'big');row['F_TEID_flags_PDU_span']=[at+4,at+5];row['F_TEID_TEID_PDU_span']=[at+5,at+9]
  if typ==93:
   try:row['Bearer_Context_IEs']=v2ies(w,at+4,finish)
   except AssertionError as e:row['Bearer_Context_refusal']=str(e)
  rows.append(row);at=finish
 assert at==end;return rows

def gtp(w):
 assert len(w)>=4;version=w[0]>>5;result=[];at=0
 if version==1:
  assert w[0]&16,'GTP prime is not GTPv1 PT1';assert len(w)>=8;size=int.from_bytes(w[2:4],'big');end=8+size;assert end==len(w),'v1 declared extent differs UDP payload';flags=w[0];h={'version':1,'flags':flags,'type':w[1],'message_length':size,'message_PDU_span':[0,end],'header_PDU_span':[0,8],'TEID':int.from_bytes(w[4:8],'big'),'TEID_PDU_span':[4,8],'field_spans':{'flags':[0,1],'type':[1,2],'message_length':[2,4]}};at=8
  if flags&7:
   assert at+4<=end;h.update(sequence=int.from_bytes(w[8:10],'big'),N_PDU=w[10],next_extension_type=w[11],sequence_PDU_span=[8,10],N_PDU_PDU_span=[10,11],next_extension_type_PDU_span=[11,12]);at=12;h['header_PDU_span']=[0,12]
  if flags&4:
   ext=[];kind=h['next_extension_type']
   while kind:
    assert len(ext)<64 and at+4<=end;size=w[at]*4;assert size>=4 and at+size<=end;ext.append({'type':kind,'length_words':w[at],'PDU_span':[at,at+size],'next_type':w[at+size-1],'content_SHA256':digest(w[at+1:at+size-1])});kind=w[at+size-1];at+=size
   h['extensions']=ext
  h['opaque_body_PDU_span']=[at,end];h['opaque_body_SHA256']=digest(w[at:end]);return[h]
 assert version==2;iterations=0
 while at<len(w):
  iterations+=1;assert iterations<=2 and at+8<=len(w),'bounded piggyback count/header';flags=w[at];assert flags>>5==2,'piggyback version differs';size=int.from_bytes(w[at+2:at+4],'big');end=at+4+size;teid=bool(flags&8);header=12 if teid else 8;assert size>=header-4 and end<=len(w),'v2 declared extent outside UDP payload';seq=at+(8 if teid else 4);h={'version':2,'flags':flags,'type':w[at+1],'message_length':size,'piggyback':bool(flags&16),'TEID_present':teid,'message_PDU_span':[at,end],'header_PDU_span':[at,at+header],'sequence':int.from_bytes(w[seq:seq+3],'big'),'sequence_PDU_span':[seq,seq+3],'priority_or_spare_octet':w[seq+3],'priority_or_spare_PDU_span':[seq+3,seq+4],'field_spans':{'flags':[at,at+1],'type':[at+1,at+2],'message_length':[at+2,at+4]}}
  if teid:h.update(TEID=int.from_bytes(w[at+4:at+8],'big'),TEID_PDU_span=[at+4,at+8])
  h['IEs']=v2ies(w,at+header,end);result.append(h);at=end
  if not flags&16:assert at==len(w),'unindicated trailing bytes after v2 message';break
  assert at<len(w),'P flag without piggyback message'
 assert at==len(w);return result

def main():
 ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--phase',required=True,type=Path);ap.add_argument('--out',required=True,type=Path);a=ap.parse_args();assert not a.out.exists();assert shutil.disk_usage('C:/').free>20*2**30;a.out.mkdir();base=a.phase/'OSPFv3-independent-source37-a82-01';inv=base/'inputs01/inputs.json';doc=read(inv);sys.path.insert(0,str(base));from scan_source37 import Census
 reader=Path(doc['capture_reader']);assert sha(reader)==doc['capture_reader_SHA256'];sys.path.insert(0,str(reader.parent));spec=importlib.util.spec_from_file_location('closed_original_capture_reader',reader);reader_module=importlib.util.module_from_spec(spec);spec.loader.exec_module(reader_module);binding={str(p):sha(p)for p in(Path(__file__),inv,reader,base/'scan_source37.py',base/'ospf_source_codec.py')};summaries=[];sample_features=set();sample_count=0
 class Visit(Census):
  def __init__(self,*args):super().__init__(*args);self.udp=[];self.rsvp=[]
  def upper(self,proto,at,end,path,depth):
   if proto==46:
    self.rsvp.append({'physical_range':[at,end],'captured_bytes':end-at,'version':self.w[at]>>4 if at<end else None,'message_type':self.w[at+1]if at+2<=end else None,'declared_length':self.u(at+6)if at+8<=end else None,'carrier':path});return
   if proto==17 and at+8<=end:
    length=self.u(at+4);self.udp.append({'physical_range':[at,end],'declared_UDP_end':at+length,'UDP_length':length,'source_port':self.u(at),'destination_port':self.u(at+2),'parent_complete':length>=8 and at+length<=end,'payload_range':[at+8,min(at+length,end)]if length>=8 else[at+8,at+8],'carrier':path})
   return super().upper(proto,at,end,path,depth)
 for item in doc['inputs']:
  cap=Path(item['capture']);assert sha(cap)==item['capture_SHA256'];binding[str(cap)]=item['capture_SHA256'];folder=a.out/('input-%02d'%item['input']);folder.mkdir();counts=collections.Counter();types=collections.Counter();versions=collections.Counter();ie_types=collections.Counter();ports=collections.Counter();refusals=collections.Counter();hints=collections.Counter();selected=[];source_refusals=[];capture_count=0
  for frame,r in enumerate(reader_module.capture_records(cap),1):
   capture_count+=1;visit=Visit(r['wire'],r,frame,counts)
   try:visit.run()
   except(AssertionError,IndexError,ValueError):counts['unselected_carrier_parse_refusals']+=1
   for rv in visit.rsvp:counts['declared_RSVP_IP46']+=1;hints['RSVP_message_%s'%rv['message_type']]+=1
   for udp in visit.udp:
    counts['bounded_UDP_headers_examined']+=1;start,end=udp['payload_range'];w=r['wire'][start:end];porthint=bool({udp['source_port'],udp['destination_port']}&{2123,2152,3386})
    if porthint:hints['GTP_port_hint_version_%s'%(w[0]>>5 if w else None)]+=1
    if 623 in(udp['source_port'],udp['destination_port'])and len(w)>=4 and w[0]==6 and w[1]==0:counts['RMCP_header_shape_on_observed_UDP623']+=1;hints['RMCP_class_%d'%w[3]]+=1
    if not udp['parent_complete']:
     if porthint:counts['GTP_hint_with_incomplete_UDP_parent']+=1
     continue
    if len(w)<8 or w[0]>>5 not in(1,2)or(w[0]>>5==1 and not w[0]&16):continue
    # Structural source selection is independent of the outer port. Length
    # equality/explicit piggyback and IE boundaries must all be witnessed.
    declared=(8 if w[0]>>5==1 else 4)+int.from_bytes(w[2:4],'big')
    if declared>len(w)or(w[0]>>5==1 and declared!=len(w)):continue
    try:messages=gtp(w)
    except(AssertionError,IndexError,ValueError)as ex:
     if porthint:refusals[str(ex)]+=1;source_refusals.append({'frame':frame,'payload_range':[start,end],'payload_SHA256':digest(w),'reason':str(ex),'outer_port_hint_not_confirmation':True})
     continue
    row={'input':item['input'],'capture_SHA256':item['capture_SHA256'],'frame':frame,'domain':{'section':r['section'],'interface':r['interface'],'link':r['link']},'time_ns':r['time_ns'],'original_frame_SHA256':digest(r['wire']),'captured_length':r['captured_length'],'original_length':r['original_length'],'payload_range':[start,end],'payload_SHA256':digest(w),'UDP_carrier':udp,'messages':messages,'classification_scope':'version/extent/typed headers observed; unique protocol admission and tunnel/session state not asserted from outer ports','port_is_confirmation':False};selected.append(row);counts['structurally_bounded_GTP_candidate_datagrams']+=1;ports['%d->%d'%(udp['source_port'],udp['destination_port'])]+=1
    for msg in messages:
     versions[str(msg['version'])]+=1;types['v%d/type%d'%(msg['version'],msg['type'])]+=1
     for ie in msg.get('IEs',[]):ie_types[str(ie['type'])]+=1
     feature=(msg['version'],msg['type'])
     if sample_count<16 and feature not in sample_features:
      sample_features.add(feature);sample_count+=1;fp=a.out/('representative-%02d-input%02d-frame%d.bin'%(sample_count,item['input'],frame));fp.write_bytes(r['wire']);row.setdefault('representative_original_frames',[]).append({'path':str(fp),'SHA256':sha(fp),'feature':feature})
  assert capture_count==item['physical_frames']and sha(cap)==item['capture_SHA256'];write(folder/'source-facts.json',selected);write(folder/'source-refusals.json',source_refusals);s={'input':item['input'],'capture':str(cap),'capture_SHA256':item['capture_SHA256'],'physical_frames':capture_count,'counts':dict(counts),'header_versions':dict(versions),'message_types':dict(types),'top_IE_types':dict(ie_types),'observed_port_pairs':dict(ports),'hints_not_confirmations':dict(hints),'declared_source_refusals':dict(refusals),'source_facts_SHA256':sha(folder/'source-facts.json'),'source_refusals_SHA256':sha(folder/'source-refusals.json')};write(folder/'summary.json',s);summaries.append(s);print(json.dumps({'input':item['input'],'GTP_source_datagrams':len(selected),'versions':dict(versions),'RSVP_IP46':counts['declared_RSVP_IP46'],'RMCP_shape':counts['RMCP_header_shape_on_observed_UDP623']}),flush=True)
 assert binding=={p:sha(p)for p in binding};write(a.out/'summary.json',{'scope':__doc__,'captures':summaries,'inputs':binding,'inputs_unchanged':True,'input_count':37,'representative_frames':sample_count,'no_Go_repo_native_or_CLI_read':True,'source_shape_not_unique_protocol_identity_or_port_confirmation':True,'no_authentication_session_tunnel_or_inner_payload_claim':True,'primary_sources':['https://www.etsi.org/deliver/etsi_ts/129200_129299/129274/18.07.00_60/ts_129274v180700p.pdf','https://www.etsi.org/deliver/etsi_ts/129200_129299/129281/08.05.00_60/ts_129281v080500p.pdf']});print(json.dumps({'summary':str(a.out/'summary.json'),'SHA256':sha(a.out/'summary.json'),'source_datagrams':sum(s['counts'].get('structurally_bounded_GTP_candidate_datagrams',0)for s in summaries),'representatives':sample_count}),flush=True)
if __name__=='__main__':main()
