"""Independently decrypt selected v1/v2 Initials and check Retry transcript continuity.
Uses RFC9001/9369 HKDF/AES-GCM/header protection, never the Yak decoder. A prior header
certificate binds input/oracle/native bytes; Initial protection and the public
Retry integrity tag do not authenticate a peer. Selected observed PNs are below
128, complete client CRYPTO handshakes start at offset zero, and transports are UDP.
"""
import argparse,base64,collections,hashlib,hmac,json
from pathlib import Path
from cryptography.hazmat.primitives.ciphers import Cipher,algorithms,modes
from cryptography.hazmat.primitives.ciphers.aead import AESGCM
ap=argparse.ArgumentParser(description=__doc__)
for n in ['oracle','events','header-audit','out']:ap.add_argument('--'+n,type=Path,required=True)
ap.add_argument('--expect-retries',type=int)
a=ap.parse_args();checks=0
for p in [a.oracle,a.events,a.header_audit,a.out]:assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
def check(ok,why):
 global checks
 checks+=1;assert ok,why
def eq(x,y,why):check(x==y,(why,x,y))
def sha(w):return hashlib.sha256(w).hexdigest()
def load(p):return json.loads(p.read_text(encoding='utf-8-sig'))
def varint(w,at):
 check(at<len(w),'varint prefix');n=1<<(w[at]>>6);check(at+n<=len(w),'varint extent');return int.from_bytes(w[at:at+n],'big')&((1<<(n*8-2))-1),at+n
def expand(secret,label,length):
 label=b'tls13 '+label;info=length.to_bytes(2,'big')+bytes([len(label)])+label+b'\0';out=b'';block=b''
 for index in range(1,(length+31)//32+1):block=hmac.new(secret,block+info+bytes([index]),hashlib.sha256).digest();out+=block
 return out[:length]
def long_header(w):
 check(len(w)>=7 and w[0]&0xc0==0xc0,'selected long header');version=int.from_bytes(w[1:5],'big');check(version in[1,0x6b3343cf],'selected wire version');dl=w[5];check(dl<=20 and 7+dl<=len(w),'DCID extent');sl=w[6+dl];check(sl<=20 and 7+dl+sl<=len(w),'SCID extent');at=7+dl+sl;typ=(w[0]>>4)&3
 return w[6:6+dl],w[7+dl:at],at,(typ+3)&3 if version==0x6b3343cf else typ,version
def initial(w,seed,direction):
 dcid,scid,at,typ,version=long_header(w);eq(typ,0,'selected Initial');n,at=varint(w,at);check(at+n<=len(w),'token extent');token=w[at:at+n];at+=n;n,pn_at=varint(w,at);eq(pn_at+n,len(w),'literal complete Initial packet');check(pn_at+20<=len(w),'HP sample extent')
 salt='0dede3def700a6db819381be6e269dcbf9bd2ed9'if version==0x6b3343cf else'38762cf7f55934b34d179ae6a4c80cadccbb7f0a';prefix=b'quicv2 'if version==0x6b3343cf else b'quic '
 initial=hmac.new(bytes.fromhex(salt),seed,hashlib.sha256).digest();secret=expand(initial,b'client in'if direction==0 else b'server in',32);key=expand(secret,prefix+b'key',16);iv=expand(secret,prefix+b'iv',12);hp=expand(secret,prefix+b'hp',16)
 enc=Cipher(algorithms.AES(hp),modes.ECB()).encryptor();mask=enc.update(w[pn_at+4:pn_at+20])+enc.finalize();header=bytearray(w[:pn_at+4]);header[0]^=mask[0]&15;plen=(header[0]&3)+1
 for i in range(plen):header[pn_at+i]^=mask[1+i]
 pn=int.from_bytes(header[pn_at:pn_at+plen],'big');check(pn<128,'selected unambiguous small packet-number scope');aad=bytes(header[:pn_at+plen]);nonce=bytearray(iv)
 for i,b in enumerate(pn.to_bytes(8,'big'),4):nonce[i]^=b
 plain=AESGCM(key).decrypt(bytes(nonce),w[pn_at+plen:],aad);check(header[0]&12==0,'authenticated reserved bits');at=0;crypto=[]
 while at<len(plain):
  typ,at=varint(plain,at)
  if typ in[0,1]:continue
  if typ in[2,3]:
   largest,at=varint(plain,at);delay,at=varint(plain,at);count,at=varint(plain,at);first,at=varint(plain,at);check(count<1024,'bounded ACK ranges')
   for _ in range(count*2+(3 if typ==3 else 0)):_,at=varint(plain,at)
   continue
  if typ==6:
   off,at=varint(plain,at);n,at=varint(plain,at);check(at+n<=len(plain),'CRYPTO extent');crypto.append((off,plain[at:at+n]));at+=n;continue
  check(False,('unselected Initial frame',typ))
 return pn,token,crypto

certificate=load(a.header_audit);eq(certificate['oracle_sha256'],sha(a.oracle.read_bytes()),'certified oracle');eq(certificate['events_sha256'],sha(a.events.read_bytes()),'certified native input');eq(certificate['reference_value_differences'],[],'header audit value differences')
native=collections.defaultdict(list)
for line in a.events.open(encoding='utf-8-sig'):
 e=json.loads(line)
 if e['protocol']not in['quic','http3','doq']:continue
 for r in e['source_bytes']['PacketRefs']or[]:native[r['Number']].append(e)
connections={};pairs=[];packets=0;retry_tags=0
for row in load(a.oracle):
 l=row['_source']['layers'];number=int(l['frame']['frame.number']);vs=l.get('quic',[]);vs=vs if isinstance(vs,list)else[vs];raws=l.get('quic_raw')or[];raws=[raws]if raws and isinstance(raws[0],str)else raws
 cs={str(v['QUIC Connection information']['quic.connection.number'])for v in vs if isinstance(v,dict)and'quic.connection.number'in(v.get('QUIC Connection information')or{})}
 if not cs:continue
 eq(len(cs),1,'single observed reference connection per datagram');conn=next(iter(cs));udp=l['udp'];ports=(str(udp['udp.srcport']),str(udp['udp.dstport']))
 for view,raw in zip(vs,raws):
  w=bytes.fromhex(raw[0])
  if len(w)<7 or w[0]&0xc0!=0xc0 or int.from_bytes(w[1:5],'big')not in[1,0x6b3343cf]:continue
  dcid,scid,at,typ,version=long_header(w)
  if typ not in[0,3]:continue
  if conn not in connections:
   eq(typ,0,'observed connection begins with an Initial');connections[conn]={'original_dcid':dcid,'initial_scid':scid,'ports':ports,'first':None,'retry':None,'version':version,'cipher_suite':None,'receiver_CID_lengths':[len(scid),None]}
  c=connections[conn];direction=0 if ports==c['ports']else 1;eq(ports,c['ports']if direction==0 else c['ports'][::-1],'observed UDP direction')
  eq(version,c['version'],'selected fixed-version experiment; compatible negotiation is outside this certificate')
  if typ==3:
   eq(direction,1,'server Retry direction');eq(dcid,c['initial_scid'],'Retry destination CID');check(scid and scid!=c['original_dcid'],'Retry source CID differs from original DCID');check(at+16<len(w),'nonempty Retry token');pseudo=bytes([len(c['original_dcid'])])+c['original_dcid']+w[:-16];rk,rn=('8fb4b01b56ac48e260fbcbcead7ccc92','d86969bc2d7c6d9990efb04a')if version==0x6b3343cf else('be0c690b9f66575a1d766b54e368c84e','461599d35d632bf2239825bb');tag=AESGCM(bytes.fromhex(rk)).encrypt(bytes.fromhex(rn),b'',pseudo);eq(tag,w[-16:],'independent public Retry tag');c['retry']=(scid,w[at:-16]);retry_tags+=1;continue
  seed=c['retry'][0]if c['retry']and not(direction==0 and dcid==c['original_dcid'])else c['original_dcid'];pn,token,crypto=initial(w,seed,direction);packets+=1;eq(pn,int(view['quic.packet_number']),'independently decoded/reference PN')
  fs=view.get('quic.frame')or[];fs=fs if isinstance(fs,list)else[fs];expected=[(int(f['quic.crypto.offset']),bytes.fromhex(f['quic.crypto.crypto_data_raw'][0]))for f in fs if int(f.get('quic.frame_type','0'),0)==6];eq(crypto,expected,'independent/reference CRYPTO octets')
  c['receiver_CID_lengths'][direction]=len(scid)
  if direction!=0:
   for off,data in crypto:
    if off==0 and len(data)>=41 and data[0]==2:
     sid=data[38];check(41+sid<=len(data),'ServerHello cipher extent');c['cipher_suite']=int.from_bytes(data[39+sid:41+sid],'big')
   continue
  if c['retry']:
   eq(scid,c['initial_scid'],'client source CID unchanged after Retry');eq(token,c['retry'][1],'client carries observed Retry token')
  for off,data in crypto:
   if off!=0 or not data or data[0]!=1:continue
   eq(int.from_bytes(data[1:4],'big')+4,len(data),'selected complete ClientHello');check(len(data)>=38,'ClientHello random extent')
   if c['first']is None:c['first']=(number,data);continue
   if not c['retry']:continue
   before,old=c['first'];different=[i for i,(x,y)in enumerate(zip(old,data))if x!=y];same=old==data;events=[e for e in native[number]if base64.b64decode(e['raw']or'').startswith(w)];check(events,'native packet/refusal retains exact Initial prefix');rejected=any('conflicting retransmission'in e.get('error','')for e in events)
   eq(rejected,not same,'only conflicting ClientHello is refused; identical post-Retry transcript remains usable')
   pairs.append({'connection':conn,'before_frame':before,'after_frame':number,'before_SHA256':sha(old),'after_SHA256':sha(data),'bytes_before':len(old),'bytes_after':len(data),'same_ClientHello':same,'same_random':old[6:38]==data[6:38],'different_octets':len(different),'native_conflict_diagnosed':rejected})
if a.expect_retries is not None:eq(retry_tags,a.expect_retries,'all explicitly expected Retry packets certified')
eq({v['connection']for v in pairs},{k for k,v in connections.items()if v['retry']},'every observed Retry has a checked post-Retry ClientHello')
facts=[{'reference_connection':k,'wire_version':v['version'],'client_ports':list(v['ports']),'first_ClientHello_frame':v['first'][0]if v['first']else None,'client_random_sha256':sha(v['first'][1][6:38])if v['first']else None,'cipher_suite':v['cipher_suite'],'receiver_CID_lengths':v['receiver_CID_lengths']}for k,v in connections.items()]
out={'scope':__doc__,'checks':checks,'connections':len(connections),'observed_connections':facts,'independently_decrypted_Initial_packets':packets,'verified_public_Retry_tags':retry_tags,'post_Retry_ClientHello_pairs':pairs,'nonconforming_pairs':sum(not v['same_ClientHello']for v in pairs),'oracle_sha256':sha(a.oracle.read_bytes()),'events_sha256':sha(a.events.read_bytes()),'header_certificate_sha256':sha(a.header_audit.read_bytes())};a.out.parent.mkdir(parents=True,exist_ok=True);a.out.write_text(json.dumps(out,indent=2)+'\n',encoding='utf8');print(json.dumps(out))
