"""Original public source parent extents, refusal ledger and isolated Wireshark.
No GTP protocol identity is admitted from UDP port values or failed parent bounds.
"""
import argparse,collections,hashlib,importlib.util,json,os,shutil,subprocess,sys,time,xml.etree.ElementTree as ET
from pathlib import Path
def sha(p):
 h=hashlib.sha256()
 with Path(p).open('rb')as f:
  for b in iter(lambda:f.read(1<<20),b''):h.update(b)
 return h.hexdigest()
def put(p,d):
 with Path(p).open('x',encoding='utf8')as f:json.dump(d,f,indent=2);f.write('\n')
def main():
 ap=argparse.ArgumentParser();ap.add_argument('--phase',type=Path,required=True);ap.add_argument('--tshark',type=Path,required=True);ap.add_argument('--out',type=Path,required=True);a=ap.parse_args();assert not a.out.exists();assert shutil.disk_usage('C:/').free>20*2**30;a.out.mkdir()
 base=a.phase/'OSPFv3-independent-source37-a82-01';inv=base/'inputs01/inputs.json';doc=json.loads(inv.read_text('utf8'));sys.path.insert(0,str(base));from scan_source37 import Census
 reader=Path(doc['capture_reader']);assert sha(reader)==doc['capture_reader_SHA256'];sys.path.insert(0,str(reader.parent));sp=importlib.util.spec_from_file_location('capture_reader',reader);m=importlib.util.module_from_spec(sp);sp.loader.exec_module(m)
 item=next(i for i in doc['inputs']if i['input']==35);cap=Path(item['capture']);assert sha(cap)==item['capture_SHA256'];frozen={str(q):sha(q)for q in[Path(__file__),a.tshark,inv,reader,base/'scan_source37.py',cap]};c=collections.Counter();rows=[];reps=[];chosen={36549,36550,36554,36612,41344,42968}
 class V(Census):
  def __init__(self,*x):super().__init__(*x);self.udps=[]
  def upper(self,proto,at,end,path,depth):
   if proto==17 and at+8<=end and({self.u(at),self.u(at+2)}&{2123,2152,3386}):self.udps.append((at,end,path))
   return super().upper(proto,at,end,path,depth)
 for frame,r in enumerate(m.capture_records(cap),1):
  v=V(r['wire'],r,frame,c)
  try:v.run()
  except(AssertionError,IndexError,ValueError):pass
  for at,end,path in v.udps:
   w=r['wire'];length=int.from_bytes(w[at+4:at+6],'big');parent=next(x for x in reversed(path)if x['kind']in('IPv4','IPv6'));declaredIP=parent['declared_end'];availableIP=parent['available_end'];uEnd=at+length
   # Contradiction against full captured declared IP is independent of snaplen.
   verdict='UDP-declared-exceeds-complete-IP'if declaredIP==availableIP and uEnd>availableIP else('UDP-under-minimum'if length<8 else('UDP-needs-parent-bytes'if uEnd>availableIP else'UDP-bounded'))
   row={'frame':frame,'domain':{'section':r['section'],'interface':r['interface'],'link':r['link']},'captured_length':r['captured_length'],'original_length':r['original_length'],'original_frame_SHA256':hashlib.sha256(w).hexdigest(),'carrier':path,'IP_declared_end':declaredIP,'IP_available_end':availableIP,'UDP_physical_header_span':[at,at+8],'UDP_source_port':int.from_bytes(w[at:at+2],'big'),'UDP_destination_port':int.from_bytes(w[at+2:at+4],'big'),'UDP_declared_length':length,'UDP_length_physical_span':[at+4,at+6],'UDP_declared_end':uEnd,'IP_available_UDP_bytes':max(0,end-at),'captured_payload_physical_span':[at+8,end],'port_hint_not_protocol_confirmation':True,'verdict':verdict}
   if at+12<=end:
    flags=w[at+8];version=flags>>5;n=int.from_bytes(w[at+10:at+12],'big');row['unadmitted_child_header_observations']={'flags':flags,'version_bits':version,'type_octet':w[at+9],'length_octets':n,'flags_span':[at+8,at+9],'type_span':[at+9,at+10],'length_span':[at+10,at+12],'conditional_v1_extent':8+n if version==1 else None,'conditional_v2_extent':4+n if version==2 else None,'GTP_identity_asserted':False}
   rows.append(row);c['ledger_'+verdict]+=1
   if frame in chosen:reps.append(row)
 put(a.out/'diagnostic-counts.json',{'rows':len(rows),'counts':dict(c),'verdicts':dict(collections.Counter(x['verdict']for x in rows))});assert len(rows)==5535;assert all(x['verdict']=='UDP-declared-exceeds-complete-IP'for x in rows)
 put(a.out/'parent-refusals.json',rows);put(a.out/'representatives.json',reps)
 config=a.out/'isolated-config';config.mkdir();env=os.environ.copy();env['APPDATA']=str(config);env['WIRESHARK_CONFIG_DIR']=str(config);env.pop('SSLKEYLOGFILE',None)
 cmd=[str(a.tshark),'-n','-2','-r',str(cap),'-Y','frame.number in {'+','.join(map(str,sorted(chosen)))+'}','-T','pdml'];start=time.time()
 with(a.out/'reference.pdml').open('xb')as out,(a.out/'reference.stderr').open('xb')as err:r=subprocess.run(cmd,stdout=out,stderr=err,env=env,timeout=90)
 assert r.returncode==0
 tree=ET.parse(a.out/'reference.pdml');summary=[]
 allowed={'frame.number','frame.interface_id','frame.encap_type','frame.len','frame.cap_len','ip.hdr_len','ip.len','ip.flags.mf','ip.frag_offset','udp.srcport','udp.dstport','udp.length','_ws.col.Protocol'}
 for packet in tree.getroot().findall('packet'):
  fs=[];expert=[];protocols=[]
  for proto in packet.findall('proto'):
   owner=proto.attrib.get('name');protocols.append(owner)
   for f in proto.iter('field'):
    name=f.attrib.get('name','')
    if name in allowed or(name.startswith(('gtp.','gtpv2.'))and any(k in name.lower()for k in('version','flags','message','length','len'))and int(f.attrib.get('size','0'))<=4):fs.append({'name':name,'owner':owner,**{k:f.attrib[k]for k in('show','pos','size','value')if k in f.attrib}})
    if name=='_ws.expert.message':expert.append({'owner':owner,'message':f.attrib.get('show'),'pos':f.attrib.get('pos'),'size':f.attrib.get('size')})
  number=next(int(f['show'])for f in fs if f['name']=='frame.number');source=next(x for x in reps if x['frame']==number)
  byname={f['name']:f for f in fs}
  for name,val,span in[('frame.len',source['original_length'],None),('frame.cap_len',source['captured_length'],None),('frame.interface_id',source['domain']['interface'],None),('udp.length',source['UDP_declared_length'],source['UDP_length_physical_span'])]:
   assert name in byname and int(byname[name]['show'])==val,(number,name,byname.get(name),val)
   if span:assert[int(byname[name]['pos']),int(byname[name]['pos'])+int(byname[name]['size'])]==span
  summary.append({'frame':number,'source_SHA256':source['original_frame_SHA256'],'domain':source['domain'],'protocols':protocols,'header_fields':fs,'expert':expert,'parent_verdict':source['verdict']})
 assert len(summary)==len(chosen);put(a.out/'reference-comparison.json',summary);assert frozen=={q:sha(q)for q in frozen}
 report={'source_inputs':frozen,'input_hashes_unchanged':True,'physical_records':item['physical_frames'],'UDP_port_hint_observations':len(rows),'parent_verdict_counts':{k.removeprefix('ledger_'):v for k,v in c.items()if k.startswith('ledger_')},'admitted_GTP_messages':0,'all_refusals_complete_IP_parent_not_snaplen':True,'representative_count':len(reps),'reference_returncode':r.returncode,'reference_seconds':round(time.time()-start,3),'reference_command':cmd,'isolated_config':str(config),'no_decode_as':True,'no_native_or_candidate_read':True,'artifacts':{p.name:sha(p)for p in a.out.iterdir()if p.is_file()}}
 put(a.out/'summary.json',report);print(json.dumps({'summary':str(a.out/'summary.json'),'SHA256':sha(a.out/'summary.json'),'ledger':len(rows),'verdicts':report['parent_verdict_counts'],'representatives':[{k:x[k]for k in('frame','domain','UDP_declared_length','IP_available_UDP_bytes')}for x in reps]}))
if __name__=='__main__':main()
