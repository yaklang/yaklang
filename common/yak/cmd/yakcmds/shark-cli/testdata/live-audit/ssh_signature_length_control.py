"""Declare one malformed-length control from an actual owned SSH capture.

Change only a complete initial Ed25519 signature's inner string length from
64 to 4, retaining every captured octet and outer packet/string length. Update
the TCP checksum. This is a counterfactual replay control, not peer acceptance
or an actual network attack. The original capture is never rewritten.
"""
import argparse,hashlib,ipaddress,json,struct
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
ap.add_argument('--capture',type=Path,required=True);ap.add_argument('--out',type=Path,required=True);a=ap.parse_args()
for path in [a.capture,a.out]:assert not any((p/'.git').exists()for p in[path.resolve(),*path.resolve().parents]),'captures and detailed proofs stay outside Git'
assert not a.out.exists();a.out.mkdir(parents=True)
w=a.capture.read_bytes();assert w[:4]in[b'\xd4\xc3\xb2\xa1',b'\x4d\x3c\xb2\xa1'];link=struct.unpack_from('<I',w,20)[0];assert link in [0,1];ip=4 if link==0 else 14
out=bytearray(w);at=24;number=0;change=None
def digest(data):return hashlib.sha256(data).hexdigest()
def checksum(data):
 if len(data)%2:data+=b'\0'
 n=sum(int.from_bytes(data[i:i+2],'big')for i in range(0,len(data),2))
 while n>>16:n=(n&65535)+(n>>16)
 return n^65535
while at<len(w):
 assert at+16<=len(w);_,_,n,wire=struct.unpack_from('<IIII',w,at);at+=16;assert 0<n<=wire and at+n<=len(w);start=at;b=w[at:at+n];at+=n;number+=1
 if change is not None or len(b)<ip+20 or b[ip]>>4!=4 or b[ip+9]!=6:continue
 ihl=(b[ip]&15)*4;t=ip+ihl
 if ihl<20 or len(b)<t+20 or int.from_bytes(b[ip+6:ip+8],'big')&0x3fff:continue
 size=(b[t+12]>>4)*4;end=ip+int.from_bytes(b[ip+2:ip+4],'big');payload=t+size
 if size<20 or end>len(b)or payload+16>end:continue
 raw=b[payload:end]
 if len(raw)%8 or int.from_bytes(raw[:4],'big')+4!=len(raw)or raw[5]!=31 or raw[4]<4:continue
 pos=6;body_end=len(raw)-raw[4]
 def string():
  global pos
  assert pos+4<=body_end;n=int.from_bytes(raw[pos:pos+4],'big');pos+=4;assert n<=body_end-pos;value=raw[pos:pos+n];pos+=n;return value
 try:
  host=string();exchange=string();sig_start=pos+4;sig=string();assert pos==body_end
  k=int.from_bytes(host[:4],'big');assert host[4:4+k]==b'ssh-ed25519'and int.from_bytes(host[4+k:8+k],'big')==32 and len(host)==8+k+32
  s=int.from_bytes(sig[:4],'big');assert sig[4:4+s]==b'ssh-ed25519'and int.from_bytes(sig[4+s:8+s],'big')==64 and len(sig)==8+s+64
 except AssertionError:continue
 length_at=payload+sig_start+4+s;mutated=bytearray(b);mutated[length_at:length_at+4]=(4).to_bytes(4,'big');mutated[t+16:t+18]=b'\0\0';pseudo=bytes(mutated[ip+12:ip+20])+b'\0\x06'+(end-t).to_bytes(2,'big');mutated[t+16:t+18]=checksum(pseudo+bytes(mutated[t:end])).to_bytes(2,'big');assert checksum(pseudo+bytes(mutated[t:end]))==0
 assert {i for i,(old,new)in enumerate(zip(b,mutated))if old!=new}<={length_at,length_at+1,length_at+2,length_at+3,t+16,t+17};out[start:start+n]=mutated
 change={'physical_frame':number,'source':str(ipaddress.ip_address(b[ip+12:ip+16]))+':'+str(int.from_bytes(b[t:t+2],'big')),'destination':str(ipaddress.ip_address(b[ip+16:ip+20]))+':'+str(int.from_bytes(b[t+2:t+4],'big')),'signature_string_length_before':64,'signature_string_length_after':4,'frame_length_unchanged':len(b),'inner_length_frame_offset':length_at,'TCP_checksum_frame_offset':t+16,'original_frame_sha256':digest(b),'mutated_frame_sha256':digest(mutated),'original_PDU_sha256':digest(raw),'mutated_PDU_sha256':digest(mutated[payload:end]),'enclosing_lengths_payload_and_packet_order_unchanged':True}
assert change,'one complete actual initial Ed25519 reply required'
(a.out/'malformed-signature.pcap').write_bytes(out)
result={'scope':__doc__,'capture_sha256':digest(w),'control_sha256':digest(out),'physical_frames':number,'changed_frame_count':1,'change':change,'generator_sha256':digest(Path(__file__).read_bytes())};(a.out/'manifest.json').write_text(json.dumps(result,indent=2)+'\n');print(json.dumps(result))
