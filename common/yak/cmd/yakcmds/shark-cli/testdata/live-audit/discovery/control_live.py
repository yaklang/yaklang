"""Scapy-encoded socket wire controls plus actual telnetlib3 negotiation/echo.
These controls are not a complete VPN/router/RPC daemon. No host routes change.
Usage: control_live.py OUTPUT_DIR TSHARK PYTHON_LIBS
"""
import asyncio,base64,hashlib,importlib.metadata,json,secrets,socket,struct,subprocess,sys,time
from pathlib import Path
from capture_ready import start_capture
sys.path.insert(0,sys.argv[3])
from scapy.packet import Raw
from scapy.layers.rip import RIP,RIPEntry
from scapy.layers.netbios import NBTDatagram
from scapy.layers.l2tp import L2TP
from scapy.contrib.ikev2 import IKEv2,IKEv2_SA,IKEv2_Proposal,IKEv2_Transform,IKEv2_KE,IKEv2_Nonce,IKEv2_Notify
from scapy.contrib.oncrpc import RPC,RPC_Call,RPC_Reply
import telnetlib3
out=Path(sys.argv[1]);out.mkdir(exist_ok=False);nonce=secrets.token_hex(12);operations=[]
be=lambda *ns:struct.pack('!'+'I'*len(ns),*ns)
avp=lambda typ,data:struct.pack('!HHH',0x8000|6+len(data),0,typ)+data
ike=bytes(IKEv2(init_SPI=secrets.token_bytes(8),exch_type=34,flags=8)/IKEv2_SA(prop=IKEv2_Proposal(proposal=1,proto=1,trans_nb=1,trans=IKEv2_Transform(transform_type=1,transform_id=12,key_length=128)))/IKEv2_KE(group=14,ke=secrets.token_bytes(256))/IKEv2_Nonce(nonce=secrets.token_bytes(32)))
reply=bytes(IKEv2(init_SPI=ike[:8],resp_SPI=secrets.token_bytes(8),exch_type=34,flags=32)/IKEv2_Notify(type=14))
l2body=avp(0,struct.pack('!H',1))+avp(2,b'\x01\x00')+avp(3,be(3))+avp(7,('audit-'+nonce[:6]).encode())+avp(9,b'\x00\x17')
l2=bytes(L2TP(hdr=0xc80,len=12+len(l2body),tunnel_id=0,session_id=0,ns=0,nr=0)/Raw(l2body))
rpc=bytes(RPC(xid=secrets.randbits(32),mtype=0)/RPC_Call(version=2,program=100000,pversion=2,procedure=0,aflavor=0,alength=0,vflavor=0,vlength=0))
rpcreply=rpc[:4]+be(1,0,0,0,0)
nb=bytes(NBTDatagram(Type=0x10,Flags=2,ID=secrets.randbits(16),SourceIP='127.0.0.1',SourcePort=138,SourceName=b'AUDITCLIENT',DestinationName=b'AUDITSERVER')/Raw(('wire-control-'+nonce).encode()))
ov=bytes([7<<3])+secrets.token_bytes(8)+b'\0'+be(0);ovreply=bytes([8<<3])+secrets.token_bytes(8)+b'\0'+be(0);ovdata=bytes([9<<3])+b'\0\0\1'+secrets.token_bytes(48)
asf=bytes.fromhex('0600ff06000011be80010000')
rmcp=bytes.fromhex('0600ff07061000000000000000002000')+bytes.fromhex('0100000078563412')+bytes.fromhex('000000080100000001000008010000000200000801000000')
messages=[('ike',500,[ike,reply]),('ike-natt',4500,[bytes(4)+ike,bytes(4)+reply]),('rip',520,[bytes(RIP(cmd=1,version=2)/RIPEntry(AF=0,metric=16)),bytes(RIP(cmd=2,version=2)/RIPEntry(AF=2,addr='192.0.2.0',mask='255.255.255.0',metric=2))]),('nbt-dg',138,[nb]),('onc-rpc',111,[rpc,rpcreply]),('l2tp',1701,[l2,bytes(L2TP(hdr=0xc80,len=12,tunnel_id=23,session_id=0,ns=1,nr=1))]),('openvpn',1194,[ov,ovreply,ovdata]),('ipmi',623,[asf,rmcp])]
servers=[];clients=[];bound=[];fallbacks=[]
for name,port,ws in messages:
 s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
 try:s.bind(('127.0.0.1',port))
 except OSError as e:s.bind(('127.0.0.1',0));fallbacks.append(dict(protocol=name,conventional=port,actual=s.getsockname()[1],reason=str(e)))
 s.settimeout(2);servers.append(s);bound.append((name,s.getsockname()[1],ws))
 c=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);c.bind(('127.0.0.1',0));c.settimeout(2);clients.append(c)
messages=bound
async def main():
 async def shell(reader,writer):
  try:
   count=int((await reader.readline()).strip());assert count==4096
   body=(bytes(range(256))*16);writer.write(body);await writer.drain();writer.close()
  except Exception:writer.close();raise
 telnet_server=await telnetlib3.create_server(host='127.0.0.1',port=0,shell=shell,encoding=False,force_binary=True,connect_maxwait=1)
 port=telnet_server.sockets[0].getsockname()[1];telnet_port=port
 async def nbss_shell(reader,writer):
  header=await reader.readexactly(4);size=int.from_bytes(header[2:],'big');body=await reader.readexactly(size);assert header[0]==0x81 and size==68 and body==nb[14:82];writer.write(bytes.fromhex('8200000085000000'));await writer.drain();writer.close()
 nbss_server=await asyncio.start_server(nbss_shell,'127.0.0.1',0);nbss_port=nbss_server.sockets[0].getsockname()[1]
 cap,marker_port=start_capture(sys.argv[2],out/'live.pcap',f'tcp port {port} or tcp port {nbss_port} or ('+' or '.join(f'udp port {p}' for _,p,_ in messages)+')',12)
 try:
  for (name,port,ws),s,c in zip(messages,servers,clients):
   for i,w in enumerate(ws):
    sender,receiver,address=(c,s,s.getsockname()) if i%2==0 else (s,c,c.getsockname())
    sender.sendto(w,address);got,_=receiver.recvfrom(65535);assert got==w
    operations.append(dict(protocol=name,source=sender.getsockname()[1],destination=receiver.getsockname()[1],sha256=hashlib.sha256(w).hexdigest(),wire=base64.b64encode(w).decode(),positive=True))
   for w in [ws[0][:3],('invalid-'+nonce).encode()]:
    c.sendto(w,s.getsockname());assert s.recvfrom(65535)[0]==w;operations.append(dict(protocol=name,source=c.getsockname()[1],destination=s.getsockname()[1],sha256=hashlib.sha256(w).hexdigest(),positive=False))
  for _ in range(2):
   reader,writer=await telnetlib3.open_connection('127.0.0.1',telnet_server.sockets[0].getsockname()[1],encoding=False,force_binary=True,connect_maxwait=1,term='AUDIT',cols=97,rows=43)
   writer.write(b'4096\n');await writer.drain();body=await reader.readexactly(4096);assert body==bytes(range(256))*16;writer.close();await writer.wait_closed();operations.append(dict(protocol='telnet',body_sha256=hashlib.sha256(body).hexdigest(),bytes=len(body)))
  reader,writer=await asyncio.open_connection('127.0.0.1',nbss_port);writer.write(bytes.fromhex('81000044')+nb[14:82]);await writer.drain();assert await reader.readexactly(8)==bytes.fromhex('8200000085000000');writer.close();await writer.wait_closed();operations.append(dict(protocol='nbss',controls=3))
 finally:
  telnet_server.close();await telnet_server.wait_closed()
  nbss_server.close();await nbss_server.wait_closed()
  for s in servers+clients:s.close()
  stdout,stderr=await asyncio.to_thread(cap.communicate,timeout=20);(out/'capture.stderr').write_bytes(stderr)
  (out/'manifest.json').write_text(json.dumps(dict(nonce=nonce,marker_port=marker_port,scapy=importlib.metadata.version('scapy'),telnetlib3=importlib.metadata.version('telnetlib3'),telnet_port=telnet_port,nbss_port=nbss_port,ports={name:p for name,p,_ in messages},fallbacks=fallbacks,operations=operations),indent=2),encoding='utf8')
 print('PASS',len(operations),'wire controls and two real Telnet binary transfers')
asyncio.run(main())
