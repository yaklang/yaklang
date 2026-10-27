"""Independently certify selected initial SSH PDUs and role/offer evidence.
Scope: owned IPv4 loopback TCP, original capture or declared replay controls,
initial uncompressed no-MAC packets through each direction's first NEWKEYS.
Encrypted rekey/authentication/application contents, key mathematics, signature
verification and native peer identity are not certified. DecodeAs server ports
are declared reference hints, not native role evidence.
"""
import argparse,base64,collections,hashlib,json,struct
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in['capture','manifest','events','oracle','profile','out']:ap.add_argument('--'+n,required=True,type=str if n=='profile'else Path)
ap.add_argument('--before',type=Path)
a=ap.parse_args();checks=0
for p in vars(a).values():
 if isinstance(p,Path):assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
def check(ok,why):
 global checks
 checks+=1;assert ok,why
def eq(x,y,why):check(x==y,(why,x,y))
def sha(w):return hashlib.sha256(w).hexdigest()
def load(p):return json.loads(p.read_text(encoding='utf-8-sig'))
def native(p):return[json.loads(l)for l in p.open(encoding='utf-8-sig')]
m=load(a.manifest);ops={o['server_port']:o for o in m['operations']};eq(len(ops),5,'five actual SSH operations');eq(m['errors'],[],'endpoint/proxy operations succeeded')
for o in ops.values():
 eq(o['bytes'],131072,'actual byte-exact application receipt');check(o['client_selected'],'independent OpenSSH debug selection')
 for key,value in o['server_selected'].items():eq(value,o['client_selected'][key],'independent endpoints agree on actual selected algorithms')
w=a.capture.read_bytes();check(w[:4]in[b'\xd4\xc3\xb2\xa1',b'\x4d\x3c\xb2\xa1'],'selected classic little-endian PCAP');link=struct.unpack_from('<I',w,20)[0];check(link in[0,1],'selected loopback/Ethernet link');at=24;frames={};frame_hashes={};segments=collections.defaultdict(list);syn=collections.defaultdict(list)
while at<len(w):
 check(at+16<=len(w),'complete PCAP record');_,_,n,wire=struct.unpack_from('<IIII',w,at);at+=16;check(0<n<=wire and at+n<=len(w),'captured/wire bounds');b=w[at:at+n];at+=n;number=len(frames)+1;frames[number]=None;frame_hashes[number]=sha(b);ip=4 if link==0 else 14
 if len(b)<ip+20 or b[ip]>>4!=4 or b[ip+9]!=6:continue
 ihl=(b[ip]&15)*4;t=ip+ihl
 if ihl<20 or len(b)<t+20:continue
 sport,dport,seq=struct.unpack_from('!HHI',b,t);port=sport if sport in ops else dport
 if port not in ops:continue
 o=ops[port];eq({sport,dport},{port,o['captured_client_port']},'literal owned connection endpoints');direction=1 if sport==port else 0;key=(port,direction);size=(b[t+12]>>4)*4;check(size>=20 and t+size<=len(b),'complete TCP header');end=ip+int.from_bytes(b[ip+2:ip+4],'big');f={'key':key,'seq':seq,'flags':b[t+13],'complete':end<=len(b),'payload':b[t+size:min(end,len(b))]};frames[number]=f
 if f['flags']&2:syn[key].append(seq)
 if f['payload']and f['complete']:segments[key].append((number,f))
records={};bases={};expected=collections.Counter();offered={};names=['Kex Algorithms','Host Key Algorithms','Offered Encryption C2S','Offered Encryption S2C','Offered MAC C2S','Offered MAC S2C','Offered Compression C2S','Offered Compression S2C','Languages C2S','Languages S2C']
def string(b,at):
 check(at+4<=len(b),'SSH string length present');n=int.from_bytes(b[at:at+4],'big');at+=4;check(n<=len(b)-at,'SSH string contained in message');return b[at:at+n],at+n
for key,ss in segments.items():
 base=((syn[key][0]+1)&0xffffffff)if syn[key]else ss[0][1]['seq'];bases[key]=base;octets={}
 for number,f in ss:
  start=(f['seq']-base)&0xffffffff
  if start>=65536:continue
  for i,value in enumerate(f['payload'][:65536-start],start):
   check(i not in octets or octets[i]==value,'identical TCP overlap');octets[i]=value
 prefix=bytearray();cursor=0
 while cursor in octets:prefix.append(octets[cursor]);cursor+=1
 prefix=bytes(prefix);end=prefix.find(b'\n');check(0<=end<255,'complete initial identification');raw=prefix[:end+1];check(raw.startswith((b'SSH-2.0-',b'SSH-1.99-')),'observed supported identification');r={'key':key,'start':0,'end':len(raw),'raw':raw,'type':'identification','fields':{'Identification':raw.rstrip(b'\r\n').decode('ascii')}};records[(key,sha(raw))]=r;expected[(key,sha(raw))]+=1;offset=len(raw)
 while offset+4<=len(prefix):
  size=4+int.from_bytes(prefix[offset:offset+4],'big')
  if offset+size>len(prefix):break
  check(16<=size<=35000 and size%8==0,'initial SSH packet boundary');raw=prefix[offset:offset+size];pad=raw[4];check(4<=pad and pad+6<=size,'complete padded payload');body=raw[5:len(raw)-pad];code=body[0];fields={'Message Number':code};kind=str(code)
  if code==20:
   check(len(body)>=17,'full KEXINIT cookie');pos=17
   for name in names:
    value,pos=string(body,pos);fields[name]=value.decode('ascii')
   eq(len(body)-pos,5,'KEXINIT flags and reserved fields complete');fields['First Kex Packet Follows']=body[pos];eq(body[pos+1:],b'\0'*4,'selected reserved field');offered[key]=fields
  if code==21:eq(len(body),1,'NEWKEYS contains only message code')
  r={'key':key,'start':offset,'end':offset+size,'raw':raw,'type':kind,'fields':fields};records[(key,sha(raw))]=r;expected[(key,sha(raw))]+=1;offset+=size
  if code==21:break
observed=collections.Counter();flows={};selected_count=collections.Counter();seen_offers=set();known_roles={port:bool(syn[(port,0)]and syn[(port,1)])for port in ops};events=native(a.events);refmatched=0;refmissing=[];source_checks=0
soft=m.get('declared_nonconforming_server_identification');warned={};warning_pdus=0;opaque_warning_pdus=0
endpoint_orientation={}
def leaves(o,key):
 if isinstance(o,dict):
  for k,v in o.items():
   if k==key:yield from(v if isinstance(v,list)else[v])
   elif isinstance(v,(dict,list)):yield from leaves(v,key)
 elif isinstance(o,list):
  for v in o:yield from leaves(v,key)
ref=collections.defaultdict(list)
for row in load(a.oracle):
 l=row['_source']['layers'];tcp=l.get('tcp')or{};sport=int(tcp['tcp.srcport']);dport=int(tcp['tcp.dstport']);port=sport if sport in ops else dport
 if port not in ops:continue
 key=(port,1 if sport==port else 0);views=l.get('ssh')or[];views=views if isinstance(views,list)else[views];raws=l.get('ssh_raw')or[];raws=[raws]if raws and isinstance(raws[0],str)else raws
 number=int(l['frame']['frame.number']);check(number in frames and frames[number]and frames[number]['key']==key,'reference physical tuple/direction bound to capture');eq(sha(bytes.fromhex(l['frame_raw'][0])),frame_hashes[number],'reference literal physical frame bytes')
 for view,raw in zip(views,raws):
  b=bytes.fromhex(raw[0]);pos=0
  while pos<len(b):
   if b[pos:].startswith(b'SSH-'):
    end=b.find(b'\n',pos);size=end-pos+1 if end>=pos else 0
   elif len(b)-pos>=4:size=4+int.from_bytes(b[pos:pos+4],'big')
   else:break
   if size<=0 or pos+size>len(b):break
   part=b[pos:pos+size];lookup=(key,sha(part))
   if lookup in records:ref[lookup].append((int(l['frame']['frame.number']),view))
   pos+=size
selected_keys=['Kex Algorithm','Host Key Algorithm','Encryption C2S','Encryption S2C','MAC C2S','MAC S2C','Compression C2S','Compression S2C']
ws_names={'Identification':'ssh.protocol','Kex Algorithms':'ssh.kex_algorithms','Host Key Algorithms':'ssh.server_host_key_algorithms','Offered Encryption C2S':'ssh.encryption_algorithms_client_to_server','Offered Encryption S2C':'ssh.encryption_algorithms_server_to_client','Offered MAC C2S':'ssh.mac_algorithms_client_to_server','Offered MAC S2C':'ssh.mac_algorithms_server_to_client','Offered Compression C2S':'ssh.compression_algorithms_client_to_server','Offered Compression S2C':'ssh.compression_algorithms_server_to_client','Languages C2S':'ssh.languages_client_to_server','Languages S2C':'ssh.languages_server_to_client','Message Number':'ssh.message_code'}
for e in events:
 s=e.get('session')or{}
 if e['protocol']!='ssh':continue
 sport=int(e['source'].rsplit(':',1)[1]);dport=int(e['destination'].rsplit(':',1)[1]);port=sport if sport in ops else dport;check(port in ops,'native SSH endpoint belongs to the actual capture');direction=1 if sport==port else 0;check(e['direction']in[0,1],'native logical endpoint index is binary');endpoint_orientation.setdefault(port,e['direction']^direction);eq(e['direction']^direction,endpoint_orientation[port],'logical endpoint indexing is stable across both actual directions')
 if soft and s.get('Identification')==soft:
  check(direction==1,'declared nonconformant identifier belongs to the actual server')
  warned[port]="endpoint %d: softwareversion uses '-' (RFC4253 4.2)"%e['direction']
 if port in warned:
  eq(s.get('Identification Conformance'),'nonconformant','nonconformant literal identifier never becomes a conformant transport')
  eq(s.get('Identification Warning'),warned[port],'warning persists after the observed identification, including opaque transport')
  vals=(e.get('field_values')or{}).get('values',[])
  for name,value in [('ssh.identification.conformance','nonconformant'),('ssh.identification.warning',warned[port])]:
   bound=[v for v in vals if v['name']==name];eq(len(bound),1,'one queryable identification evidence field');eq(bound[0]['value'],value,'literal context warning projection');eq(bound[0]['origin'],'context','warning is context, not a fabricated wire field');check('pdu_range'not in bound[0],'warning has no invented byte range')
  warning_pdus+=1;opaque_warning_pdus+='Packet Name'not in s
 else:check('Identification Conformance'not in s and'Identification Warning'not in s,'a compatibility warning cannot precede its observed nonconformant identity or appear on a conformant connection')
 if'Packet Name'not in s:continue
 sport=int(e['source'].rsplit(':',1)[1]);dport=int(e['destination'].rsplit(':',1)[1]);port=sport if sport in ops else dport;check(port in ops,'native captured endpoint');key=(port,1 if sport==port else 0);raw=base64.b64decode(e['raw']or'');lookup=(key,sha(raw));check(lookup in records,('native PDU is an exact independently reassembled initial message',e['id'],s['Packet Name']));r=records[lookup];observed[lookup]+=1;eq(e['source_bytes']['SHA256'],sha(raw),'owned literal PDU digest');check(e['flow'],'native connection retained');flows.setdefault(port,e['flow']);eq(flows[port],e['flow'],'one observed connection per actual endpoint pair');cursor=r['start'];intervals=[]
 for pr in e['source_bytes']['PacketRefs']:
  f=frames[pr['Number']];check(f and f['complete']and f['key']==key,'each contributing physical packet is complete and belongs to this direction');start=(f['seq']-bases[key])&0xffffffff;end=start+len(f['payload']);check(start<r['end']and end>r['start'],'physical contributor intersects logical message');intervals.append((start,end));source_checks+=1
 for start,end in sorted(intervals):
  if start<=cursor:cursor=max(cursor,end)
 check(cursor>=r['end'],'physical contributors cover every logical PDU byte')
 for name,value in r['fields'].items():eq(s.get(name),value,('literal independent SSH field',e['id'],name))
 check(e['status']in['decoded','deferred']and not e.get('error'),'complete selected initial message is decoded without a refusal')
 if r['type']=='20':seen_offers.add(key)
 if not known_roles[port]:
  for name in selected_keys:eq(name in s,False,('missing role cannot manufacture a selected algorithm',e['id'],name))
  check('Initial Packet Fields'not in s and'Host Key Fingerprint'not in s,'method-specific interpretation is not invented from an unknown client preference')
  aliases=(e.get('field_values')or{}).get('values',[]);eq([v for v in aliases if v['name']=='ssh.kex.selected'],[],'typed selected-algorithm field is also absent')
  if r['type']!='identification':eq(s.get('Negotiation Context'),'client/server role not observed','explicit missing role context');eq([v['value']for v in aliases if v['name']=='ssh.negotiation.context'],['client/server role not observed'],'typed evidence limitation remains queryable')
 else:
  check(s.get('Negotiation Context')!='client/server role not observed','observed TCP role evidence must not be discarded')
  if any(name in s for name in selected_keys):check(all((port,d)in seen_offers for d in[0,1]),'both complete offers precede a selected algorithm')
  for name in ops[port]['client_selected']:
   if name in s:eq(s[name],ops[port]['client_selected'][name],('actual endpoint/native selected algorithm',e['id'],name));selected_count[port]+=1
 for name in['Signature Verified','Peer Identity Verified']:
  if name in s:eq(s[name],False,'native byte parsing does not certify peer identity')
 if'Host Key SHA256'in s:eq(s['Host Key SHA256'],m['host_key_sha256'],'actual server key bytes fingerprint')
 if lookup in ref:
  refmatched+=1
  for name,value in r['fields'].items():
   if name not in ws_names:continue
   vals=[v for _,view in ref[lookup]for v in leaves(view,ws_names[name])];vals=[int(v)for v in vals]if isinstance(value,int)else vals;check(value in vals,('literal reference field',e['id'],name,value,vals))
 else:refmissing.append({'PDU':e['id'],'type':r['type'],'source_frames':[v['Number']for v in e['source_bytes']['PacketRefs']]})
eq(observed,expected,'all complete independent initial PDUs exactly once; none guessed or replayed');eq(len(set(flows.values())),5,'no cross-connection merge')
for port in ops:
 if known_roles[port]and all((port,d)in offered for d in[0,1]):check(selected_count[port]>0,'complete role/offer evidence yields actual selected values')
wrong_before=0
if a.before:
 for e in native(a.before):
  s=e.get('session')or{}
  if e['protocol']!='ssh':continue
  sport=int(e['source'].rsplit(':',1)[1]);dport=int(e['destination'].rsplit(':',1)[1]);port=sport if sport in ops else dport
  if port in ops and'Kex Algorithm'in s:wrong_before+=any(name in s and s[name]!=value for name,value in ops[port]['client_selected'].items())
if soft:eq(set(warned),set(ops),'all five actual nonconformant server identifiers observed')
out={'scope':__doc__,'profile':a.profile,'checks':checks,'physical_frames':len(frames),'native_connections':len(flows),'complete_initial_PDUs':sum(expected.values()),'physical_source_checks':source_checks,'known_TCP_roles':known_roles,'PDUs_compared_to_reference':refmatched,'reference_unavailable_initial_PDUs':refmissing,'baseline_PDUs_with_wrong_selected_algorithms':wrong_before,'selected_algorithm_comparisons':sum(selected_count.values()),'declared_nonconforming_server_identification':soft,'PDUs_with_persistent_identification_warning':warning_pdus,'opaque_PDUs_with_persistent_identification_warning':opaque_warning_pdus,'capture_sha256':sha(w),'manifest_sha256':sha(a.manifest.read_bytes()),'events_sha256':sha(a.events.read_bytes()),'oracle_sha256':sha(a.oracle.read_bytes()),'before_events_sha256':sha(a.before.read_bytes())if a.before else None};a.out.parent.mkdir(parents=True,exist_ok=True);a.out.write_text(json.dumps(out,indent=2)+'\n',encoding='utf8');print(json.dumps(out))
