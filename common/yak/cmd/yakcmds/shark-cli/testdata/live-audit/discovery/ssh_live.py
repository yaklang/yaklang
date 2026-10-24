"""Real Windows OpenSSH clients and independent Paramiko server. No Yak codec.
Usage: ssh_live.py OUTPUT_DIR TSHARK PYTHON_LIBS OPENSSH
"""
import base64,hashlib,importlib.metadata,json,logging,re,secrets,socket,subprocess,sys,threading,time
from pathlib import Path
from capture_ready import start_capture
sys.path.insert(0,sys.argv[3]);import paramiko
from paramiko.kex_group14 import KexGroup14SHA256
out=Path(sys.argv[1]);out.mkdir(exist_ok=False);nonce=secrets.token_hex(12)
host=paramiko.RSAKey.generate(2048);client_key=paramiko.RSAKey.generate(2048);client_key.write_private_key_file(str(out/'client.key'))
subprocess.run(['icacls',str(out/'client.key'),'/inheritance:r','/grant:r',__import__('getpass').getuser()+':(R,W)'],stdout=subprocess.PIPE,check=True)
moduli=out/'moduli';moduli.write_text('20261024000000 2 6 100 2047 2 '+format(KexGroup14SHA256.P,'x')+'\n')
kexs=['curve25519-sha256@libssh.org','ecdh-sha2-nistp256','diffie-hellman-group14-sha256','diffie-hellman-group-exchange-sha256','default']
listeners=[];operations=[];errors=[];threads=[]
logging.basicConfig(filename=str(out/'paramiko.log'),level=logging.DEBUG)
class Server(paramiko.ServerInterface):
 def check_auth_publickey(self,user,key):return paramiko.AUTH_SUCCESSFUL if user=='audituser' and key==client_key else paramiko.AUTH_FAILED
 def get_allowed_auths(self,user):return 'publickey'
 def check_channel_request(self,kind,chanid):return paramiko.OPEN_SUCCEEDED if kind=='session' else paramiko.OPEN_FAILED_ADMINISTRATIVELY_PROHIBITED
 def check_channel_exec_request(self,channel,command):self.command=command;self.ready.set();return True
def serve(listener,kex):
 try:
  raw,_=listener.accept();raw.settimeout(15);t=paramiko.Transport(raw);t.add_server_key(host);t.load_server_moduli(str(moduli));server=Server();server.ready=threading.Event();t.start_server(server=server)
  ch=t.accept(10);assert ch is not None and server.ready.wait(10);assert server.command==b'audit-payload'
  body=(f'{nonce}|{kex}|'.encode()*12000)[:131072];ch.sendall(body);ch.send_exit_status(0);ch.shutdown_write();time.sleep(.3);ch.close();t.close();operations.append(dict(kex=kex,bytes=len(body),sha256=hashlib.sha256(body).hexdigest()))
 except Exception as e:errors.append((kex,repr(e)))
 finally:listener.close()
for kex in kexs:
 s=socket.socket();s.bind(('127.0.0.1',0));s.listen(1);s.settimeout(20);listeners.append(s);th=threading.Thread(target=serve,args=(s,kex));th.start();threads.append(th)
ports=[s.getsockname()[1] for s in listeners]
(out/'known_hosts').write_text(''.join(f'[127.0.0.1]:{p} {host.get_name()} {host.get_base64()}\n' for p in ports))
cap,marker_port=start_capture(sys.argv[2],out/'live.pcap',' or '.join(f'tcp port {p}' for p in ports),18)
try:
 for kex,port in zip(kexs,ports):
  options=['BatchMode=yes','IdentitiesOnly=yes','StrictHostKeyChecking=yes','UserKnownHostsFile='+str(out/'known_hosts'),'GlobalKnownHostsFile='+str(out/'known_hosts'),'HostKeyAlgorithms=rsa-sha2-256','RekeyLimit=32K','ConnectTimeout=5']
  if kex!='default':options+=['KexAlgorithms='+kex]
  args=[sys.argv[4],'-vvv','-F','none','-p',str(port),'-i',str(out/'client.key')]
  for opt in options:args+=['-o',opt]
  p=subprocess.run(args+['audituser@127.0.0.1','audit-payload'],stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=15)
  (out/(kex+'.stderr')).write_bytes(p.stderr);assert p.returncode==0,(kex,p.returncode,p.stderr[-1000:]);expected=(f'{nonce}|{kex}|'.encode()*12000)[:131072];assert p.stdout==expected
  print('PASS',kex,len(p.stdout),flush=True)
  if kex=='default':
   negotiated=re.search(rb'kex: algorithm: ([^\r\n]+)',p.stderr);assert negotiated
   for operation in operations:
    if operation['kex']=='default':operation['requested']='default';operation['kex']=negotiated.group(1).decode()
 for th in threads:th.join(15)
 assert not errors,errors
finally:
 stdout,stderr=cap.communicate(timeout=25);(out/'capture.stderr').write_bytes(stderr)
 (out/'manifest.json').write_text(json.dumps(dict(nonce=nonce,marker_port=marker_port,ports=ports,paramiko=importlib.metadata.version('paramiko'),host_key_sha256=hashlib.sha256(host.asbytes()).hexdigest(),operations=operations,errors=errors),indent=2),encoding='utf8')
assert len(operations)==len(kexs)
