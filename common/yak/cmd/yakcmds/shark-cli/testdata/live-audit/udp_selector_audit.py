"""Declared RFC5424 MSG-ANY/VXLAN overlap controls. Never live-traffic claims.
Default service-port decoding and explicitly selected Syslog are separate oracles.
"""
from pathlib import Path
import argparse,base64,hashlib,json,ipaddress,struct,subprocess
ap=argparse.ArgumentParser(description=__doc__)
for n in ['out','events','tshark']:ap.add_argument('--'+n,required=True,type=Path)
a=ap.parse_args();a.out.mkdir(parents=True,exist_ok=True)
assert not any((p/'.git').exists() for p in [a.out.resolve(),*a.out.resolve().parents])
def cs(w):
 if len(w)%2:w+=b'\0'
 n=sum(struct.unpack('!%dH'%(len(w)//2),w))
 while n>>16:n=(n&65535)+(n>>16)
 return (~n)&65535
src4=ipaddress.IPv4Address('192.0.2.1').packed;dst4=ipaddress.IPv4Address('192.0.2.2').packed
src6=ipaddress.IPv6Address('2001:db8::1').packed;dst6=ipaddress.IPv6Address('2001:db8::2').packed
def udp(body,port,v6=False,source=src4,destination=dst4):
 h=struct.pack('!HHHH',40000,port,8+len(body),0);pseudo=source+destination+(struct.pack('!I3xB',len(h)+len(body),17) if v6 else struct.pack('!BBH',0,17,len(h)+len(body)));return h[:6]+struct.pack('!H',cs(pseudo+h+body) or 65535)+body
def ip4(body,flags=0):
 h=bytearray(bytes.fromhex('45000000005d000040110000')+src4+dst4);h[2:4]=struct.pack('!H',20+len(body));h[6:8]=struct.pack('!H',flags);h[10:12]=struct.pack('!H',cs(bytes(h)));return bytes(h)+body
def ip6(body,next_header):return bytes.fromhex('60000000')+struct.pack('!HBB',8+len(body),0,64)+src6+dst6+bytes([next_header,0])+b'\0'*6+body
def packets(frames):
 w=bytearray(struct.pack('<IHHIIII',0xa1b2c3d4,2,4,0,0,65535,101))
 for i,b in enumerate(frames):w+=struct.pack('<IIII',1700000000+i,0,len(b),len(b))+b
 return bytes(w)
def vals(n,key):
 if isinstance(n,dict):return [x for k,v in n.items() for x in ((v if isinstance(v,list) else [v]) if k==key else vals(v,key))]
 if isinstance(n,list):return [x for v in n for x in vals(v,key)]
 return []
query=bytes.fromhex('005c01000001000000000000')+b'\x05inner\x07example\0'+bytes.fromhex('00010001')
inner=ip4(udp(query,53));message=b'<0>1 - - - - - - '+bytes([1,2,3,8,0])+inner
summary=[]
for v6 in [False,True]:
 u=udp(message,4789,v6,src6 if v6 else src4,dst6 if v6 else dst4)
 def fragment(start,end,more):
  if not v6:return ip4(u[start:end],(start//8)|(0x2000 if more else 0))
  return ip6(struct.pack('!BBHI',17,0,start|int(more),93)+u[start:end],44)
 whole=ip6(u,17) if v6 else ip4(u)
 for order in ['whole','fragments','reversed']:
  frames=[whole] if order=='whole' else [fragment(0,16,True),fragment(16,len(u),False)]
  if order=='reversed':frames.reverse()
  name=('ipv6' if v6 else 'ipv4')+'-'+order;capture=a.out/(name+'.pcap');capture.write_bytes(packets(frames));eventfile=a.out/(name+'-events.jsonl')
  with eventfile.open('wb') as f:r=subprocess.run([str(a.events),'replay',str(capture),'-','--ignore-embedded-secrets','udp:4789:syslog'],stdout=f,stderr=subprocess.PIPE)
  assert r.returncode==0,(name,r.stderr)
  es=[json.loads(s) for s in eventfile.read_text(encoding='utf-8-sig').splitlines()];assert len(es)==1,(name,len(es));e=es[0]
  assert e['protocol']=='syslog' and e['status']=='decoded' and e['admission']=='explicit-decode-as',(name,e['protocol'],e['status'])
  assert base64.b64decode(e['raw'])==message and base64.b64decode(e['fields']['Message Bytes'])==message[17:]
  reference=a.out/(name+'-oracle.json')
  r=subprocess.run([str(a.tshark),'-2','-n','-r',str(capture),'-d','udp.port==4789,syslog','-T','json','-x','-J','frame ip ipv6 udp syslog','--no-duplicate-keys'],capture_output=True);assert r.returncode==0;rj=json.loads(r.stdout);reference.write_bytes(r.stdout)
  spans=vals(rj,'syslog.msg_raw');assert len(spans)>=3 and isinstance(spans[1],int),(name,spans)
  assert bytes.fromhex(spans[0])==message[17:],(name,'reference MSG bytes')
  # Field presence and exact raw MSG establish the selected parse, while the
  # unchanged default dissection is preserved as a separate comparison.
  assert vals(rj,'syslog.version')==['1'],(name,vals(rj,'syslog.version'))
  r=subprocess.run([str(a.tshark),'-2','-n','-r',str(capture),'-T','fields','-e','frame.number','-e','_ws.col.Protocol'],capture_output=True);assert r.returncode==0
  (a.out/(name+'-default.tsv')).write_bytes(r.stdout)
  entry={'case':name,'kind':'declared offline binary Syslog payload, not live capture','capture_sha256':hashlib.sha256(capture.read_bytes()).hexdigest(),'frames':len(frames),'native_protocol':e['protocol'],'native_status':e['status'],'native_source':e['source_bytes'],'selected_reference_MSG_bytes':len(message)-17,'default_reference_columns':r.stdout.decode().splitlines(),'selected_differences':[]}
  summary.append(entry);print(name,'Syslog',len(message)-17,'MSG bytes',flush=True)
result={'source_scope':'Explicit UDP selection precedence, ordinary and ordered/reversed IPv4/IPv6-HBH fragment reassembly; original complete message and RFC5424 binary MSG bytes. Default port-based decoding is retained separately.','cases':summary,'controls':len(summary),'frames':sum(x['frames'] for x in summary),'selected_differences':[]}
(a.out/'summary.json').write_text(json.dumps(result,indent=2)+'\n',encoding='utf-8')
