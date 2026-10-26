"""Independently verify selected real QUIC v1/v2 1-RTT protection and key updates.
Inputs are bound to prior literal-header and independent Initial certificates.
Scope: fixed versions, observed unchanging CID lengths, complete short packets,
three traffic-secret generations, and captured application PNs below 128.
The explicitly enabled legacy-v2 negative profile diagnoses a peer using v1 KU
labels; it never makes that ciphertext valid under RFC9369 or native decoding.
Packet protection, application receipt, and peer identity are separate claims.
"""
import argparse,base64,collections,hashlib,hmac,json
from pathlib import Path
from cryptography.exceptions import InvalidTag
from cryptography.hazmat.primitives.ciphers import Cipher,algorithms,modes
from cryptography.hazmat.primitives.ciphers.aead import AESGCM,ChaCha20Poly1305

ap=argparse.ArgumentParser(description=__doc__)
for n in ['oracle','events','header-audit','initial-audit','keylog','out']:ap.add_argument('--'+n,type=Path,required=True)
ap.add_argument('--allow-legacy-v2-negative',action='store_true')
ap.add_argument('--declared-fault-manifest',type=Path)
a=ap.parse_args();checks=0
for p in vars(a).values():
 if isinstance(p,Path):assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])

def check(ok,why):
 global checks
 checks+=1;assert ok,why

def eq(x,y,why):check(x==y,(why,x,y))
def sha(w):return hashlib.sha256(w).hexdigest()
def load(p):return json.loads(p.read_text(encoding='utf-8-sig'))
def expand(secret,label,length,digest):
 label=b'tls13 '+label;info=length.to_bytes(2,'big')+bytes([len(label)])+label+b'\0';out=b'';block=b'';size=digest().digest_size
 for i in range(1,(length+size-1)//size+1):block=hmac.new(secret,block+info+bytes([i]),digest).digest();out+=block
 return out[:length]

header=load(a.header_audit);initial=load(a.initial_audit)
for c in[header,initial]:
 eq(c['oracle_sha256'],sha(a.oracle.read_bytes()),'certified oracle bytes');eq(c['events_sha256'],sha(a.events.read_bytes()),'certified native export bytes')
eq(initial['header_certificate_sha256'],sha(a.header_audit.read_bytes()),'independent Initial certificate bound to header certificate')
eq(header['reference_value_differences'],[],'no selected header/reference value differences')
facts={f['reference_connection']:f for f in initial['observed_connections']};keylog={}
faults={};faults_seen=set()
if a.declared_fault_manifest:
 manifest=load(a.declared_fault_manifest);check(manifest['transport_controls'].get('inject_transport_faults')is True,'explicit controlled packet injection profile');records=manifest['transport_faults'];faults={r['wire_sha256']:r for r in records};eq(len(faults),len(records),'unique declared wire control digests');check(records and all(r['receiver_observed_and_drained']for r in records),'control datagrams observed and deliberately drained; no peer acceptance claim')
for line in a.keylog.read_text().splitlines():
 if not line.strip()or line.startswith('#'):continue
 label,random,secret=line.split();key=(sha(bytes.fromhex(random)),label);value=bytes.fromhex(secret)
 check(key not in keylog or keylog[key]==value,'no conflicting TLS traffic secrets');keylog[key]=value
native=collections.defaultdict(list)
for line in a.events.open(encoding='utf-8-sig'):
 e=json.loads(line)
 if e['protocol']in['quic','http3','doq']:native[base64.b64decode(e['raw']or'')].append(e)
largest={};receivers=collections.defaultdict(set);counts=collections.Counter();packets=[]
for row in load(a.oracle):
 l=row['_source']['layers'];number=int(l['frame']['frame.number']);views=l.get('quic')or[];views=views if isinstance(views,list)else[views];raws=l.get('quic_raw')or[];raws=[raws]if raws and isinstance(raws[0],str)else raws
 cs={str(v['QUIC Connection information']['quic.connection.number'])for v in views if isinstance(v,dict)and'quic.connection.number'in(v.get('QUIC Connection information')or{})}
 if not cs:continue
 eq(len(cs),1,'one reference connection per selected datagram');conn=next(iter(cs));check(conn in facts,'independently observed connection');f=facts[conn];ports=[str(l['udp']['udp.srcport']),str(l['udp']['udp.dstport'])];direction=0 if ports==f['client_ports']else 1
 eq(ports,f['client_ports']if direction==0 else f['client_ports'][::-1],'literal selected direction');version=f['wire_version'];check(version in[1,0x6b3343cf],'fixed selected wire version')
 for view,r in zip(views,raws):
  w=bytes.fromhex(r[0])
  if len(w)>=7 and w[0]&0x80:
   if int.from_bytes(w[1:5],'big')!=version:continue
   dl=w[5];check(dl<=20 and 7+dl<=len(w),'long header DCID extent');sl=w[6+dl];check(sl<=20 and 7+dl+sl<=len(w),'long header SCID extent')
   receivers[(conn,direction)].add(w[7+dl:7+dl+sl]);continue
  if not w or w[0]&0xc0!=0x40:continue
  cidlen=f['receiver_CID_lengths'][1-direction];check(isinstance(cidlen,int)and 0<=cidlen<=20,'independently observed receiver CID length');pn_at=1+cidlen
  check(w[1:pn_at]in receivers[(conn,1-direction)],'short DCID belongs to previously observed receiving SCID');check(pn_at+20<=len(w),'complete short packet HP sample')
  suite=f['cipher_suite'];check(suite in[0x1301,0x1302,0x1303],'selected TLS cipher');digest=hashlib.sha384 if suite==0x1302 else hashlib.sha256;size=digest().digest_size;keylen=16 if suite==0x1301 else 32
  secret=keylog[(f['client_random_sha256'],'CLIENT_TRAFFIC_SECRET_0'if direction==0 else'SERVER_TRAFFIC_SECRET_0')];eq(len(secret),size,'traffic secret length');prefix=b'quicv2 'if version==0x6b3343cf else b'quic '
  hp=expand(secret,prefix+b'hp',keylen,digest);sample=w[pn_at+4:pn_at+20]
  enc=Cipher(algorithms.ChaCha20(hp,sample),mode=None).encryptor()if suite==0x1303 else Cipher(algorithms.AES(hp),modes.ECB()).encryptor();mask=enc.update(b'\0'*5 if suite==0x1303 else sample);first=w[0]^(mask[0]&31);n=(first&3)+1;aad=bytearray(w[:pn_at+n]);aad[0]=first
  for i in range(n):aad[pn_at+i]^=mask[1+i]
  truncated=int.from_bytes(aad[pn_at:],'big');expected=largest.get((conn,direction),-1)+1;window=1<<(8*n);pn=(expected&~(window-1))|truncated
  if pn+window//2<=expected and pn+window<1<<62:pn+=window
  elif pn>expected+window//2 and pn>=window:pn-=window
  check(pn<128,'selected small captured application packet number');phase=first>>2&1;successful=[]
  for policy in['RFC']+(['legacy-v2-negative']if a.allow_legacy_v2_negative and version==0x6b3343cf else[]):
   s=secret
   for generation in range(3):
    if generation&1==phase:
     key=expand(s,prefix+b'key',keylen,digest);iv=bytearray(expand(s,prefix+b'iv',12,digest))
     for i,b in enumerate(pn.to_bytes(8,'big'),4):iv[i]^=b
     try:plain=(ChaCha20Poly1305(key)if suite==0x1303 else AESGCM(key)).decrypt(bytes(iv),w[pn_at+n:],bytes(aad));successful.append((policy,generation,plain))
     except InvalidTag:pass
    s=expand(s,b'quic ku'if policy=='legacy-v2-negative'else prefix+b'ku',size,digest)
  check(successful,('no independently verified selected protection',number,conn,direction,pn));rfc=[x for x in successful if x[0]=='RFC'];choice=rfc[0]if rfc else successful[0];fault=faults.get(sha(w));reserved=first&24
  if fault:
   eq(pn,fault['packet_number'],'declared control independently decoded PN');eq(direction,fault['direction'],'declared control direction');eq(phase,fault['key_phase'],'declared control key phase');eq(choice[2],bytes.fromhex(fault['plaintext_hex']),'declared control exact independently decrypted bytes');eq(reserved,fault['reserved_bits'],'declared control actual reserved bits');faults_seen.add(sha(w))
  if reserved:check(fault and fault['case']=='reserved-bits'and reserved==8,'reserved-bit exceptions require this exact declared negative wire packet')
  largest[(conn,direction)]=max(pn,largest.get((conn,direction),-1))
  v=dict(view);v.update(view.get('quic.short')or{})
  if'quic.packet_number'in v:eq(pn,int(v['quic.packet_number']),'independently decoded/reference packet number')
  es=[e for e in native[w]if any(x['Number']==number for x in e['source_bytes']['PacketRefs'])];check(es,'same native physical packet and literal logical PDU')
  authenticated=[e for e in es if(e.get('session')or{}).get('Authentication Verified')]
  eq(bool(authenticated),bool(rfc),('native protection agrees with RFC-derived independent AEAD',number,conn,direction,pn,[e.get('error')for e in es]))
  for e in authenticated:
   eq(pn,e['session']['Packet Number'],'independent/native packet number')
   if'Rejected Packet Payload'in e['session']:
    eq(base64.b64decode(e['session']['Rejected Packet Payload']or''),choice[2],'inspectable refused payload equals independently authenticated octets');eq(e['session']['Rejected Packet Payload SHA256'],sha(choice[2]),'refused plaintext digest');check('not delivered'in e['session']['Payload Delivery'],'payload delivery limitation explicit');check('Frames'not in e['session'],'refused plaintext cannot masquerade as delivered frame state')
  policy,generation,plain=choice;counts[(policy,suite,generation,phase)]+=1;packets.append({'frame':number,'connection':conn,'direction':direction,'packet_number':pn,'key_phase':phase,'generation':generation,'policy':policy,'cipher_suite':suite,'wire_sha256':sha(w),'plaintext_sha256':sha(plain),'native_RFC_authenticated':bool(authenticated),'reserved_bits':reserved,'declared_control':fault['case']if fault else None})
check(packets,'selected protected short packets exist')
if faults:eq(faults_seen,set(faults),'every declared controlled wire packet independently certified')
out={'scope':__doc__,'checks':checks,'protected_short_packets':len(packets),'profiles':{str(k):v for k,v in counts.items()},'legacy_only_packets':sum(p['policy']!='RFC'for p in packets),'declared_control_packets':len(faults_seen),'nonzero_reserved_bit_controls':sum(bool(p['reserved_bits'])for p in packets),'declared_manifest_sha256':sha(a.declared_fault_manifest.read_bytes())if a.declared_fault_manifest else None,'oracle_sha256':sha(a.oracle.read_bytes()),'events_sha256':sha(a.events.read_bytes()),'keylog_sha256':sha(a.keylog.read_bytes()),'header_certificate_sha256':sha(a.header_audit.read_bytes()),'initial_certificate_sha256':sha(a.initial_audit.read_bytes()),'packets':packets};a.out.parent.mkdir(parents=True,exist_ok=True);a.out.write_text(json.dumps(out,indent=2)+'\n',encoding='utf8');print(json.dumps({k:v for k,v in out.items()if k not in['scope','packets']}))
