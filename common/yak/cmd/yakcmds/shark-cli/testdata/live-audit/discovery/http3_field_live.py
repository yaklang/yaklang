"""Declared HTTP3 malformed-field controls over real aioquic/Windows UDP.
The raw QUIC peers intentionally do not enforce HTTP semantics. Transport receipt
and exact byte hashes are the oracle, not claims of a conforming HTTP server.
Uses no Yak parser. PCAP, certificates, keys and full manifests stay outside Git.
Usage: http3_field_live.py OUT TSHARK AIOQUIC_LIBS [DECLARED_SCENARIOS_JSON]
"""
import collections,datetime,hashlib,importlib.metadata,json,queue,secrets,select,socket,subprocess,sys,threading,time
from pathlib import Path
sys.path.insert(0,sys.argv[3])
from aioquic.buffer import Buffer
from aioquic.quic.configuration import QuicConfiguration
from aioquic.quic.connection import QuicConnection
from aioquic.quic.events import HandshakeCompleted,StreamDataReceived,ConnectionTerminated
from aioquic.quic.packet import pull_quic_header,encode_quic_retry,QuicProtocolVersion
from aioquic.quic.retry import QuicRetryTokenHandler
from aioquic.quic import crypto as peer_crypto
from aioquic.tls import CipherSuite,Epoch
from cryptography import x509
from cryptography.hazmat.primitives import hashes,serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.x509.oid import NameOID

out=Path(sys.argv[1]);assert not any((p/'.git').exists() for p in [out.resolve(),*out.resolve().parents]);out.mkdir(exist_ok=False)
key=ec.generate_private_key(ec.SECP256R1());name=x509.Name([x509.NameAttribute(NameOID.COMMON_NAME,'field-audit.invalid')]);now=datetime.datetime.now(datetime.timezone.utc)
cert=x509.CertificateBuilder().subject_name(name).issuer_name(name).public_key(key.public_key()).serial_number(x509.random_serial_number()).not_valid_before(now-datetime.timedelta(minutes=1)).not_valid_after(now+datetime.timedelta(days=1)).add_extension(x509.SubjectAlternativeName([x509.DNSName('field-audit.invalid')]),False).sign(key,hashes.SHA256())
(out/'cert.pem').write_bytes(cert.public_bytes(serialization.Encoding.PEM));(out/'key.pem').write_bytes(key.private_bytes(serialization.Encoding.PEM,serialization.PrivateFormat.PKCS8,serialization.NoEncryption()))
keys=(out/'session.keys').open('w',encoding='ascii');network=collections.Counter();operations=[];profiles=[]
server_socket=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);server_socket.bind(('127.0.0.1',0));server_socket.setblocking(False)
client_socket=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);client_socket.bind(('127.0.0.1',0));client_socket.setblocking(False)
server_address=server_socket.getsockname();client_address=client_socket.getsockname()
capture=subprocess.Popen([sys.argv[2],'-n','-i',r'\Device\NPF_Loopback','-f',f'udp port {server_address[1]}','-F','pcap','-w',str(out/'live.pcap'),'-P','-l','-T','fields','-e','udp.payload','-a','duration:20'],stdout=subprocess.PIPE,stderr=subprocess.PIPE,creationflags=0x08000000)
capture_lines=[];capture_errors=[];observed=queue.Queue()
def drain(pipe,storage,report=False):
 for line in iter(pipe.readline,b''):
  storage.append(line)
  if report:observed.put(line)
readers=[threading.Thread(target=drain,args=(capture.stdout,capture_lines,True),daemon=True),threading.Thread(target=drain,args=(capture.stderr,capture_errors),daemon=True)]
for reader in readers:reader.start()

def vi(n):
 size=1 if n<64 else 2 if n<16384 else 4
 return (n|((size.bit_length()-1)<<(8*size-2))).to_bytes(size,'big')

def pref(n,bits,first=0):
 cap=(1<<bits)-1
 if n<cap:return bytes([first|n])
 w=bytearray([first|cap]);n-=cap
 while n>=128:w.append((n&127)|128);n>>=7
 return bytes(w)+bytes([n])

def frame(typ,payload):return vi(typ)+vi(len(payload))+payload

def headers(hs):
 w=b'\0\0'
 for name,value in hs:
  n,v=name.encode('ascii'),value.encode('ascii');w+=pref(len(n),3,0x20)+n+pref(len(v),7)+v
 return frame(1,w)

base=[(':method','GET'),(':scheme','https'),(':path','/'),(':authority','field-audit.invalid')]
cases=[('valid-GET',base,'base'),('missing-path',[h for h in base if h[0]!=':path'],'malformed'),('missing-method',base[1:],'malformed'),('duplicate-method',base+[base[0]],'malformed'),('request-status',base+[(':status','200')],'malformed'),('pseudo-after-field',[('x-test','ok')]+base,'malformed'),('unknown-pseudo',base+[(':unknown','x')],'unsupported'),('uppercase',base+[('X-Test','ok')],'malformed'),('invalid-token',base+[('x(test','ok')],'malformed'),('control-value',base+[('x-test','a\x01b')],'malformed'),('edge-whitespace',base+[('x-test',' leading')],'malformed'),('connection',base+[('connection','close')],'malformed'),('TE-gzip',base+[('te','gzip')],'malformed'),('CONNECT-with-path',[(':method','CONNECT'),(':authority','example.com:443'),(':path','')],'malformed'),('CONNECT-without-port',[(':method','CONNECT'),(':authority','example.com')],'malformed'),('valid-CONNECT',[(':method','CONNECT'),(':authority','example.com:443')],'base'),('valid-OPTIONS',[(':method','OPTIONS'),(':scheme','https'),(':path','*'),(':authority','field-audit.invalid')],'base'),('missing-path-large',[h for h in base if h[0]!=':path']+[('x-pad','p'*18000)],'malformed')]
plans={};transport_controls={}
if len(sys.argv)>4:
 selected=json.loads(Path(sys.argv[4]).read_text(encoding='utf-8-sig'));plans={p['name']:p for p in selected['cases']};assert len(plans)==len(selected['cases'])
 cases=[(p['name'],p['request_headers'],p['request_expect']) for p in selected['cases']]
 transport_controls=selected.get('transport_controls',{})
cid_lengths=transport_controls.get('connection_id_lengths',[8,8,8]);assert len(cid_lengths)==3 and all(type(n)is int and 0<=n<=20 for n in cid_lengths)
wire_versions=transport_controls.get('wire_versions',[1,1,1]);assert len(wire_versions)==3 and all(v in[1,0x6b3343cf]for v in wire_versions)
suite_ids=transport_controls.get('cipher_suites',[0x1301,0x1301,0x1301]);assert len(suite_ids)==3 and all(v in[0x1301,0x1302,0x1303]for v in suite_ids)
key_followup=transport_controls.get('client_key_update_followup',False);assert type(key_followup)is bool
retry_padding=transport_controls.get('retry_token_padding_bytes',[0,0,0]);assert len(retry_padding)==3 and all(type(n)is int and 0<=n<=512 for n in retry_padding)
if transport_controls.get('v2_key_update_policy')=='rfc9369-label':
 original_next_key_phase=peer_crypto.next_key_phase
 def declared_v2_next_key_phase(context):
  if context.version!=QuicProtocolVersion.VERSION_2:return original_next_key_phase(context)
  algorithm=peer_crypto.cipher_suite_hash(context.cipher_suite);crypto=peer_crypto.CryptoContext(key_phase=int(not context.key_phase))
  crypto.setup(cipher_suite=context.cipher_suite,secret=peer_crypto.hkdf_expand_label(algorithm,context.secret,b'quicv2 ku',b'',algorithm.digest_size),version=context.version)
  return crypto
 # aioquic1.7 uses the v1 ku label for v2. This explicitly declared peer
 # control changes only the update label; the installed package is untouched.
 peer_crypto.next_key_phase=declared_v2_next_key_phase
generator_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest()
class ZeroCIDControlConnection(QuicConnection):
 # aioquic1.7 replenishes zero-byte IDs into illegal NEW_CONNECTION_ID frames.
 # An explicitly declared test peer may suppress issuance; its zero-length
 # header CID remains valid. This is not an unmodified production-server claim.
 def __init__(self,*args,**kwargs):
  super().__init__(*args,**kwargs)
  if self._is_client and self._configuration.connection_id_length==0:
   # RFC9000 client Initial DCID still needs at least8 octets, independently
   # of a zero-length local/source CID. aioquic1.7 otherwise generates0 here.
   self._peer_cid.cid=secrets.token_bytes(8)
   self._original_destination_connection_id=self._peer_cid.cid
 def _replenish_connection_ids(self):
  if self._configuration.connection_id_length==0:return
  return super()._replenish_connection_ids()
PeerConnection=ZeroCIDControlConnection if transport_controls.get('zero_cid_policy')=='omit-new-connection-id'else QuicConnection
if transport_controls.get('retry_client_policy')=='preserve-first-client-hello':
 class RetryTranscriptControlConnection(PeerConnection):
  # aioquic1.7 restarts TLS on Retry, generating a different ClientHello.
  # This declared positive control preserves the first transcript/private
  # TLS state, while resetting QUIC protection and resending the same bytes.
  def _connect(self,now):
   if not self._retry_count:return super()._connect(now)
   initial=self._crypto_streams[Epoch.INITIAL].sender
   assert initial._buffer_start==0 and len(initial._buffer)>=4
   first_tls,first_hello=self.tls,bytes(initial._buffer)
   self._close_at=now+self._idle_timeout();self._initialize(self._peer_cid.cid)
   self.tls=first_tls;self._crypto_buffers[Epoch.INITIAL].push_bytes(first_hello);self._push_crypto_data()
 PeerConnection=RetryTranscriptControlConnection
retry_handler=QuicRetryTokenHandler()if transport_controls.get('server_retry')else None
try:
 # A fixed startup sleep can silently miss the first complete connection.
 # Wait until TShark actually observes our nonce before sending any QUIC.
 probe=b'pcap-ready-'+secrets.token_hex(8).encode();deadline=time.monotonic()+8;ready=False
 while time.monotonic()<deadline and not ready:
  client_socket.sendto(probe,server_address)
  try:ready=probe.hex().encode() in observed.get(timeout=.1).replace(b':',b'')
  except queue.Empty:pass
 assert ready,'capture did not observe readiness probe'
 while True:
  try:server_socket.recvfrom(65535)
  except BlockingIOError:break
 for epoch in range(3):
  cfg=QuicConfiguration(is_client=True,alpn_protocols=['h3'],cipher_suites=[CipherSuite(suite_ids[epoch])],server_name='field-audit.invalid',secrets_log_file=keys,supported_versions=[QuicProtocolVersion(wire_versions[epoch])],connection_id_length=cid_lengths[epoch]);cfg.load_verify_locations(cafile=str(out/'cert.pem'))
  scfg=QuicConfiguration(is_client=False,alpn_protocols=['h3'],cipher_suites=[CipherSuite(suite_ids[epoch])],secrets_log_file=keys,supported_versions=[QuicProtocolVersion(wire_versions[epoch])],connection_id_length=cid_lengths[epoch]);scfg.load_cert_chain(str(out/'cert.pem'),str(out/'key.pem'))
  client=PeerConnection(configuration=cfg);client.connect(server_address,now=time.monotonic());server=None;handshake=[False,False];delayed=[];packets=[0,0];dropped=[0,0];received=[collections.defaultdict(bytearray),collections.defaultdict(bytearray)];ended=[set(),set()];expected={};retry_state={'sent':0,'token_envelopes':{}}
  def events(q,side):
   while True:
    e=q.next_event()
    if e is None:break
    if isinstance(e,HandshakeCompleted):handshake[side]=True
    if isinstance(e,ConnectionTerminated) and e.error_code:raise AssertionError(('QUIC terminated',side,e.error_code,e.reason_phrase))
    if isinstance(e,StreamDataReceived):
     received[side][e.stream_id]+=e.data
     if e.end_stream:
      ended[side].add(e.stream_id)
      if side==1:
       assert bytes(received[1][e.stream_id])==expected[e.stream_id]['wire']
       reply=expected[e.stream_id].get('reply_wire',headers([(':status','200')])+frame(0,expected[e.stream_id]['name'].encode()))
       expected[e.stream_id]['reply']=reply;q.send_stream_data(e.stream_id,reply,end_stream=True)
  def send(q,sock,direction):
   for data,address in q.datagrams_to_send(now=time.monotonic()):
    packets[direction]+=1;short=not data[0]&128
    if epoch and short and packets[direction]%13==7 and dropped[direction]<2:
     dropped[direction]+=1;network['dropped']+=1;continue
    if epoch and short and packets[direction]%7==4:
     delayed.append((time.monotonic()+.025,sock,data,address));network['delayed']+=1
    else:sock.sendto(data,address)
  def pump(until):
   global server
   deadline=time.monotonic()+5
   while not until():
    now=time.monotonic();assert now<deadline,('deadline',epoch,handshake,sorted(ended[0]))
    for q in [client,server]:
     if q is not None:
      timer=q.get_timer()
      if timer is not None and timer<=now:q.handle_timer(now=now)
    send(client,client_socket,0)
    if server is not None:send(server,server_socket,1)
    for item in list(delayed):
     when,sock,data,address=item
     if when<=now:sock.sendto(data,address);delayed.remove(item)
    ready,_,_=select.select([client_socket,server_socket],[],[],.001)
    for sock in ready:
     data,address=sock.recvfrom(65535)
     if sock is server_socket:
      if server is None:
       h=pull_quic_header(Buffer(data=data),host_cid_length=cid_lengths[epoch]);original,retry_cid=h.destination_cid,None
       if retry_handler is not None:
        if not h.token:
         retry_cid=secrets.token_bytes(8);token=retry_handler.create_token(address=address,original_destination_connection_id=h.destination_cid,retry_source_connection_id=retry_cid,now=time.monotonic())
         if retry_padding[epoch]:
          # Declared opaque-token envelope. Only an exactly issued envelope
          # maps back to the original token, which still passes the real
          # handler's address/CID validation; arbitrary suffixes are rejected.
          inner=token;token+=secrets.token_bytes(retry_padding[epoch]);retry_state['token_envelopes'][token]=inner
         server_socket.sendto(encode_quic_retry(version=h.version,source_cid=retry_cid,destination_cid=h.source_cid,original_destination_cid=h.destination_cid,retry_token=token),address);retry_state['sent']+=1;network['retry_packets']+=1;continue
        inner=retry_state['token_envelopes'][h.token]if retry_padding[epoch]else h.token
        original=retry_handler.validate_token(address=address,destination_cid=h.destination_cid,token=inner,now=time.monotonic());retry_cid=h.destination_cid
       server=PeerConnection(configuration=scfg,original_destination_connection_id=original,retry_source_connection_id=retry_cid)
      server.receive_datagram(data,address,now=time.monotonic());events(server,1)
     else:client.receive_datagram(data,address,now=time.monotonic());events(client,0)
  pump(lambda:all(handshake))
  client.send_stream_data(client.get_next_available_stream_id(is_unidirectional=True),b'\0'+frame(4,b''))
  server.send_stream_data(server.get_next_available_stream_id(is_unidirectional=True),b'\0'+frame(4,b''))
  def queue(name,hs,status,extra=b''):
   plan=plans.get(name)
   if plan:
    if plan.get('request_data')is not None:extra+=frame(0,plan['request_data'].encode('ascii'))
    if plan.get('request_trailers')is not None:extra+=headers(plan['request_trailers'])
   sid=client.get_next_available_stream_id();w=headers(hs)+extra
   if plan:
    trim=plan.get('request_trim_bytes',0);assert 0<=trim<=len(w)
    if trim:w=w[:-trim]
    if 'request_wire_prefix_bytes'in plan:w=w[:plan['request_wire_prefix_bytes']]
    w+=bytes.fromhex(plan.get('request_append_hex',''))
   expected[sid]={'name':name,'wire':w,'status':status}
   if plan:
    reply=b''.join(headers(h) for h in plan.get('reply_interim_headers',[]))+headers(plan['reply_headers'])
    if plan.get('reply_data')is not None:reply+=frame(0,plan['reply_data'].encode('ascii'))
    trim=plan.get('reply_trim_bytes',0);assert 0<=trim<=len(reply)
    if trim:reply=reply[:-trim]
    if 'reply_wire_prefix_bytes'in plan:reply=reply[:plan['reply_wire_prefix_bytes']]
    reply+=bytes.fromhex(plan.get('reply_append_hex',''))
    expected[sid]['reply_wire']=reply;expected[sid]['plan']=plan
   client.send_stream_data(sid,w,end_stream=True);return sid
  if plans:
   # Deliberately batch the declared scenarios: multiple independent streams
   # can share one protected QUIC packet, including message-error controls.
   pending={queue(name,hs,status) for name,hs,status in cases};pump(lambda:pending<=ended[0])
  else:
   for name,hs,status in cases:
    sid=queue(name,hs,status);pump(lambda:sid in ended[0])
   for name,extra in [('pseudo-in-trailers',headers([(':path','/another-request')])),('third-header-section',headers([('x-trailer','ok')])+headers([('x-trailer','extra')]))]:
    sid=queue(name,base,'malformed',extra);pump(lambda:sid in ended[0])
   # Queue two STREAMs before one packetization pass to exercise a stream-local
   # failure alongside a valid request in the same authenticated QUIC packet.
   bad=queue('batched-bad',[h for h in base if h[0]!=':path'],'malformed');good=queue('batched-good',base,'base');pump(lambda:bad in ended[0] and good in ended[0])
   sid=queue('valid-after-all-failures',base,'base');pump(lambda:sid in ended[0])
  if key_followup:
   client.request_key_update();sid=queue('after-client-key-update',[(k,'/after-client-key-update'if k==':path'else v)for k,v in base],'base');pump(lambda:sid in ended[0])
   assert client._cryptos[Epoch.ONE_RTT].key_phase==1 and server._cryptos[Epoch.ONE_RTT].key_phase==1
  for sid,m in expected.items():
   assert bytes(received[0][sid])==m['reply']
   operations.append({'epoch':epoch,'stream_id':sid,'case':m['name'],'expected_base_scope':m['status'],'request_bytes':len(m['wire']),'request_hex':m['wire'].hex(),'request_sha256':hashlib.sha256(m['wire']).hexdigest(),'reply_hex':m['reply'].hex(),'reply_sha256':hashlib.sha256(m['reply']).hexdigest(),'declared_scenario':m.get('plan')})
  profiles.append({'epoch':epoch,'client_port':client_address[1],'server_port':server_address[1],'packets':packets,'dropped':dropped,'requests':len(expected),'connection_id_length':cid_lengths[epoch],'retry_packets_sent':retry_state['sent'],'wire_version':wire_versions[epoch],'cipher_suite':suite_ids[epoch],'final_key_phases':[client._cryptos[Epoch.ONE_RTT].key_phase,server._cryptos[Epoch.ONE_RTT].key_phase]})
  client.close();send(client,client_socket,0);time.sleep(.08)
  for sock in [client_socket,server_socket]:
   while True:
    try:sock.recvfrom(65535)
    except BlockingIOError:break
  print('Completed raw QUIC epoch',epoch,'with',len(expected),'byte-exact requests and replies',flush=True)
finally:
 keys.close();client_socket.close();server_socket.close();capture.wait(timeout=25)
 for reader in readers:reader.join(timeout=2)
 (out/'capture.stderr').write_bytes(b''.join(capture_errors));(out/'capture-observed.txt').write_bytes(b''.join(capture_lines))
 (out/'manifest.json').write_text(json.dumps({'scope':__doc__,'generator_sha256':generator_sha256,'capture_ready_observed':ready,'aioquic':importlib.metadata.version('aioquic'),'server_port':server_address[1],'client_port':client_address[1],'network':dict(network),'profiles':profiles,'transport_controls':transport_controls,'operations':operations},indent=2)+'\n',encoding='utf8')
assert len(operations)==(len(cases)*3 if plans else 69)+(3 if key_followup else 0)
