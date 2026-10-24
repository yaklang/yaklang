"""Fresh SSH/WireGuard loss/reorder/snaplen/duplicate and TCP tuple reuse.
Usage: ingress_faults.py ROOT TSHARK YAK EVENTS
"""
import base64,collections,csv,hashlib,io,json,struct,subprocess,sys
from pathlib import Path
root,ts,yak,audit=map(Path,sys.argv[1:5]);out=root/'faults';out.mkdir(exist_ok=True)
def read(path):
 w=path.read_bytes();assert w[:4] in [bytes.fromhex('d4c3b2a1'),bytes.fromhex('4d3cb2a1')];head=w[:24];frames=[];at=24
 while at<len(w):
  sec,usec,n,original=struct.unpack_from('<IIII',w,at);at+=16;frames.append([sec,usec,w[at:at+n],original]);at+=n
 return head,frames
def info(f,head):
 w=f[2];link=struct.unpack_from('<I',head,20)[0];off=4 if link==0 else 14 if link==1 else 0;assert w[off]>>4==4
 ip=off;off+=(w[off]&15)*4;proto=w[ip+9];sp,dp=struct.unpack_from('!HH',w,off);payload=off+((w[off+12]>>4)*4 if proto==6 else 8)
 return dict(ip=ip,l4=off,proto=proto,sp=sp,dp=dp,payload=payload,data=w[payload:],flags=w[off+13] if proto==6 else 0)
def write(path,head,frames):
 with path.open('wb') as f:
  f.write(head)
  for sec,usec,w,original in frames:f.write(struct.pack('<IIII',sec,usec,len(w),original));f.write(w)

def checksum(w):
 if len(w)%2:w+=b'\0'
 total=sum(struct.unpack('!'+'H'*(len(w)//2),w))
 while total>>16:total=(total&65535)+(total>>16)
 return (~total)&65535

def tcp_slice(frame,p,start,end):
 w=bytearray(frame[2][:p['payload']]+p['data'][start:end]);seq=struct.unpack_from('!I',w,p['l4']+4)[0]
 struct.pack_into('!I',w,p['l4']+4,(seq+start)&0xffffffff)
 struct.pack_into('!H',w,p['ip']+2,len(w)-p['ip']);w[p['ip']+10:p['ip']+12]=b'\0\0'
 struct.pack_into('!H',w,p['ip']+10,checksum(bytes(w[p['ip']:p['l4']])))
 w[p['l4']+16:p['l4']+18]=b'\0\0';segment=bytes(w[p['l4']:]);pseudo=bytes(w[p['ip']+12:p['ip']+20])+bytes([0,6])+struct.pack('!H',len(segment))
 struct.pack_into('!H',w,p['l4']+16,checksum(pseudo+segment))
 return [frame[0],frame[1],bytes(w),len(w)]
def run(args,file):
 p=subprocess.run([str(a) for a in args],stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=120);file.write_bytes(p.stdout);file.with_suffix(file.suffix+'.stderr').write_bytes(p.stderr);assert p.returncode==0,(file,p.stderr[:400]);return p.stdout.decode('utf-8-sig')
baseline={};sources={}
for mode,folder in [('ssh','ssh-02'),('wireguard','wireguard-01')]:
 cap=root/folder;head,frames=read(cap/'live.pcap');es=[json.loads(l) for l in (cap/'comparison/events.jsonl').read_text(encoding='utf8').splitlines()];baseline[mode]=collections.Counter(hashlib.sha256(base64.b64decode(e['raw'])).hexdigest() for e in es if e['status']=='decoded');sources[mode]=(cap,head,frames)
variants=[]
for mode in sources:
 cap,head,fs=sources[mode];parsed=[info(f,head) for f in fs]
 if mode=='ssh':
  target=next(e['source_bytes']['PacketRefs'][-1]['Number']-1 for e in [json.loads(l) for l in (cap/'comparison/events.jsonl').read_text(encoding='utf8').splitlines()] if (e.get('session') or {}).get('Message Number')==20)
  data=[i for i,p in enumerate(parsed) if len(p['data'])>1000];a,b=data[:2]
 else:target=2;a,b=2,4
 variants += [(mode+'-duplicate',mode,fs[:target+1]+[fs[target]]+fs[target+1:],dict(duplicate=target+1)),(mode+'-loss',mode,fs[:target]+fs[target+1:],dict(drop=target+1))]
 reorder=[f.copy() for f in fs];reorder[a],reorder[b]=reorder[b],reorder[a];variants.append((mode+'-reorder',mode,reorder,dict(swapped=[a+1,b+1])))
 trunc=[f.copy() for f in fs];trunc[target][2]=trunc[target][2][:parsed[target]['payload']+9];variants.append((mode+'-snaplen',mode,trunc,dict(truncated=target+1,original_wire_length_preserved=True)))
 if mode=='ssh':
  # Split an actual plaintext KEXINIT into two valid TCP/IP packets and
  # deliver its latter sequence range first, after the observed SYN/banner.
  # This exercises handshake reassembly, not just encrypted bulk-data order.
  p=parsed[target];cut=len(p['data'])//2
  split=[tcp_slice(fs[target],p,cut,len(p['data'])),tcp_slice(fs[target],p,0,cut)]
  variants.append(('ssh-kex-segment-reorder','ssh',fs[:target]+split+fs[target+1:],dict(split_original_frame=target+1,split_offset=cut,arrival='second half first',checksums_recomputed=True)))
  repaired=trunc[:target+3]+[fs[target]]+trunc[target+3:];variants.append(('ssh-snaplen-retransmitted','ssh',repaired,dict(truncated=target+1,retransmitted=True)))
  ident=next(i for i,p in enumerate(parsed) if p['data'].startswith(b'SSH-'));variants.append(('ssh-missing-identification','ssh',fs[:ident]+fs[ident+1:],dict(drop=ident+1)))
  manifest=json.loads((cap/'manifest.json').read_text());p1,p2=manifest['ports'][:2]
  c1=next(p['sp'] for p in parsed if p['dp']==p1 and p['flags']&2 and not p['flags']&16);c2=next(p['sp'] for p in parsed if p['dp']==p2 and p['flags']&2 and not p['flags']&16)
  reused=[f.copy() for f in fs]
  for f,p in zip(reused,parsed):
   if p2 not in (p['sp'],p['dp']):continue
   w=bytearray(f[2]);ports=[p1 if v==p2 else c1 if v==c2 else v for v in [p['sp'],p['dp']]];struct.pack_into('!HH',w,p['l4'],*ports);w[p['l4']+16:p['l4']+18]=b'\0\0'
   segment=w[p['l4']:];pseudo=w[p['ip']+12:p['ip']+20]+bytes([0,6])+struct.pack('!H',len(segment));data=pseudo+segment
   if len(data)%2:data+=b'\0'
   total=sum(struct.unpack('!'+'H'*(len(data)//2),data))
   while total>>16:total=(total&65535)+(total>>16)
   struct.pack_into('!H',w,p['l4']+16,(~total)&65535);f[2]=bytes(w)
  variants.append(('ssh-tcp-tuple-reuse','ssh',reused,dict(second_ports_replaced=[p2,c2],new_ports=[p1,c1],tcp_checksum_recomputed=True)))
 else:
  variants.append(('wireguard-missing-initial','wireguard',fs[1:],dict(drop=1)))
  changed=[f.copy() for f in fs];w=bytearray(changed[target][2]);w[parsed[target]['payload']+4]^=0x80;changed[target][2]=bytes(w);variants.append(('wireguard-unknown-receiver','wireguard',changed,dict(modified_receiver_frame=target+1)))
results=[]
for name,mode,frames,mutation in variants:
 directory=out/name;directory.mkdir(exist_ok=True);cap,head,original=sources[mode];path=directory/'live.pcap';write(path,head,frames)
 e=[json.loads(l) for l in run([audit,'replay',path],directory/'events.jsonl').splitlines()];y=[json.loads(l) for l in run([yak,'shark','-r',path,'--json'],directory/'yak.jsonl').splitlines()]
 args=[ts,'-2','-n','-r',path,'-o','tcp.reassemble_out_of_order:TRUE']
 if mode=='ssh':
  for port in json.loads((cap/'manifest.json').read_text())['ports']:args+=['-d',f'tcp.port=={port},ssh']
 else:args+=['-d','udp.port==51820,wg']
 args+=['-T','fields','-E','header=y','-e','frame.number','-e','_ws.col.Protocol','-e','_ws.col.Info']
 w=list(csv.DictReader(io.StringIO(run(args,directory/'tshark.tsv')),delimiter='\t',quoting=csv.QUOTE_NONE));assert len(w)==len(y)==len(frames)
 if mode=='ssh':
  observed=collections.Counter(hashlib.sha256(base64.b64decode(z['raw'])).hexdigest() for z in e if z['status']=='decoded');assert not observed-baseline[mode],(name,'invented decoded PDU')
  if name in ['ssh-duplicate','ssh-reorder','ssh-kex-segment-reorder','ssh-snaplen-retransmitted','ssh-tcp-tuple-reuse']:assert observed==baseline[mode],(name,'intact retransmitted messages lost')
  if name=='ssh-tcp-tuple-reuse':assert len({z['flow'] for z in e if z['flow']})==4
 else:
  assert all(z['protocol'] in ['','wireguard'] for z in e),(name,'encrypted bytes became another protocol')
  assert not any((z.get('session') or {}).get('Authentication Verified') for z in e)
  if name=='wireguard-missing-initial':assert not any(z['protocol']=='wireguard' and z['status']=='decoded' for z in e)
 with (directory/'frame-comparison.csv').open('w',encoding='utf-8-sig',newline='') as f:
  writer=csv.writer(f);writer.writerow(['frame','ws_protocol','yak_protocol','ws_info','yak_info']);writer.writerows([i+1,a['_ws.col.protocol'],b['protocol'],a['_ws.col.info'],b['info']] for i,(a,b) in enumerate(zip(w,y)))
 result=dict(name=name,frames=len(frames),source_sha256=hashlib.sha256((cap/'live.pcap').read_bytes()).hexdigest(),mutation=mutation,events=dict(collections.Counter(z['protocol']+'/'+z['status'] for z in e)));results.append(result);print('PASS',name,len(frames),flush=True)
(out/'summary.json').write_text(json.dumps(results,indent=2),encoding='utf8')
