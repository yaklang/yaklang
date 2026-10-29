"""Original-byte small public GTP source census and isolated reference only.
Source facts use independently bounded parent lengths. No native output read.
"""
import argparse,collections,hashlib,importlib.util,json,os,shutil,struct,subprocess,sys,time,xml.etree.ElementTree as ET
from pathlib import Path
def sha(p):
 h=hashlib.sha256()
 with Path(p).open('rb')as f:
  for x in iter(lambda:f.read(1<<20),b''):h.update(x)
 return h.hexdigest()
def dg(w):return hashlib.sha256(w).hexdigest()
def put(p,x):
 with Path(p).open('x',encoding='utf8')as f:json.dump(x,f,indent=2);f.write('\n')
def mod(name,p):
 sp=importlib.util.spec_from_file_location(name,p);m=importlib.util.module_from_spec(sp);sp.loader.exec_module(m);return m
def block(t,data):
 data+=b'\0'*((-len(data))%4);n=12+len(data);return struct.pack('<II',t,n)+data+struct.pack('<I',n)
def reference(ts,cap,out,bindings):
 config=out/'config';config.mkdir();env=os.environ.copy();env['APPDATA']=str(config);env['WIRESHARK_CONFIG_DIR']=str(config);env.pop('SSLKEYLOGFILE',None)
 cmd=[str(ts),'-n','-2','-r',str(cap),'-T','pdml'];t=time.time()
 with(out/'reference.pdml').open('xb')as f,(out/'stderr.txt').open('xb')as e:r=subprocess.run(cmd,env=env,stdout=f,stderr=e,timeout=45)
 assert r.returncode==0;tree=ET.parse(out/'reference.pdml');rows=[]
 # Header and IE framing only: no subscriber/authentication payload exported here.
 names={'frame.number','frame.interface_id','frame.encap_type','frame.len','frame.cap_len','ip.hdr_len','ip.len','ip.flags.mf','ip.frag_offset','udp.srcport','udp.dstport','udp.length','gtp.flags','gtp.flags.version','gtp.message','gtp.length','gtpv2.flags','gtpv2.version','gtpv2.p','gtpv2.t','gtpv2.message_type','gtpv2.msg_length','gtpv2.teid','gtpv2.seq','gtpv2.spare','gtpv2.ie_type','gtpv2.ie_len','gtpv2.cr','gtpv2.instance'}
 for packet in tree.getroot().findall('packet'):
  fields=[];expert=[];protocols=[]
  for pr in packet.findall('proto'):
   owner=pr.attrib.get('name');protocols.append(owner)
   for f in pr.iter('field'):
    n=f.attrib.get('name','')
    if n in names:fields.append({'name':n,'owner':owner,**{k:f.attrib[k]for k in('show','pos','size','value')if k in f.attrib}})
    if n=='_ws.expert.message':expert.append({'owner':owner,'message':f.attrib.get('show'),'pos':f.attrib.get('pos'),'size':f.attrib.get('size')})
  n=int(next(f['show']for f in fields if f['name']=='frame.number'));source=bindings[n-1];rows.append({'reference_frame':n,'original_frame':source['frame'],'domain':source['domain'],'source_raw_SHA256':source['original_frame_SHA256'],'protocols':protocols,'header_fields':fields,'experts':expert})
 assert len(rows)==len(bindings);put(out/'header-comparison.json',rows);put(out/'receipt.json',{'command':cmd,'binary':str(ts),'binary_SHA256':sha(ts),'input':str(cap),'input_SHA256':sha(cap),'returncode':r.returncode,'seconds':round(time.time()-t,3),'configuration_isolated':True,'no_decode_as':True,'records':len(rows),'PDML_SHA256':sha(out/'reference.pdml'),'comparison_SHA256':sha(out/'header-comparison.json')});return rows

def main():
 ap=argparse.ArgumentParser();ap.add_argument('--phase',type=Path,required=True);ap.add_argument('--tshark',type=Path,required=True);ap.add_argument('--out',type=Path,required=True);a=ap.parse_args();assert not a.out.exists();assert shutil.disk_usage('C:/').free>20*2**30;a.out.mkdir();base=a.phase/'OSPFv3-independent-source37-a82-01';doc=json.loads((base/'inputs01/inputs.json').read_text('utf8'));rp=Path(doc['capture_reader']);sys.path.insert(0,str(rp.parent));reader=mod('reader',rp);sys.path.insert(0,str(base));from scan_source37 import Census
 gt=mod('source_gt',a.phase/'census_GTP_family_source43.py');item=next(x for x in doc['inputs']if x['input']==35);original=Path(item['capture']);official=a.phase/'GTP-official-download43-01/project-upload.pcap';frozen={str(p):sha(p)for p in[Path(__file__),a.tshark,rp,base/'scan_source37.py',a.phase/'census_GTP_family_source43.py',original,official]};chosen={36549,36550,36554,36612,41344,42968};records=[];reps=json.loads((a.phase/'GTP-parent-refusals43-02/representatives.json').read_text('utf8'))
 for n,r in enumerate(reader.capture_records(original),1):
  if n in chosen:
   source=next(x for x in reps if x['frame']==n);assert dg(r['wire'])==source['original_frame_SHA256'];assert source['domain']=={k:r[k]for k in('section','interface','link')};records.append(r)
  if n>=max(chosen):break
 assert len(records)==6 and all(r['section']==0 and r['interface']==225 and r['link']==1 for r in records)
 # Derived reference subset preserves raw bytes, cap/original lengths, timestamps,
 # section/interface IDs and link. IDB options not required for these stateless
 # Ethernet/IPv4/UDP/GTP headers are intentionally not claimed to be preserved.
 extract=a.out/'original-six-reference-subset.pcapng'
 with extract.open('xb')as f:
  f.write(block(0x0a0d0d0a,struct.pack('<IHHq',0x1a2b3c4d,1,0,-1)))
  for i in range(226):f.write(block(1,struct.pack('<HHI',1,0,65535)+struct.pack('<HH',9,1)+b'\x09\0\0\0'+struct.pack('<HH',0,0)))
  for r in records:
   t=r['time_ns'];f.write(block(6,struct.pack('<IIIII',r['interface'],t>>32,t&0xffffffff,r['captured_length'],r['original_length'])+r['wire']))
 re=list(reader.capture_records(extract));assert len(re)==6
 for r,s in zip(records,re):
  for k in('section','interface','link','time_ns','captured_length','original_length','wire'):assert r[k]==s[k],k
 put(a.out/'extraction-source-binding.json',{'original_capture':str(original),'original_capture_SHA256':sha(original),'original_frames':sorted(chosen),'derived_capture':str(extract),'derived_capture_SHA256':sha(extract),'exact_raw_caplen_original_domain_timestamp_verified':True,'nonselected_IDB_options_not_preserved_or_claimed':True,'reference_slots':[{'reference_frame':i+1,'original_frame':x['frame'],'raw_SHA256':x['original_frame_SHA256']}for i,x in enumerate(reps)]})
 bad=a.out/'malformed-parent-reference';bad.mkdir();badref=reference(a.tshark,extract,bad,reps)
 facts=[];counts=collections.Counter()
 class V(Census):
  def __init__(self,*args):super().__init__(*args);self.udps=[]
  def upper(self,proto,at,end,path,depth):
   if proto==17:self.udps.append((at,end,path))
   return super().upper(proto,at,end,path,depth)
 for n,r in enumerate(reader.capture_records(official),1):
  v=V(r['wire'],r,n,counts);v.run();assert len(v.udps)==1;at,end,path=v.udps[0];w=r['wire'];length=int.from_bytes(w[at+4:at+6],'big');assert length>=8 and at+length<=end;pay=w[at+8:at+length];msgs=gt.gtp(pay);parents=[x for x in path if x['kind']in('IPv4','IPv6')];assert all(x['declared_end']==x['available_end']for x in parents)
  fact={'frame':n,'domain':{k:r[k]for k in('section','interface','link')},'original_frame_SHA256':dg(w),'captured_length':r['captured_length'],'original_length':r['original_length'],'carrier':path,'UDP_header_physical_span':[at,at+8],'UDP_length':length,'UDP_length_physical_span':[at+4,at+6],'UDP_source_port':int.from_bytes(w[at:at+2],'big'),'UDP_destination_port':int.from_bytes(w[at+2:at+4],'big'),'payload_physical_span':[at+8,at+length],'payload_SHA256':dg(pay),'bounded_messages':msgs,'packet_tail_outside_declared_IP_bytes':len(w)-end,'ports_are_not_confirmation':True,'scope':'bounded wire header/TLV observations; no subscriber identity, authentication, bearer/session or tunnel state asserted'};facts.append(fact)
  for m in msgs:counts['v%d/type%d'%(m['version'],m['type'])]+=1;counts['messages']+=1
 put(a.out/'official-source-facts.json',facts);good=a.out/'official-reference';good.mkdir();goodref=reference(a.tshark,official,good,facts)
 comparisons=[];checks=0
 for row,ref in zip(facts,goodref):
  start=row['payload_physical_span'][0];msg=row['bounded_messages'][0];fields=ref['header_fields'];byname=collections.defaultdict(list)
  for f in fields:byname[f['name']].append(f)
  for name,value,span in[('udp.length',row['UDP_length'],row['UDP_length_physical_span']),('gtpv2.version',msg['version'],[start,start+1]),('gtpv2.message_type',msg['type'],[start+1,start+2]),('gtpv2.msg_length',msg['message_length'],[start+2,start+4]),('gtpv2.teid',msg['TEID'],[start+4,start+8]),('gtpv2.seq',msg['sequence'],[start+8,start+11])]:
   fs=byname[name];assert len(fs)==1,(row['frame'],name,len(fs));f=fs[0]
   actual=int(f['show'],0)if f['show'].startswith('0x')else int(f['show']);assert actual==value,(row['frame'],name,value,actual);assert[int(f['pos']),int(f['pos'])+int(f['size'])]==span,(row['frame'],name,span,f);checks+=1
  def flatten_ies(ies):
   for ie in ies:
    yield ie
    yield from flatten_ies(ie.get('Bearer_Context_IEs',[]))
  for ie in flatten_ies(msg['IEs']):
   b=ie['header_PDU_span'][0]+start
   for name,value,span in [('gtpv2.ie_type',ie['type'],[b,b+1]),('gtpv2.ie_len',ie['length'],[b+1,b+3]),('gtpv2.cr',ie['CR_or_spare_nibble'],[b+3,b+4]),('gtpv2.instance',ie['instance'],[b+3,b+4])]:
    matches=[f for f in byname[name]if int(f['pos'])==span[0]];assert len(matches)==1,(row['frame'],name,span,len(matches));f=matches[0];actual=int(f['show'],0)if f['show'].startswith('0x')else int(f['show']);assert actual==value and [int(f['pos']),int(f['pos'])+int(f['size'])]==span,(row['frame'],name,value,f);checks+=1
  comparisons.append({'frame':row['frame'],'source_payload_SHA256':row['payload_SHA256'],'message_type':msg['type'],'source_top_level_IEs':len(msg['IEs']),'reference_experts':ref['experts']})
 assert frozen=={p:sha(p)for p in frozen};put(a.out/'summary.json',{'scope':__doc__,'inputs':frozen,'inputs_unchanged':True,'original37_GTP_admitted':0,'original_malformed_parent_refusals':5535,'other_bounded_port_hints_not_GTP':3,'official_capture':str(official),'official_capture_SHA256':sha(official),'official_frames':len(facts),'official_messages':counts['messages'],'official_message_types':{k:v for k,v in counts.items()if k.startswith('v')},'header_scalar_and_span_reference_checks':checks,'reference_comparisons':comparisons,'original_parent_reference':[{'frame':x['original_frame'],'domain':x['domain'],'experts':x['experts'],'protocols':x['protocols']}for x in badref],'no_native_CLI_or_Go_read_or_execution':True,'no_traffic_authentication_or_session_claim':True,'artifacts':{str(p.relative_to(a.out)):sha(p)for p in a.out.rglob('*')if p.is_file()}});print(json.dumps({'summary':str(a.out/'summary.json'),'SHA256':sha(a.out/'summary.json'),'official_frames':len(facts),'header_checks':checks}))
if __name__=='__main__':main()
