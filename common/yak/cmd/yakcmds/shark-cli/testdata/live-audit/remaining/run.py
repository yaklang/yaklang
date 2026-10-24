"""Fresh loopback capture using independent clients/services, never Yak replies.

Usage: run.py OUTPUT_DIR GO_PEER_EXE PYTHON_LIBS BROKER_LIBS
All services bind 127.0.0.1. Output contains disposable credentials and keys.
"""
import asyncio, datetime, ftplib, hashlib, importlib.metadata, io, json, logging
import os, poplib, imaplib, secrets, smtplib, socket, socketserver, ssl, subprocess
import sys, threading, time
from pathlib import Path

if len(sys.argv)!=5:raise SystemExit(__doc__)
out=Path(sys.argv[1]).resolve();out.mkdir(parents=True,exist_ok=False)
sys.path[:0]=[sys.argv[3],sys.argv[4]]
from cryptography import x509
from cryptography.hazmat.primitives import hashes,serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.x509.oid import NameOID
from pyftpdlib.authorizers import DummyAuthorizer
from pyftpdlib.handlers import FTPHandler,TLS_FTPHandler
from pyftpdlib.servers import FTPServer
from pyftpdlib.ioloop import IOLoop
from aiosmtpd.controller import Controller
from impacket import ntlm
from impacket.smbserver import SimpleSMBServer
from impacket.smbconnection import SMBConnection,SessionError
from impacket.smb3structs import SMB2_DIALECT_002
import paho.mqtt.client as mqtt
from amqtt.broker import Broker
import ntplib

logging.basicConfig(filename=str(out/'services.log'),level=logging.WARNING)
nonce=secrets.token_hex(8);password='pw-'+nonce;ports={};ops=[];servers=[];threads=[]
def freeport():
    with socket.socket() as s:s.bind(('127.0.0.1',0));return s.getsockname()[1]
def note(protocol,kind,data=b'',**extra):
    ops.append(dict(Protocol=protocol,Kind=kind,Bytes=len(data),Hash=hashlib.sha256(data).hexdigest(),**extra))
def exercise(protocol,f):
    try:f()
    except Exception as e:ops.append(dict(Protocol=protocol,Error=type(e).__name__+': '+str(e)))
def launch(f):
    t=threading.Thread(target=f,daemon=True);t.start();threads.append(t)

private=ec.generate_private_key(ec.SECP256R1())
subject=x509.Name([x509.NameAttribute(NameOID.COMMON_NAME,'localhost')])
now=datetime.datetime.now(datetime.timezone.utc)
cert=(x509.CertificateBuilder().subject_name(subject).issuer_name(subject).public_key(private.public_key())
      .serial_number(x509.random_serial_number()).not_valid_before(now-datetime.timedelta(minutes=1))
      .not_valid_after(now+datetime.timedelta(days=1)).add_extension(x509.SubjectAlternativeName([x509.DNSName('localhost'),x509.IPAddress(__import__('ipaddress').ip_address('127.0.0.1'))]),False).sign(private,hashes.SHA256()))
(out/'certificate.pem').write_bytes(cert.public_bytes(serialization.Encoding.PEM))
(out/'server.key').write_bytes(private.private_bytes(serialization.Encoding.PEM,serialization.PrivateFormat.TraditionalOpenSSL,serialization.NoEncryption()))
ctx=ssl.create_default_context(cafile=str(out/'certificate.pem'));ctx.keylog_filename=str(out/'session.keys')
ctx.maximum_version=ssl.TLSVersion.TLSv1_2;ctx.set_ciphers('ECDHE-ECDSA-AES128-GCM-SHA256')
server_ctx=ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER);server_ctx.load_cert_chain(str(out/'certificate.pem'),str(out/'server.key'))
# Impacket's Windows file backend uses CRT text mode: keep LF-only/ASCII file
# contents so the independent server does not translate CRLF or truncate on ^Z.
payload=(('fresh-'+nonce+'\nSSH-2.0-embedded-file-body\n').encode()*1800)+secrets.token_hex(64).encode()
share=out/'share';share.mkdir();(share/'payload.bin').write_bytes(payload)
mail=('From: audit@example.invalid\r\nTo: loopback@example.invalid\r\nSubject: '+nonce+'\r\nMessage-ID: <'+nonce+'@example.invalid>\r\n\r\nSSH-2.0-mail-body-remains-mail\r\n'+nonce+'\r\n').encode()

for tls in [False,True]:
    auth=DummyAuthorizer();auth.add_user('audit',password,str(share),perm='elr')
    handler=type('AuditFTP',(TLS_FTPHandler if tls else FTPHandler,),{})
    handler.authorizer=auth;handler.banner='fresh audit FTP '+nonce
    handler.passive_ports=[freeport() for _ in range(4)]
    for i,p in enumerate(handler.passive_ports):ports[('ftps' if tls else 'ftp')+'_data_'+str(i)]=p
    if tls:handler.certfile=str(out/'certificate.pem');handler.keyfile=str(out/'server.key')
    server=FTPServer(('127.0.0.1',0),handler,ioloop=IOLoop());servers.append(server)
    ports['ftps' if tls else 'ftp']=server.socket.getsockname()[1]
    launch(lambda s=server:s.serve_forever(timeout=.05,blocking=True,handle_exit=False))

class MailSink:
    async def handle_DATA(self,server,session,envelope):
        if nonce.encode() not in envelope.original_content:return '550 bad test payload'
        return '250 captured by test sink'
ports['smtp']=freeport();smtp=Controller(MailSink(),hostname='127.0.0.1',port=ports['smtp'],tls_context=server_ctx,require_starttls=False);smtp.start()

smb=SimpleSMBServer(listenAddress='127.0.0.1',listenPort=0)
smb.getServer().daemon_threads=True;smb.getServer().block_on_close=False
smb.setSMB2Support(True);smb.setSMBChallenge(secrets.token_hex(8));smb.addCredential('audit',0,ntlm.compute_lmhash(password),ntlm.compute_nthash(password));smb.addShare('AUDIT',str(share),readOnly='yes')
ports['smb2']=smb.getServer().server_address[1];launch(smb.start)

class LocalTCP(socketserver.ThreadingTCPServer):
    allow_reuse_address=True;daemon_threads=True
class POP(socketserver.StreamRequestHandler):
    def handle(self):
        self.wfile.write(b'+OK fresh POP3 audit\r\n')
        for line in self.rfile:
            cmd=line.split(None,1)[0].upper()
            if cmd==b'CAPA':self.wfile.write(b'+OK\r\nUSER\r\nUIDL\r\n.\r\n')
            elif cmd in (b'USER',b'PASS',b'NOOP'):self.wfile.write(b'+OK\r\n')
            elif cmd==b'STAT':self.wfile.write(('+OK 1 %d\r\n'%len(mail)).encode())
            elif cmd==b'LIST':self.wfile.write(('+OK\r\n1 %d\r\n.\r\n'%len(mail)).encode())
            elif cmd==b'UIDL':self.wfile.write(('+OK\r\n1 '+nonce+'\r\n.\r\n').encode())
            elif cmd==b'RETR':self.wfile.write(b'+OK\r\n'+mail+b'.\r\n')
            elif cmd==b'QUIT':self.wfile.write(b'+OK bye\r\n');return
            else:self.wfile.write(b'-ERR test peer does not implement command\r\n')
class IMAP(socketserver.StreamRequestHandler):
    def handle(self):
        self.wfile.write(b'* OK fresh IMAP4rev1 audit\r\n')
        for line in self.rfile:
            parts=line.split(None,2)
            if len(parts)<2:return
            tag,cmd=parts[:2];cmd=cmd.upper()
            if cmd==b'CAPABILITY':self.wfile.write(b'* CAPABILITY IMAP4rev1\r\n')
            elif cmd==b'SELECT':self.wfile.write(b'* FLAGS (\\Seen)\r\n* 1 EXISTS\r\n* 0 RECENT\r\n')
            elif cmd==b'FETCH':self.wfile.write(('* 1 FETCH (RFC822 {%d}\r\n'%len(mail)).encode()+mail+b')\r\n')
            elif cmd==b'LOGOUT':self.wfile.write(b'* BYE closing\r\n'+tag+b' OK LOGOUT\r\n');return
            elif cmd not in (b'LOGIN',b'NOOP',b'CLOSE'):self.wfile.write(tag+b' BAD unsupported test command\r\n');continue
            self.wfile.write(tag+b' OK '+cmd+b' completed\r\n')
for name,handler in [('pop3',POP),('imap',IMAP)]:
    s=LocalTCP(('127.0.0.1',0),handler);servers.append(s);ports[name]=s.server_address[1];launch(s.serve_forever)

ports['mqtt']=freeport();broker_ready=threading.Event();broker_stop=None;broker_loop=None
def mqtt_server():
    async def run():
        global broker_stop,broker_loop
        broker_loop=asyncio.get_running_loop();broker_stop=asyncio.Event()
        b=Broker({'listeners':{'default':{'type':'tcp','bind':address}},'plugins':{'amqtt.plugins.authentication.AnonymousAuthPlugin':{'allow_anonymous':True}}})
        await b.start();broker_ready.set();await broker_stop.wait();await b.shutdown()
    address='127.0.0.1:'+str(ports['mqtt']);asyncio.run(run())
launch(mqtt_server)
if not broker_ready.wait(10):raise RuntimeError('MQTT broker did not start')
(out/'config.json').write_text(json.dumps(dict(Nonce=nonce,Ports=ports)))
log=(out/'peer.stderr').open('wb');child=subprocess.Popen([sys.argv[2],str(out)],stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=log)
try:
    deadline=time.monotonic()+12
    while not (out/'capture-ready.json').exists():
        if child.poll() is not None:raise RuntimeError('Go peer exited before capture')
        if time.monotonic()>deadline:raise TimeoutError('capture startup')
        time.sleep(.02)
    ports=json.loads((out/'capture-ready.json').read_text())
    def ftp_client(tls):
        c=ftplib.FTP_TLS(context=ctx,timeout=4) if tls else ftplib.FTP(timeout=4)
        c.connect('127.0.0.1',ports['ftps' if tls else 'ftp']);c.login('audit',password)
        if tls:c.prot_p()
        for _ in range(2):
            data=io.BytesIO();c.retrbinary('RETR payload.bin',data.write)
            assert data.getvalue()==payload
            note('ftps' if tls else 'ftp','retr',data.getvalue())
        c.quit()
    exercise('ftp',lambda:ftp_client(False));exercise('ftps',lambda:ftp_client(True))
    def smtp_client():
        c=smtplib.SMTP('127.0.0.1',ports['smtp'],timeout=4);c.ehlo('audit.invalid')
        c.sendmail('audit@example.invalid',['loopback@example.invalid'],mail);note('smtp','data',mail)
        c.starttls(context=ctx);c.ehlo('audit.invalid')
        for _ in range(2):c.sendmail('audit@example.invalid',['loopback@example.invalid'],mail);note('smtp','starttls-data',mail)
        c.quit()
    exercise('smtp',smtp_client)
    def smb_client():
        for _ in range(2):
            c=SMBConnection('AUDIT','127.0.0.1',sess_port=ports['smb2'],preferredDialect=SMB2_DIALECT_002,timeout=4)
            try:
                c.login('audit',password)
                data=io.BytesIO();c.getFile('AUDIT','payload.bin',data.write)
                if data.getvalue()!=payload:
                    (out/'smb-read.bin').write_bytes(data.getvalue())
                    raise AssertionError('SMB bytes mismatch: received=%d expected=%d'%(len(data.getvalue()),len(payload)))
                note('smb2','read-file',data.getvalue());c.logoff()
            finally:c.close()
        c=SMBConnection('AUDIT','127.0.0.1',sess_port=ports['smb2'],preferredDialect=SMB2_DIALECT_002,timeout=4)
        try:c.login('audit','wrong-'+nonce);raise AssertionError('wrong SMB password accepted')
        except SessionError as e:
            assert e.getErrorCode()==0xc000006d;note('smb2','authentication-rejected')
        finally:c.close()
    exercise('smb2',smb_client)
    def pop_client():
        c=poplib.POP3('127.0.0.1',ports['pop3'],timeout=4);c.user('audit');c.pass_(password);c.stat();c.list();c.uidl()
        for _ in range(2):data=b'\r\n'.join(c.retr(1)[1])+b'\r\n';assert data==mail;note('pop3','retr',data)
        c.quit()
    exercise('pop3',pop_client)
    def imap_client():
        c=imaplib.IMAP4('127.0.0.1',ports['imap'],timeout=4);c.login('audit',password);c.select('INBOX')
        for _ in range(2):status,rows=c.fetch('1','(RFC822)');assert status=='OK';data=next(v[1] for v in rows if isinstance(v,tuple));assert data==mail;note('imap','fetch-literal',data)
        c.close();c.logout()
    exercise('imap',imap_client)
    def mqtt_client():
        connected=threading.Event();subscribed=threading.Event();messages=[]
        sub=mqtt.Client(mqtt.CallbackAPIVersion.VERSION2,client_id='sub-'+nonce)
        sub.on_connect=lambda *a:connected.set();sub.on_subscribe=lambda *a:subscribed.set();sub.on_message=lambda c,u,m:messages.append(m.payload)
        sub.connect('127.0.0.1',ports['mqtt'],keepalive=10);sub.loop_start();assert connected.wait(4)
        topic='audit/'+nonce;sub.subscribe(topic,qos=2);assert subscribed.wait(4)
        pub=mqtt.Client(mqtt.CallbackAPIVersion.VERSION2,client_id='pub-'+nonce);pub.connect('127.0.0.1',ports['mqtt'],keepalive=10);pub.loop_start()
        expected=[]
        for qos,size in enumerate([7,4096,70000]):
            data=(('mqtt-'+nonce+'|SSH-2.0-body|').encode()*3000)[:size];expected.append(data)
            info=pub.publish(topic,data,qos=qos,retain=qos==2);info.wait_for_publish(timeout=4);assert info.is_published();note('mqtt','publish-qos'+str(qos),data)
        deadline=time.monotonic()+5
        while len(messages)<3 and time.monotonic()<deadline:time.sleep(.02)
        assert messages==expected
        for data in messages:note('mqtt','received',data)
        pub.disconnect();pub.loop_stop();sub.disconnect();sub.loop_stop()
    exercise('mqtt',mqtt_client)
    exercise('ntp',lambda:note('ntp','query',str(ntplib.NTPClient().request('127.0.0.1',version=4,port=ports['ntp'],timeout=4).orig_time).encode()))
finally:
    (out/'python-operations.json').write_text(json.dumps(ops,indent=2),encoding='utf8')
    print('client results',json.dumps([x for x in ops if x.get('Error')]),flush=True)
    if child.poll() is None:child.stdin.write(b'done\n');child.stdin.flush()
    stdout,_=child.communicate(timeout=25);log.close()
    for s in servers:
        if isinstance(s,FTPServer):s.close_all()
        else:s.shutdown();s.server_close()
    smtp.stop();smb.getServer().shutdown();smb.stop()
    if broker_loop and broker_stop:broker_loop.call_soon_threadsafe(broker_stop.set)
    print(stdout.decode(errors='replace').strip())
go=json.loads((out/'go-manifest.json').read_text());go['Operations']+=ops
go['Libraries']={name:importlib.metadata.version(name) for name in ['impacket','pyftpdlib','aiosmtpd','paho-mqtt','amqtt','ntplib']}
go['PeerProfiles']={'postgresql':'minimal wire peer + lib/pq client, not a database','mysql':'minimal authenticated text-query peer + go-sql-driver/mysql, not a database','ldap':'minimal BER directory peer + go-ldap client','pop3':'minimal mail-store peer + stdlib poplib','imap':'minimal literal mail-store peer + stdlib imaplib','dhcp':'controlled socket DORA messages; no OS lease configured','snmp':'gosnmp codec responder and independent client instances','ntp':'bounded socket responder + ntplib client'}
(out/'manifest.json').write_text(json.dumps(go,indent=2),encoding='utf8')
errors=[x for x in go['Operations'] if x.get('Error')]
print('operations',len(go['Operations']),'errors',json.dumps(errors))
if child.returncode or errors:raise SystemExit(1)
