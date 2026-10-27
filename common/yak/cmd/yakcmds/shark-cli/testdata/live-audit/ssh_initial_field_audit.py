"""Independently decode selected clear SSH DH/GEX/ECDH/RSA/EdDSA field layouts.
Use RFC 4253/4419/5656/8709 binary strings and bounded cursors, not a Yak codec.
Compare the entire native initial packet field tree with exact PDU bytes, then
compare selected values with byte/frame-matched Wireshark views. This certifies
framing, values and fingerprints, not DH mathematics, a signature or identity.
"""
import argparse,base64,collections,hashlib,ipaddress,json,re
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in['events','oracle','out']:ap.add_argument('--'+n,type=Path,required=True)
ap.add_argument('--wire-certificate',type=Path)
a=ap.parse_args();checks=0;wire_fields=0;ref_fields=0;reference=collections.defaultdict(list)
for p in vars(a).values():
 if isinstance(p,Path):assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
def check(ok,why):
 global checks
 checks+=1;assert ok,why
def eq(x,y,why):check(x==y,why)
def digest(w):return hashlib.sha256(w).hexdigest()
def field_digest(v):return digest(json.dumps(v,sort_keys=True,separators=(',',':')).encode())
def leaf_count(v):
 if isinstance(v,dict):return sum(leaf_count(x)for x in v.values())
 if isinstance(v,list):return sum(leaf_count(x)for x in v)
 return 1
wire_certificate=None
if a.wire_certificate:
 wire_certificate=json.loads(a.wire_certificate.read_text());eq(wire_certificate['events_sha256'],digest(a.events.read_bytes()),'independent PCAP/TCP/source audit binds the exact SDK bytes');eq(wire_certificate['oracle_sha256'],digest(a.oracle.read_bytes()),'independent PCAP/frame audit binds the exact reference bytes')
def leaves(o,key):
 if isinstance(o,dict):
  for k,v in o.items():
   if k==key:yield from(v if isinstance(v,list)else[v])
   elif isinstance(v,(dict,list)):yield from leaves(v,key)
 elif isinstance(o,list):
  for v in o:yield from leaves(v,key)
def methods(o):
 if isinstance(o,dict):
  for k,v in o.items():
   m=re.fullmatch(r'Key Exchange \(method:(.+)\)',k)
   if m:yield m.group(1)
   if isinstance(v,(dict,list)):yield from methods(v)
 elif isinstance(o,list):
  for v in o:yield from methods(v)
reference_endpoints={};attribution_differences=[];unselected_reference_carriers=[]
for row in json.loads(a.oracle.read_text(encoding='utf-8-sig')):
 l=row['_source']['layers'];views=l.get('ssh')or[];views=views if isinstance(views,list)else[views];raws=l.get('ssh_raw')or[];raws=[raws]if raws and isinstance(raws[0],str)else raws
 number=int(l['frame']['frame.number']);chain=l['frame'].get('frame.protocols','').split(':')
 if'tcp'not in chain or not l.get('tcp'):unselected_reference_carriers.append({'physical_frame':number,'reference_protocol_stack':':'.join(chain),'scope':'selected auditor requires observed IP/TCP carrier'});continue
 tcp_at=max(i for i,v in enumerate(chain)if v=='tcp');family=next(v for v in reversed(chain[:tcp_at])if v in['ip','ipv6']);src=list(leaves(l.get(family),family+'.src'))[-1];dst=list(leaves(l.get(family),family+'.dst'))[-1];sport=list(leaves(l.get('tcp'),'tcp.srcport'))[-1];dport=list(leaves(l.get('tcp'),'tcp.dstport'))[-1];reference_endpoints[number]=(ipaddress.ip_address(src),int(sport),ipaddress.ip_address(dst),int(dport),int(l['frame'].get('frame.interface_id',0)))
 for view,rr in zip(views,raws):
  w=bytes.fromhex(rr[0]);pos=0
  while pos<len(w):
   if w[pos:].startswith(b'SSH-'):
    end=w.find(b'\n',pos);n=end-pos+1 if end>=pos else 0
   elif len(w)-pos>=16:n=4+int.from_bytes(w[pos:pos+4],'big')
   else:break
   if n<=0 or pos+n>len(w):break
   reference[digest(w[pos:pos+n])].append((int(l['frame']['frame.number']),view));pos+=n
class Reader:
 def __init__(self,b):self.b=b;self.p=0
 def u32(self):
  check(self.p+4<=len(self.b),'complete independent uint32');v=int.from_bytes(self.b[self.p:self.p+4],'big');self.p+=4;return v
 def string(self):
  n=self.u32();check(n<=len(self.b)-self.p,'complete independent SSH string');v=self.b[self.p:self.p+n];self.p+=n;return v
 def done(self):eq(self.p,len(self.b),'all mandatory bytes consumed exactly; no hidden trailing fields')
def b64(w):return base64.b64encode(w).decode()
def positive(w):check(bool(w)and w[0]<128 and(w[0]!=0 or len(w)>1 and w[1]>=128),'canonical positive SSH mpint')
messages=[];profiles=collections.Counter();unselected=[];opaque_keys=[];typed_edwards_keys=[];reference_opaque_signatures=[];generic_packets=[]
for line in a.events.open(encoding='utf-8-sig'):
 e=json.loads(line);s=e.get('session')or{};tree=s.get('Initial Packet Fields')
 if e['protocol']!='ssh'or e.get('entry')!='SSHPacket'or e['status']not in['decoded','deferred']:continue
 raw=base64.b64decode(e['raw']);frames={r['Number']for r in e['source_bytes']['PacketRefs']};src,sport=e['source'].rsplit(':',1);dst,dport=e['destination'].rsplit(':',1);endpoint=(ipaddress.ip_address(src.strip('[]')),int(sport),ipaddress.ip_address(dst.strip('[]')),int(dport),e['domain']['Interface']);candidates=[(f,v)for f,v in reference[digest(raw)]if reference_endpoints[f]==endpoint];direct=[(f,v)for f,v in candidates if f in frames]
 if not direct and candidates:
  check(wire_certificate is not None,'a reference trigger frame outside byte contributors requires independent PCAP/source proof');check(e['domain']['Section']==0 and e['domain']['Encapsulation']=='','selected classic-IP fallback never crosses a tunnel/capture-section domain');attribution_differences.append({'PDU':e['id'],'native_byte_contributors':sorted(frames),'reference_reassembly_trigger_frames':sorted({f for f,v in candidates}),'literal_bytes_and_exact_direction_match':True});direct=candidates
 views=[v for f,v in direct];check(bool(views),('exact PDU bytes, connection, direction and captured interface match a reference view',e['id']))
 if not tree:
  n=int.from_bytes(raw[:4],'big');pad=raw[4];eq(n+4,len(raw),'complete generic packet boundary');check(len(raw)%8==0 and 4<=pad<=len(raw)-6,'generic padding and block bounds');body=raw[5:-pad];code=body[0];r=Reader(body[1:]);payload={'Message Number':code}
  if code==20:
   check(len(body)>=17,'entire literal KEXINIT cookie');payload['Cookie']=b64(body[1:17]);r=Reader(body[17:])
   for name in['Kex Algorithms','Host Key Algorithms','Encryption C2S','Encryption S2C','MAC C2S','MAC S2C','Compression C2S','Compression S2C','Languages C2S','Languages S2C']:
    value=r.string();payload[name+' Length']=len(value);payload[name]=value.decode('ascii')
   check(r.p+5==len(r.b),'entire KEXINIT flag and reserved word');payload['First Kex Packet Follows']=r.b[r.p];r.p+=1;payload['Reserved']=r.u32()
  elif code!=21:payload['Opaque Message Data']=b64(body[1:]);r.p=len(r.b)
  r.done();expected={'Packet Length':n,'Padding Length':pad,'Payload':payload,'Padding':b64(raw[-pad:])};eq(e['fields'],expected,('complete generic fields without an invented exchange layout',e['id']));wire_fields+=leaf_count(expected)
  for field,value in{'ssh.packet_length':n,'ssh.padding_length':pad,'ssh.message_code':code}.items():check(value in[int(v)for o in views for v in leaves(o,field)],('generic reference envelope field',e['id'],field));ref_fields+=1
  generic_packets.append({'PDU':e['id'],'message':code,'physical_frames':sorted(frames),'source':e['source'],'destination':e['destination'],'capture_domain':e['domain'],'raw_sha256':digest(raw),'field_tree_sha256':field_digest(expected),'layout':'KEXINIT'if code==20 else'NEWKEYS'if code==21 else'opaque method context'})
  continue
 offered_methods={v for o in views for v in methods(o)};eq(offered_methods,{s['Kex Algorithm']},'method layout independently agrees with the exact reference negotiation')
 n=int.from_bytes(raw[:4],'big');pad=raw[4];eq(n+4,len(raw),'complete independent packet length');check(len(raw)%8==0 and 4<=pad<=len(raw)-6,'initial block and padding bounds');body=raw[5:len(raw)-pad];code=body[0];r=Reader(body[1:]);payload={'Message Number':code};expected_ref={'ssh.packet_length':n,'ssh.padding_length':pad,'ssh.padding_string':raw[-pad:],'ssh.message_code':code};method=next(iter(offered_methods))
 def item(name,field,mpint=False):
  w=r.string()
  if mpint:positive(w)
  payload[name+' Length']=len(w);payload[name]=b64(w);expected_ref[field]=w
  if name in['Client Point','Server Point']and method=='ecdh-sha2-nistp256':
   eq(len(w),65,'uncompressed P-256 point length');eq(w[0],4,'uncompressed P-256 point format');payload[name]={name+' Format':w[0],name+' X':b64(w[1:33]),name+' Y':b64(w[33:65])}
  return w
 gex=method.startswith('diffie-hellman-group-exchange-');ecdh=method.startswith(('ecdh-','curve25519-'));curve=method.startswith('curve25519-')
 if code==34:
  eq(gex,True,'GEX request only in selected GEX context')
  for name,field in[('Minimum Group Bits','ssh.dh_gex.min'),('Preferred Group Bits','ssh.dh_gex.nbits'),('Maximum Group Bits','ssh.dh_gex.max')]:payload[name]=r.u32();expected_ref[field]=payload[name]
 elif code==31 and gex:item('Group Prime','ssh.dh_gex.p',True);item('Group Generator','ssh.dh_gex.g',True)
 elif code in[30,32]:
  if code==30 and gex:payload['Preferred Group Bits']=r.u32();expected_ref['ssh.dh_gex.nbits']=payload['Preferred Group Bits']
  else:item('Client Curve25519 Public Key'if curve else'Client Point'if ecdh else'Client Exchange Value','ssh.ecdh.q_c'if ecdh else'ssh.dh.e',not ecdh)
 elif code in[31,33]:
  host=r.string();payload['Host Key Length']=len(host);expected_ref['ssh.host_key.length']=len(host);h=Reader(host);kind=h.string().decode('ascii');expected_ref['ssh.host_key.type']=kind;expected_ref['ssh.host_key.type_length']=len(kind);key={'Host Key Format Length':len(kind),'Host Key Format':kind}
  if kind=='ssh-rsa':
   for name,field in[('RSA Exponent','ssh.host_key.rsa.e'),('RSA Modulus','ssh.host_key.rsa.n')]:
    w=h.string();positive(w);key[name+' Length']=len(w);key[name]=b64(w);expected_ref[field]=w
  elif kind in['ssh-ed25519','ssh-ed448']:
   name,size=('Ed25519',32)if kind=='ssh-ed25519'else('Ed448',57);w=h.string();eq(len(w),size,'RFC8709 '+name+' key length');key[name+' Public Key Length']=len(w);key[name+' Public Key']=b64(w);expected_ref['ssh.host_key.eddsa.key']=w;expected_ref['ssh.host_key.eddsa.key_length']=len(w);typed_edwards_keys.append({'PDU':e['id'],'host_key':kind,'key_bytes':len(w),'key_sha256':digest(w)})
  else:
   key['Opaque Host Key Data']=b64(host[h.p:]);opaque_keys.append({'PDU':e['id'],'host_key':kind});unselected.append({'PDU':e['id'],'host_key':kind});h.p=len(host)
  h.done();payload['Host Key']=key;item('Server Curve25519 Public Key'if curve else'Server Point'if ecdh else'Server Exchange Value','ssh.ecdh.q_s'if ecdh else'ssh.dh.f',not ecdh)
  sig=r.string();payload['Signature Length']=len(sig);expected_ref['ssh.host_sig.length']=len(sig);sr=Reader(sig);sig_type=sr.string().decode('ascii');opaque_sig=sig[sr.p:];sig_bytes=sr.string();sr.done();expected_ref.update({'ssh.host_sig.type':sig_type,'ssh.host_sig.type_length':len(sig_type)});signature={'Signature Format Length':len(sig_type),'Signature Format':sig_type}
  if sig_type in['ssh-rsa','rsa-sha2-256','rsa-sha2-512']:
   signature.update({'RSA Signature Bytes Length':len(sig_bytes),'RSA Signature Bytes':b64(sig_bytes)})
   if sig_type=='ssh-rsa':expected_ref['ssh.host_sig.rsa']=sig_bytes
   else:expected_ref['ssh.host_sig.data']=opaque_sig;reference_opaque_signatures.append({'PDU':e['id'],'signature_format':sig_type,'reference_field':'ssh.host_sig.data','native_inner_bytes_independently_decoded':True})
  elif sig_type in['ssh-ed25519','ssh-ed448']:
   name,size=('Ed25519',64)if sig_type=='ssh-ed25519'else('Ed448',114);eq(len(sig_bytes),size,'RFC8709 '+name+' signature length');signature.update({name+' Signature Bytes Length':len(sig_bytes),name+' Signature Bytes':b64(sig_bytes)});expected_ref['ssh.host_sig.data']=opaque_sig;reference_opaque_signatures.append({'PDU':e['id'],'signature_format':sig_type,'reference_field':'ssh.host_sig.data','native_inner_bytes_independently_decoded':True})
  else:
   signature['Opaque Signature Data']=b64(opaque_sig)
   expected_ref['ssh.host_sig.data']=opaque_sig
  payload['Signature']=signature
  eq(s.get('Host Key SHA256'),digest(host),'exact literal public host key digest');eq(s.get('Signature Verified'),False,'signature bytes are not signature verification');eq(s.get('Peer Identity Verified'),False,'public key bytes do not verify the peer')
 else:check(False,('unselected initial method code',e['id'],code))
 r.done();expected={'Packet Length':n,'Padding Length':pad,'Payload':payload,'Padding':b64(raw[-pad:])};eq(tree,expected,('entire independently decoded initial packet tree',e['id']));eq(e['fields'],expected,('lazy bin-parser fields and native context expose the same literal tree',e['id']));wire_fields+=leaf_count(expected)
 for field,value in expected_ref.items():
  vals=[v for o in views for v in leaves(o,field)]
  if isinstance(value,bytes):vals=[bytes.fromhex(v.replace(':',''))for v in vals]
  elif isinstance(value,int):vals=[int(v)for v in vals]
  check(value in vals,('literal reference field',e['id'],field));ref_fields+=1
 profiles[method]+=1;messages.append({'PDU':e['id'],'message':code,'physical_frames':sorted(frames),'source':e['source'],'destination':e['destination'],'capture_domain':e['domain'],'raw_sha256':digest(raw),'field_tree_sha256':field_digest(expected)})
out={'scope':__doc__,'checks':checks,'complete_method_PDUs':len(messages),'complete_generic_packet_PDUs':len(generic_packets),'method_profiles':dict(profiles),'native_literal_tree_fields_compared':wire_fields,'reference_fields_compared':ref_fields,'native_opaque_host_key_views':opaque_keys,'native_typed_edwards_host_key_views':typed_edwards_keys,'reference_opaque_signature_views':reference_opaque_signatures,'unselected_host_key_views':unselected,'unselected_reference_carriers':unselected_reference_carriers,'reference_attribution_differences':attribution_differences,'independent_wire_certificate_sha256':digest(a.wire_certificate.read_bytes())if a.wire_certificate else None,'compared_PDUs':messages,'generic_PDUs':generic_packets,'events_sha256':digest(a.events.read_bytes()),'oracle_sha256':digest(a.oracle.read_bytes())};a.out.parent.mkdir(parents=True,exist_ok=True);a.out.write_text(json.dumps(out,indent=2)+'\n');print(json.dumps({k:v for k,v in out.items()if k not in['scope','compared_PDUs','generic_PDUs']}))
