"""Every-frame Wireshark/CLI/native comparisons for fresh ingress captures.
Usage: ingress_compare.py CAPTURE_DIR OUTPUT_DIR TSHARK YAK EVENTS ssh|wireguard|control
"""
import base64,collections,csv,hashlib,io,json,subprocess,sys
from pathlib import Path
csv.field_size_limit(8000000)
cap,out,ts,yak,audit=map(Path,sys.argv[1:6]);mode=sys.argv[6];out.mkdir(exist_ok=True)
m=json.loads((cap/'manifest.json').read_text());sha=lambda w:hashlib.sha256(w).hexdigest()
def run(args,name):
 p=subprocess.run([str(x) for x in args],stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=120);(out/name).write_bytes(p.stdout);(out/(name+'.stderr')).write_bytes(p.stderr);assert p.returncode==0,(name,p.stderr.decode(errors='replace')[:1000]);return p.stdout.decode('utf-8-sig')
available={s.split('\t')[2] for s in subprocess.check_output([str(ts),'-G','fields']).decode(errors='replace').splitlines() if s.startswith('F\t')}
fields=['frame.number','frame.protocols','_ws.col.Protocol','_ws.col.Info','tcp.stream','tcp.srcport','tcp.dstport','tcp.len','udp.srcport','udp.dstport','udp.payload','_ws.expert.message']
extra=['ssh.protocol','ssh.message_code','ssh.kex_algorithms','ssh.server_host_key_algorithms','wg.type','wg.sender','wg.receiver','wg.counter','isakmp.exchangetype','isakmp.messageid','isakmp.length','rip.command','rip.version','rip.metric','nbdgm.type','nbdgm.dgram_id','nbdgm.dgram_len','rpc.xid','rpc.msgtyp','rpc.program','l2tp.tunnel','l2tp.session','openvpn.opcode','openvpn.keyid','rmcp.version','rmcp.class']
extra += ['isakmp.version','isakmp.flags','rip.ip','rip.netmask','rip.next_hop','l2tp.flags','l2tp.length','l2tp.Ns','l2tp.Nr','l2tp.avp.message_type','nbdgm.flags','nbdgm.source_name','nbdgm.destination_name']
fields += [f for f in extra if f in available]
args=[ts,'-2','-n','-r',cap/'live.pcap','-o','tcp.reassemble_out_of_order:TRUE'];yargs=[yak,'shark','-r',cap/'live.pcap','--json'];eargs=[audit,'replay',cap/'live.pcap','-']
if mode=='ssh':
 for p in m['ports']:args+=['-d',f'tcp.port=={p},ssh']
if mode=='wireguard':args+=['-d','udp.port==51820,wg']
if mode=='control':
 decoders={'ike':'isakmp','ike-natt':'udpencap','rip':'rip','nbt-dg':'nbdgm','onc-rpc':'rpc','l2tp':'l2tp','openvpn':'openvpn','ipmi':'rmcp'}
 for name,p in m['ports'].items():args+=['-d',f'udp.port=={p},{decoders[name]}']
 for v in m['fallbacks']:
  value=f"udp:{v['actual']}:{v['protocol']}";yargs+=['--decode-as',value];eargs+=[value]
 args+=['-d',f"tcp.port=={m['telnet_port']},telnet"]
 if m.get('nbss_port'):args+=['-d',f"tcp.port=={m['nbss_port']},nbss"]
args+=['-T','fields','-E','header=y','-E','occurrence=a']
for f in fields:args+=['-e',f]
ws=list(csv.DictReader(io.StringIO(run(args,'tshark.tsv')),delimiter='\t',quoting=csv.QUOTE_NONE));ys=[json.loads(s) for s in run(yargs,'yak.jsonl').splitlines()];es=[json.loads(s) for s in run(eargs,'events.jsonl').splitlines()]
assert len(ws)==len(ys)
tokens=lambda s:[x for x in s.split(',') if x]
byframe=collections.defaultdict(list)
for e in es:
 for r in e['source_bytes'].get('PacketRefs') or []:byframe[r['Number']].append(e)
summary=dict(frames=len(ws),events=dict(collections.Counter(e['protocol']+'/'+e['status'] for e in es)),yak_protocols=dict(collections.Counter(y['protocol'] for y in ys)),wireshark_protocols=dict(collections.Counter(w['_ws.col.protocol'] for w in ws)),pcap_sha256=sha((cap/'live.pcap').read_bytes()))
checked=0
if mode=='ssh':
 # Readiness markers are independently captured UDP, never SSH evidence.
 markers=[e for e in es if str(m.get('marker_port',-1)) in [e['source'].split(':')[-1],e['destination'].split(':')[-1]]]
 assert all(e['protocol']!='ssh' and e['status']!='decoded' for e in markers)
 application_events=[e for e in es if e not in markers]
 hashes=[];negotiated=set()
 for e in application_events:
  if e['protocol']!='ssh' or e['status']!='decoded':continue
  s=e['session'];refs=e['source_bytes']['PacketRefs'];w=ws[max(r['Number'] for r in refs)-1]
  if s['Packet Name']=='identification':assert s['Identification'] in tokens(w['ssh.protocol']);checked+=1
  if 'Message Number' in s:
   assert str(s['Message Number']) in tokens(w['ssh.message_code']),(e['id'],s['Packet Name'],w);checked+=1
  if s.get('Kex Algorithm'):negotiated.add(s['Kex Algorithm'])
  if s.get('Host Key SHA256'):hashes.append(s['Host Key SHA256'])
  if s.get('Kex Algorithms'):assert s['Kex Algorithms']==w['ssh.kex_algorithms']
  if s.get('Host Key Algorithms'):assert s['Host Key Algorithms']==w['ssh.server_host_key_algorithms']
 assert len(hashes)==len(m['operations']) and set(hashes)=={m['host_key_sha256']}
 assert negotiated=={o['kex'] for o in m['operations']}
 assert not [e for e in application_events if e['status'] not in ['decoded','context-required']]
 assert all(e.get('expert')=='Encrypted' for e in application_events if e['status']=='context-required')
 # Authentication/channel/application data remain cryptographically opaque.
 assert {e['protocol'] for e in application_events}=={'ssh'}
 summary.update(host_key_fingerprints_verified=len(hashes),negotiated_kex=sorted(negotiated),application_bytes_received=sum(o['bytes'] for o in m['operations']))
elif mode=='wireguard':
 markers=[e for e in es if str(m.get('marker_port',-1)) in [e['source'].split(':')[-1],e['destination'].split(':')[-1]]]
 assert all(e['protocol']!='wireguard' and e['status']!='decoded' for e in markers)
 application_events=[e for e in es if e not in markers]
 for e in application_events:
  assert e['protocol']=='wireguard' and e['status']=='decoded';s=e['session'];w=ws[e['source_bytes']['PacketRefs'][0]['Number']-1]
  for name,key in [('Message Type','wg.type'),('Sender Index','wg.sender'),('Receiver Index','wg.receiver'),('Counter','wg.counter')]:
   if name in s:assert s[name]==int(w[key],0),(name,s[name],w[key]);checked+=1
  assert s['Authentication Verified'] is False and s['Content Visibility']=='encrypted'
  assert base64.b64decode(e['raw'])==bytes.fromhex(w['udp.payload'].replace(':',''))
 assert len(application_events)==18 and len(ws)==len(application_events)+len(markers)
 assert all(y['protocol']=='WireGuard' for w,y in zip(ws,ys) if str(m.get('marker_port',-1)) not in [w['udp.srcport'],w['udp.dstport']])
 summary['independent_tun_deliveries']=len(m['operations'])
else:
 positive={o['sha256']:o for o in m['operations'] if o.get('positive')};seen=[]
 for w in ws:
  if not w['udp.payload']:continue
  wire=bytes.fromhex(w['udp.payload'].replace(':',''));digest=sha(wire);native=byframe[int(w['frame.number'])]
  if digest in positive:
   op=positive[digest];name='ike' if op['protocol']=='ike-natt' else op['protocol'];matches=[e for e in native if e['protocol']==name and e['status']=='decoded'];assert len(matches)==1,(op,w,native)
   e=matches[0];assert base64.b64decode(e['raw'])==wire;s=e['session']
   for key,field in [('Exchange Type','isakmp.exchangetype'),('Message ID','isakmp.messageid'),('Datagram ID','nbdgm.dgram_id'),('Datagram Length','nbdgm.dgram_len'),('XID','rpc.xid'),('Opcode','openvpn.opcode'),('Key ID','openvpn.keyid'),('RMCP Version','rmcp.version'),('RMCP Class','rmcp.class')]:
    if key in s and field in w and w[field]:assert s[key]==int(w[field],0),(op['protocol'],key,s[key],w[field]);checked+=1
   pairs={'ike':[('Version','isakmp.version'),('Flags','isakmp.flags'),('Length','isakmp.length')], 'rip':[('Command','rip.command'),('Version','rip.version')], 'nbt-dg':[('Message Type','nbdgm.type'),('Flags','nbdgm.flags')], 'onc-rpc':[('Message Type','rpc.msgtyp'),('Program','rpc.program')], 'l2tp':[('Flags','l2tp.flags'),('Length','l2tp.length'),('Tunnel ID','l2tp.tunnel'),('Session ID','l2tp.session'),('Message Type','l2tp.avp.message_type')]}
   for key,field in pairs.get(name,[]):
    if key in s:
     assert field in w and w[field],(name,key,'Wireshark field missing')
     assert s[key]==int(w[field],0),(name,key,s[key],w[field]);checked+=1
   if name=='rip':
    assert [z['Metric'] for z in s['Entries']]==[int(z,0) for z in tokens(w['rip.metric'])];checked+=len(s['Entries'])
   if name=='nbt-dg':
    for key,field in [('Source Name','nbdgm.source_name'),('Destination Name','nbdgm.destination_name')]:
     assert w[field].split('<')[0].rstrip()==s[key],(s[key],w[field]);checked+=1
   # Every declared root byte range must still point to the original PDU.
   assert all(0<=lo<=hi<=len(wire) for lo,hi in s.get('Wire Field Ranges',{}).values())
   seen.append(digest)
  else:assert not any(e['status']=='decoded' for e in native),(w,native)
 assert collections.Counter(seen)==collections.Counter(positive.keys())
 # Reconstruct delivered application data from complete native Telnet tokens,
 # preserving escaped IAC and excluding negotiation/subnegotiation bytes.
 bodies=collections.defaultdict(bytearray)
 for e in es:
  if e['protocol']!='telnet' or e['status']!='decoded' or e['source'].split(':')[-1]!=str(m['telnet_port']):continue
  w=base64.b64decode(e['raw']);name=e['session']['Packet Name']
  if name=='Data':bodies[e['flow']]+=w
  elif name=='Escaped IAC':bodies[e['flow']]+=b'\xff'
 wanted=bytes(range(256))*16;assert len(bodies)==2 and all(bytes(b)==wanted for b in bodies.values()),[(k,len(v)) for k,v in bodies.items()]
 summary.update(positive_datagrams=len(seen),negative_datagrams=sum('positive' in o and not o['positive'] for o in m['operations']),telnet_binary_bodies_verified=len(bodies),explicit_nonstandard_profiles=m['fallbacks'])
 if m.get('nbss_port'):
  nbss=[e for e in es if e['protocol']=='nbss' and e['status']=='decoded'];assert len(nbss)==3
  assert [e['session']['Packet Name'] for e in nbss]==['Session Request','Positive Session Response','Session Keepalive']
  summary['nbss_controls_verified']=3
summary['semantic_fields_checked']=checked
with (out/'frame-comparison.csv').open('w',encoding='utf-8-sig',newline='') as f:
 writer=csv.writer(f);writer.writerow(['frame','ws_protocol','yak_protocol','protocol_stack','connection_stack','ws_info','yak_info','events'])
 for w,y in zip(ws,ys):
  assert int(w['frame.number'])==y['number'];writer.writerow([y['number'],w['_ws.col.protocol'],y['protocol'],' > '.join(y.get('protocol_stack',[])),' > '.join(y.get('connection_stack',[])),w['_ws.col.info'],y['info'],';'.join(e['protocol']+'/'+e['status'] for e in byframe[y['number']])])
(out/'summary.json').write_text(json.dumps(summary,ensure_ascii=False,indent=2),encoding='utf8');print(json.dumps(summary,ensure_ascii=False,indent=2))
