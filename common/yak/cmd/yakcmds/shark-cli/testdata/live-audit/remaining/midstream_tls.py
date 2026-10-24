import hashlib,json,struct,subprocess,sys
from pathlib import Path
if len(sys.argv)!=4:raise SystemExit('usage: midstream_tls.py REAL_TLS_PCAP OUTPUT_DIR TSHARK')
r=Path(sys.argv[2]);r.mkdir(exist_ok=True)
source=Path(sys.argv[1]);data=source.read_bytes()
assert data[:4]==bytes.fromhex('4d3cb2a1')
def parse(b):
 ip=0 if int.from_bytes(data[20:24],'little') in (12,101) else 14;assert b[ip]>>4==4
 tcp=ip+4*(b[ip]&15);off=tcp+4*(b[tcp+12]>>4)
 return ip,tcp,off,b[off:]
frames=[];at=24
while at<len(data):
 sec,ns,n,wire=struct.unpack_from('<IIII',data,at);at+=16;b=data[at:at+n];at+=n
 if len(b)<54:continue
 try:ip,tcp,off,p=parse(b)
 except:continue
 if b[ip+9]==6 and p[:3]==b'\x17\x03\x03' and len(p)>=5 and 5+int.from_bytes(p[3:5],'big')<=len(p):frames.append((sec,ns,b))
assert frames
sec,ns,b=frames[-1];ip,tcp,off,p=parse(b)
def checksum(b):
 if len(b)%2:b+=b'\0'
 n=sum(struct.unpack('!%dH'%(len(b)//2),b))
 while n>>16:n=(n&65535)+(n>>16)
 return (~n)&65535
def edit(port=None,trim=0):
 x=bytearray(b)
 if port is not None:
  if struct.unpack_from('!H',x,tcp)[0]==443:struct.pack_into('!H',x,tcp,port)
  else:struct.pack_into('!H',x,tcp+2,port)
 if trim:
  seq=struct.unpack_from('!I',x,tcp+4)[0];struct.pack_into('!I',x,tcp+4,(seq+trim)&0xffffffff)
  x=x[:off]+x[off+trim:];struct.pack_into('!H',x,ip+2,len(x)-ip)
 x[ip+10:ip+12]=b'\0\0';struct.pack_into('!H',x,ip+10,checksum(bytes(x[ip:tcp])))
 x[tcp+16:tcp+18]=b'\0\0';pseudo=bytes(x[ip+12:ip+20])+b'\0\x06'+struct.pack('!H',len(x)-tcp);struct.pack_into('!H',x,tcp+16,checksum(pseudo+bytes(x[tcp:])))
 return bytes(x)
tshark=Path(sys.argv[3])
(r/'source.json').write_text(json.dumps(dict(path=str(source),sha256=hashlib.sha256(data).hexdigest(),keys='omitted',handshakes='omitted'),indent=2))
result=[]
for name,wire,args in [('443-intact-record',edit(),[]),('nonstandard-intact-record',edit(18447),[]),('nonstandard-decode-as',edit(18447),['-d','tcp.port==18447,tls']),('443-missing-record-header',edit(trim=5),[])]:
 cap=r/(name+'.pcap');cap.write_bytes(data[:24]+struct.pack('<IIII',sec,ns,len(wire),len(wire))+wire)
 cmd=[str(tshark),'-n','-r',str(cap),*args,'-T','fields','-e','frame.number','-e','_ws.col.Protocol','-e','_ws.col.Info','-e','tls.record.content_type','-e','tls.record.version','-e','tls.record.length','-e','tls.handshake.type']
 q=subprocess.run(cmd,stdout=subprocess.PIPE,stderr=subprocess.PIPE,check=True)
 text=q.stdout.decode().strip();(r/(name+'.tsv')).write_text(text,encoding='utf8');result.append(dict(case=name,output=text))
(r/'result.json').write_text(json.dumps(result,indent=2),encoding='utf8');print(json.dumps(result,indent=2))
