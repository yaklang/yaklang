"""Independent aioquic 1.2.0 native UDP H3/DoQ fixtures; capture separately."""
import asyncio, datetime, json, pathlib, ssl, struct, sys
from aioquic.asyncio import QuicConnectionProtocol, connect, serve
from aioquic.quic.configuration import QuicConfiguration
from aioquic.quic.events import StreamDataReceived, ProtocolNegotiated
from aioquic.h3.connection import H3Connection
from aioquic.h3.events import HeadersReceived, DataReceived
from aioquic.tls import CipherSuite
from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import rsa
from cryptography.x509.oid import NameOID

out=pathlib.Path(sys.argv[1]);out.mkdir(parents=True,exist_ok=True)
key=rsa.generate_private_key(public_exponent=65537,key_size=2048)
name=x509.Name([x509.NameAttribute(NameOID.COMMON_NAME,"localhost")])
cert=(x509.CertificateBuilder().subject_name(name).issuer_name(name).public_key(key.public_key()).serial_number(1)
 .not_valid_before(datetime.datetime.now(datetime.timezone.utc)-datetime.timedelta(days=1))
 .not_valid_after(datetime.datetime.now(datetime.timezone.utc)+datetime.timedelta(days=2))
 .add_extension(x509.SubjectAlternativeName([x509.DNSName("localhost")]),critical=False).sign(key,hashes.SHA256()))
(out/'cert.pem').write_bytes(cert.public_bytes(serialization.Encoding.PEM))
(out/'key.pem').write_bytes(key.private_bytes(serialization.Encoding.PEM,serialization.PrivateFormat.PKCS8,serialization.NoEncryption()))
log=(out/'quic.keys').open('w');oracle=[]
class Protocol(QuicConnectionProtocol):
 def __init__(self,*args,**kwargs):
  super().__init__(*args,**kwargs);self.h3=None;self.replies={};self.buffers={};self.done={}
 def quic_event_received(self,event):
  if isinstance(event,ProtocolNegotiated) and event.alpn_protocol=='h3':self.h3=H3Connection(self._quic)
  if self.h3:
   for e in self.h3.handle_event(event):
    if isinstance(e,HeadersReceived):
     if not self._quic.configuration.is_client and e.stream_ended:
      path=dict(e.headers)[b':path'];body=b'm2-h3:'+path
      self.h3.send_headers(e.stream_id,[(b':status',b'200'),(b'content-type',b'text/plain')])
      self.h3.send_data(e.stream_id,body,end_stream=False)
      self.h3.send_headers(e.stream_id,[(b'x-m2-trailer',b'complete')],end_stream=True);self.transmit()
     elif self._quic.configuration.is_client and e.stream_ended:self.done[e.stream_id].set_result(bytes(self.buffers.get(e.stream_id,b'')))
    if isinstance(e,DataReceived):
     self.buffers.setdefault(e.stream_id,bytearray()).extend(e.data)
     if e.stream_ended and e.stream_id in self.done:self.done[e.stream_id].set_result(bytes(self.buffers[e.stream_id]))
  elif isinstance(event,StreamDataReceived):
   self.buffers.setdefault(event.stream_id,bytearray()).extend(event.data)
   if event.end_stream:
    data=bytes(self.buffers[event.stream_id])
    if self._quic.configuration.is_client:self.done[event.stream_id].set_result(data)
    else:
     n=struct.unpack('!H',data[:2])[0];q=data[2:];assert n==len(q) and q[:2]==b'\0\0'
     response=q[:2]+b'\x81\x80'+q[4:6]+b'\0\x01\0\0\0\0'+q[12:]+b'\xc0\x0c\0\x01\0\x01\0\0\0\x3c\0\x04\xc0\0\x02\x0a'
     self._quic.send_stream_data(event.stream_id,struct.pack('!H',len(response))+response,end_stream=True);self.transmit()
async def main():
 for alpn,port in [('h3',14443),('doq',14853)]:
  server=QuicConfiguration(is_client=False,alpn_protocols=[alpn],cipher_suites=[CipherSuite.AES_128_GCM_SHA256]);server.load_cert_chain(str(out/'cert.pem'),str(out/'key.pem'))
  listener=await serve('127.0.0.1',port,configuration=server,create_protocol=Protocol)
  client=QuicConfiguration(is_client=True,alpn_protocols=[alpn],cipher_suites=[CipherSuite.AES_128_GCM_SHA256],server_name='localhost',secrets_log_file=log);client.load_verify_locations(cafile=str(out/'cert.pem'))
  async with connect('127.0.0.1',port,configuration=client,create_protocol=Protocol) as p:
   pending=[]
   for i in range(2):
    sid=p._quic.get_next_available_stream_id();p.done[sid]=asyncio.get_running_loop().create_future()
    if alpn=='h3':p.h3.send_headers(sid,[(b':method',b'GET'),(b':scheme',b'https'),(b':authority',b'localhost'),(b':path',f'/m2/{i}'.encode())],end_stream=True)
    else:
     q=b'\0\0\x01\0\0\x01\0\0\0\0\0\0'+bytes([3])+b'm2x'+bytes([7])+b'example'+b'\0\0\x01\0\x01';wire=struct.pack('!H',len(q))+q;p._quic.send_stream_data(sid,wire[:1]);p.transmit();await asyncio.sleep(.03);p._quic.send_stream_data(sid,wire[1:],end_stream=True)
    p.transmit();pending.append((sid,p.done[sid],i))
   for sid,future,i in pending:
    result=await asyncio.wait_for(future,10)
    if alpn=='h3':assert result==f'm2-h3:/m2/{i}'.encode()
    else:assert result.endswith(b'\xc0\0\x02\x0a')
    oracle.append({'alpn':alpn,'stream_id':sid,'response_hex':result.hex()})
   p._quic.request_key_update();p._quic.send_ping(7);p.transmit();await asyncio.sleep(.15)
  listener.close();await asyncio.sleep(.15)
 log.close();(out/'quic-oracle.json').write_text(json.dumps(oracle,indent=2)+'\n')
asyncio.run(main())
