"""Real Windows OpenSSH clients and independent Paramiko server. No Yak codec.
Usage: ssh_live.py OUTPUT_DIR TSHARK PYTHON_LIBS OPENSSH [--server-first-proxy] [--nonconforming-banner] [--ed25519-host-key]
The optional loopback proxy forwards the actual server banner before forwarding
client data; it does not rewrite SSH bytes or use a Yak codec.
"""
import base64,hashlib,importlib.metadata,json,logging,re,secrets,socket,subprocess,sys,threading,time
from pathlib import Path
from capture_ready import start_capture
sys.path.insert(0,sys.argv[3]);import paramiko
from paramiko.kex_group14 import KexGroup14SHA256
out=Path(sys.argv[1]);assert not any((p/'.git').exists()for p in[out.resolve(),*out.resolve().parents]);out.mkdir(exist_ok=False);nonce=secrets.token_hex(12);generator_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest();server_first='--server-first-proxy'in sys.argv[5:];nonconforming_banner='--nonconforming-banner'in sys.argv[5:]
ed25519_host='--ed25519-host-key'in sys.argv[5:]
if ed25519_host:
 import io
 from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
 from cryptography.hazmat.primitives import serialization
 pem=Ed25519PrivateKey.generate().private_bytes(serialization.Encoding.PEM,serialization.PrivateFormat.OpenSSH,serialization.NoEncryption())
 host=paramiko.Ed25519Key(file_obj=io.StringIO(pem.decode('ascii')))
else:host=paramiko.RSAKey.generate(2048)
host_algorithm='ssh-ed25519'if ed25519_host else'rsa-sha2-256'
client_key=paramiko.RSAKey.generate(2048);client_key.write_private_key_file(str(out/'client.key'))
subprocess.run(['icacls',str(out/'client.key'),'/inheritance:r','/grant:r',__import__('getpass').getuser()+':(R,W)'],stdout=subprocess.PIPE,check=True)
moduli=out/'moduli';moduli.write_text('20261024000000 2 6 100 2047 2 '+format(KexGroup14SHA256.P,'x')+'\n')
kexs=['curve25519-sha256@libssh.org','ecdh-sha2-nistp256','diffie-hellman-group14-sha256','diffie-hellman-group-exchange-sha256','default']
listeners=[];operations=[];errors=[];threads=[];proxy_records=[]
logging.basicConfig(filename=str(out/'paramiko.log'),level=logging.DEBUG)
class Server(paramiko.ServerInterface):
 def check_auth_publickey(self,user,key):return paramiko.AUTH_SUCCESSFUL if user=='audituser' and key==client_key else paramiko.AUTH_FAILED
 def get_allowed_auths(self,user):return 'publickey'
 def check_channel_request(self,kind,chanid):return paramiko.OPEN_SUCCEEDED if kind=='session' else paramiko.OPEN_FAILED_ADMINISTRATIVELY_PROHIBITED
 def check_channel_exec_request(self,channel,command):self.command=command;self.ready.set();return True
def serve(listener,kex):
 try:
  raw,peer=listener.accept();raw.settimeout(15);server_port=raw.getsockname()[1];t=paramiko.Transport(raw)
  if nonconforming_banner:t.local_version='SSH-2.0-Cisco-1.25' # Declared Paramiko emulation, not an actual Cisco device.
  if server_first:t.get_security_options().ciphers=('aes256-ctr','aes128-ctr');t.get_security_options().digests=('hmac-sha2-512','hmac-sha2-256')
  t.add_server_key(host);t.load_server_moduli(str(moduli));server=Server();server.ready=threading.Event();t.start_server(server=server)
  ch=t.accept(10);assert ch is not None and server.ready.wait(10);assert server.command==b'audit-payload'
  body=(f'{nonce}|{kex}|'.encode()*12000)[:131072];ch.sendall(body);ch.send_exit_status(0);ch.shutdown_write();time.sleep(.3);ch.close();operations.append(dict(requested=kex,kex=kex,bytes=len(body),sha256=hashlib.sha256(body).hexdigest(),server_port=server_port,captured_client_port=peer[1],server_selected={'Encryption S2C':t.local_cipher,'Encryption C2S':t.remote_cipher,'MAC S2C':t.local_mac,'MAC C2S':t.remote_mac}));t.close()
 except Exception as e:errors.append((kex,repr(e)))
 finally:listener.close()
for kex in kexs:
 s=socket.socket();s.bind(('127.0.0.1',0));s.listen(1);s.settimeout(20);listeners.append(s);th=threading.Thread(target=serve,args=(s,kex));th.start();threads.append(th)
ports=[s.getsockname()[1] for s in listeners]
connect_ports=ports[:]
def proxy(listener,backend_port):
 front=back=None
 try:
  front,_=listener.accept();back=socket.create_connection(('127.0.0.1',backend_port),timeout=15);front.settimeout(15);back.settimeout(15);banner=b''
  while not banner.endswith(b'\n'):
   part=back.recv(1);assert part,'server closed before banner';banner+=part;assert len(banner)<=255
  assert banner.startswith(b'SSH-');front.sendall(banner);record={'server_port':backend_port,'captured_client_port':back.getsockname()[1],'server_banner_sha256':hashlib.sha256(banner).hexdigest(),'forwarded_server_before_client':True};proxy_records.append(record)
  def copy_wire(src,dst):
   try:
    while True:
     wire=src.recv(65536)
     if not wire:break
     dst.sendall(wire)
   except (OSError,EOFError):pass
   finally:
    try:dst.shutdown(socket.SHUT_WR)
    except OSError:pass
  worker=threading.Thread(target=copy_wire,args=(front,back));worker.start();copy_wire(back,front);worker.join(16);assert not worker.is_alive(),'proxy copy did not terminate'
 except Exception as e:errors.append(('proxy',backend_port,repr(e)))
 finally:
  for sock in[front,back,listener]:
   if sock is not None:sock.close()
if server_first:
 connect_ports=[]
 for port in ports:
  listener=socket.socket();listener.bind(('127.0.0.1',0));listener.listen(1);listener.settimeout(25);connect_ports.append(listener.getsockname()[1]);th=threading.Thread(target=proxy,args=(listener,port));th.start();threads.append(th)
(out/'known_hosts').write_text(''.join(f'[127.0.0.1]:{p} {host.get_name()} {host.get_base64()}\n' for p in connect_ports))
cap,marker_port=start_capture(sys.argv[2],out/'live.pcap',' or '.join(f'tcp port {p}' for p in ports),30)
try:
 for kex,port in zip(kexs,connect_ports):
  options=['BatchMode=yes','IdentitiesOnly=yes','StrictHostKeyChecking=yes','UserKnownHostsFile='+str(out/'known_hosts'),'GlobalKnownHostsFile='+str(out/'known_hosts'),'HostKeyAlgorithms='+host_algorithm,'RekeyLimit=32K','ConnectTimeout=5']
  if server_first:options+=['Ciphers=aes128-ctr,aes256-ctr','MACs=hmac-sha2-256,hmac-sha2-512']
  if kex!='default':options+=['KexAlgorithms='+kex]
  args=[sys.argv[4],'-vvv','-F','none','-p',str(port),'-i',str(out/'client.key')]
  for opt in options:args+=['-o',opt]
  p=subprocess.run(args+['audituser@127.0.0.1','audit-payload'],stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=15)
  (out/(kex+'.stderr')).write_bytes(p.stderr);assert p.returncode==0,(kex,p.returncode,p.stderr[-1000:]);expected=(f'{nonce}|{kex}|'.encode()*12000)[:131072];assert p.stdout==expected
  print('PASS',kex,len(p.stdout),flush=True)
  negotiated=re.search(rb'kex: algorithm: ([^\r\n]+)',p.stderr);host_alg=re.search(rb'kex: host key algorithm: ([^\r\n]+)',p.stderr);assert negotiated and host_alg
  selected={'Kex Algorithm':negotiated.group(1).decode(),'Host Key Algorithm':host_alg.group(1).decode()}
  for wire_direction,native_direction in[(b'client->server','C2S'),(b'server->client','S2C')]:
   match=re.search(rb'kex: '+wire_direction+rb' cipher: ([^ ]+) MAC: ([^ ]+)',p.stderr);assert match;selected['Encryption '+native_direction]=match.group(1).decode();selected['MAC '+native_direction]=match.group(2).decode()
  for operation in operations:
   if operation['requested']==kex:operation['kex']=selected['Kex Algorithm'];operation['client_selected']=selected
 for th in threads:th.join(15)
 assert not errors,errors
finally:
 stdout,stderr=cap.communicate(timeout=35);(out/'capture.stderr').write_bytes(stderr)
 (out/'manifest.json').write_text(json.dumps(dict(generator_sha256=generator_sha256,nonce=nonce,marker_port=marker_port,ports=ports,client_connect_ports=connect_ports,server_first_proxy=server_first,declared_nonconforming_server_identification='SSH-2.0-Cisco-1.25'if nonconforming_banner else None,identification_emulation='Paramiko test endpoint; no Cisco device claim'if nonconforming_banner else None,declared_client_cipher_preference=['aes128-ctr','aes256-ctr']if server_first else None,declared_server_cipher_preference=['aes256-ctr','aes128-ctr']if server_first else None,proxy_records=proxy_records,paramiko=importlib.metadata.version('paramiko'),host_key_format=host.get_name(),requested_host_key_algorithm=host_algorithm,host_key_sha256=hashlib.sha256(host.asbytes()).hexdigest(),operations=operations,errors=errors),indent=2),encoding='utf8')
assert len(operations)==len(kexs)
