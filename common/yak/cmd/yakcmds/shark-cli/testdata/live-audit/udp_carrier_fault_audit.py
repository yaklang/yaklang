"""Declared offline UDP/IPv6 carrier controls; never a claim of live capture.
Run with --out outside Git, --events SDK probe and --tshark unmodified reference.
"""
from pathlib import Path
import argparse,base64,collections,hashlib,ipaddress,json,struct,subprocess,sys
ap=argparse.ArgumentParser(description=__doc__)
for name in ['out','events','tshark']:ap.add_argument('--'+name,required=True,type=Path)
a=ap.parse_args();a.out.mkdir(parents=True,exist_ok=True)
assert not any((p/'.git').exists() for p in [a.out.resolve(),*a.out.resolve().parents])
query=bytes.fromhex('004801000001000000000000')+b'\x07carrier\x07example\0'+bytes.fromhex('00010001')
def checksum(w):
 if len(w)%2:w+=b'\0'
 n=sum(struct.unpack('!%dH'%(len(w)//2),w))
 while n>>16:n=(n&65535)+(n>>16)
 return (~n)&65535
def udp(declared):return struct.pack('!HHHH',40000,53,declared,0)+query
def ip4(body):
 h=bytearray(bytes.fromhex('450000000001000040110000c0000201c0000202'));h[2:4]=struct.pack('!H',20+len(body));h[10:12]=struct.pack('!H',checksum(bytes(h)));return bytes(h)+body
def ip6(body,extensions='',next_header=17):
 for kind in reversed(extensions):
  body=bytes([next_header,0])+b'\0'*6+body;next_header={'h':0,'d':60}[kind]
 return bytes.fromhex('60000000')+struct.pack('!HBB',len(body),next_header,64)+ipaddress.IPv6Address('2001:db8::1').packed+ipaddress.IPv6Address('2001:db8::2').packed+body
def pcap(frames):
 w=bytearray(struct.pack('<IHHIIII',0xa1b2c3d4,2,4,0,0,65535,101))
 for i,(body,wirelen) in enumerate(frames):w+=struct.pack('<IIII',1700000000+i,0,len(body),wirelen)+body
 return bytes(w)
cases=[]
for family in ['ipv4','ipv6','ipv6-hbh','ipv6-dest','ipv6-hbh-dest']:
 for mode in ['complete','bad-long','bad-short','bad-zero','capture-cut','cut-and-contradiction','link-padding']:
  declared={'bad-long':2000,'bad-short':7,'bad-zero':0,'cut-and-contradiction':2000}.get(mode,8+len(query))
  body=udp(declared)
  full=ip4(body) if family=='ipv4' else ip6(body,{'ipv6':'','ipv6-hbh':'h','ipv6-dest':'d','ipv6-hbh-dest':'hd'}[family])
  raw=full[:-3] if mode in ['capture-cut','cut-and-contradiction'] else full
  if mode=='link-padding':raw+=b'\xa5'*16
  expected='incomplete' if mode=='capture-cut' else 'malformed' if mode.startswith('bad-') or mode=='cut-and-contradiction' else 'decoded'
  cases.append((family+'-'+mode,[(raw,len(full) if mode in ['capture-cut','cut-and-contradiction'] else len(raw))],expected,False))
inner=bytearray(ip4(udp(8+len(query))));inner[12:20]=bytes.fromhex('c6336401c6336402');inner[10:12]=b'\0\0';inner[10:12]=struct.pack('!H',checksum(bytes(inner[:20])));inner=bytes(inner)
ethernet=bytes.fromhex('0200000000020200000000010800')+inner
gtp=bytes.fromhex('30ff')+struct.pack('!H',len(inner))+bytes.fromhex('00000001')+inner
for tunnel,port,payload in [('vxlan',4789,bytes.fromhex('0800000000000100')+ethernet),('geneve',6081,bytes.fromhex('0000655800000100')+ethernet),('gtp',2152,gtp)]:
 for mode in ['valid','bad-long','capture-cut']:
  body=struct.pack('!HHHH',40000,port,2000 if mode=='bad-long' else 8+len(payload),0)+payload;raw=ip4(body)
  expected='malformed' if mode=='bad-long' else 'incomplete' if mode=='capture-cut' else 'limited' if tunnel=='gtp' else 'decoded'
  cases.append(('tunnel-'+tunnel+'-'+mode,[(raw,len(raw)+(4 if mode=='capture-cut' else 0))],expected,False))
for bad in [False,True]:
 body=udp(2000 if bad else 8+len(query))
 def frag(offset,more,part):return ip6(struct.pack('!BBHI',17,0,offset|int(more),123)+part,'h',44)
 first,last=frag(0,True,body[:16]),frag(16,False,body[16:]);conflict=first[:-1]+bytes([first[-1]^1])
 for name,frames,expected in [('ordered',[first,last],'malformed' if bad else 'decoded'),('reversed',[last,first],'malformed' if bad else 'decoded'),('duplicate',[first,first,last],'malformed' if bad else 'decoded'),('missing-tail',[first],'incomplete'),('conflicting-overlap',[first,conflict,last],'malformed')]:
  cases.append(('ipv6-hbh-fragments-'+('bad-' if bad else '')+name,[(w,len(w)) for w in frames],expected,True))
summary=[];checks=0
for name,frames,expected,fragmented in cases:
 capture=a.out/(name+'.pcap');capture.write_bytes(pcap(frames));eventfile=a.out/(name+'-events.jsonl');oracle=a.out/(name+'-oracle.json')
 with eventfile.open('wb') as f:r=subprocess.run([str(a.events),'replay',str(capture),'-','--ignore-embedded-secrets'],stdout=f,stderr=subprocess.PIPE)
 assert r.returncode==0,(name,r.stderr)
 native=[json.loads(s) for s in eventfile.read_text(encoding='utf-8-sig').splitlines()]
 assert len(native)==1,(name,[(e['protocol'],e['status']) for e in native]);e=native[0]
 assert e['status']==expected,(name,e['status'],e.get('error'));checks+=2
 r=subprocess.run([str(a.tshark),'-2','-n','-r',str(capture),'-T','json','-x','-J','frame ip ipv6 udp icmpv6 dns','--no-duplicate-keys'],capture_output=True);assert r.returncode==0,(name,r.stderr);oracle.write_bytes(r.stdout)
 reference=json.loads(r.stdout)
 def vals(node,key):
  if isinstance(node,dict):return [x for k,v in node.items() for x in ((v if isinstance(v,list) else [v]) if k==key else vals(v,key))]
  if isinstance(node,list):return [x for v in node for x in vals(v,key)]
  return []
 dns_ids=vals(reference,'dns.id')
 entry={'case':name,'kind':'declared offline carrier mutation, not live capture','capture_sha256':hashlib.sha256(capture.read_bytes()).hexdigest(),'frames':len(frames),'native_protocol':e['protocol'],'native_status':e['status'],'native_source':e['source_bytes'],'reference_DNS_views':len(dns_ids),'expected_status':expected}
 if expected=='decoded':
  assert e['protocol']=='dns' and dns_ids==['0x0048'],(name,e['protocol'],dns_ids);checks+=1
  fields=e['field_values']['values'];aliases=collections.defaultdict(list)
  for f in fields:aliases[f['name']].append(f['value'])
  assert aliases['dns.id']==[72] and aliases['dns.qry.name']==['carrier.example'] and aliases['dns.qry.type']==[1],(name,aliases);checks+=3
  assert base64.b64decode(e['raw'])==query;checks+=1
 elif name=='tunnel-gtp-valid':
  assert e['protocol']=='ip' and e['error']=='EncapsulationDepthOrProfile';checks+=2
  entry['retained_difference']='Valid GTP inner normalization remains outside the selected native tunnel profile; not counted as a decoded application'
 elif fragmented and name.endswith(('missing-tail','conflicting-overlap')):
  assert e['protocol']=='ip';checks+=1
  if dns_ids:entry['retained_difference']='Wireshark best-effort overlapping-fragment payload; native quarantines conflicting bytes'
 else:
  assert not e['protocol'];checks+=1
  if name.startswith('tunnel-'):
   assert e['source']=='192.0.2.1:40000' and not e.get('fields');checks+=2
  if dns_ids:entry['retained_difference']=('Wireshark exposes partial application fields with an unavailable captured suffix; native refuses complete application admission' if expected=='incomplete' else 'Wireshark dissects application despite impossible UDP carrier; native refuses application admission')
 if fragmented and name.endswith(('ordered','reversed','duplicate')):
  refs=[x['Number'] for x in e['source_bytes']['PacketRefs']]
  wanted=[2,1] if name.endswith('reversed') else [1,3] if name.endswith('duplicate') else [1,2]
  assert refs==wanted and e['source_bytes']['Kind']=='reassembled',(name,refs,wanted);checks+=2
  completed=e['source_bytes'].get('CompletedIn')
  assert (completed['Number']==max(wanted) if expected=='decoded' else completed is None),(name,completed);checks+=1
 if not fragmented:
  audit=a.out/(name+'-audit.json')
  r=subprocess.run([sys.executable,'-X','utf8',str(Path(__file__).with_name('udp_carrier_wire_audit.py')),str(oracle),str(eventfile),str(audit)],capture_output=True)
  assert r.returncode==0,(name,r.stderr.decode(errors='replace'))
  verified=json.loads(audit.read_bytes());entry['wire_checks']=verified['checks'];entry['selected_differences']=verified['selected_differences'];checks+=verified['checks']
 summary.append(entry);print(name,e['protocol'],e['status'],len(dns_ids),flush=True)
result={'controls':len(summary),'frames':sum(x['frames'] for x in summary),'checks':checks,'selected_differences':[],'cases':summary,'scope':'UDP boundary admission and IPv6 extension/fragment recovery, literal DNS ID/name/type/raw message. No checksum/jumbogram/MLD-body certification.'}
(a.out/'summary.json').write_text(json.dumps(result,indent=2)+'\n',encoding='utf-8')
