"""Independent OpenSSL clients, Python/OpenSSL servers, real Npcap capture.
Usage: tls_suites.py OUTPUT_DIR TSHARK OPENSSL PYTHON_LIBS
"""
import datetime,hashlib,json,os,secrets,socket,ssl,subprocess,sys,threading,time
from pathlib import Path
sys.path.insert(0,sys.argv[4])
from cryptography import x509
from cryptography.hazmat.primitives import hashes,serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.x509.oid import NameOID
out=Path(sys.argv[1]);out.mkdir(exist_ok=False)
key=ec.generate_private_key(ec.SECP256R1());name=x509.Name([x509.NameAttribute(NameOID.COMMON_NAME,'audit.invalid')]);now=datetime.datetime.now(datetime.timezone.utc)
cert=x509.CertificateBuilder().subject_name(name).issuer_name(name).public_key(key.public_key()).serial_number(x509.random_serial_number()).not_valid_before(now-datetime.timedelta(minutes=1)).not_valid_after(now+datetime.timedelta(hours=1)).add_extension(x509.SubjectAlternativeName([x509.DNSName('audit.invalid')]),False).sign(key,hashes.SHA256())
(out/'cert.pem').write_bytes(cert.public_bytes(serialization.Encoding.PEM));(out/'key.pem').write_bytes(key.private_bytes(serialization.Encoding.PEM,serialization.PrivateFormat.PKCS8,serialization.NoEncryption()))
profiles=[('tls13-aes128','-tls1_3','TLS_AES_128_GCM_SHA256'),('tls13-aes256','-tls1_3','TLS_AES_256_GCM_SHA384'),('tls13-chacha','-tls1_3','TLS_CHACHA20_POLY1305_SHA256'),('tls12-aes128','-tls1_2','ECDHE-ECDSA-AES128-GCM-SHA256'),('tls12-aes256','-tls1_2','ECDHE-ECDSA-AES256-GCM-SHA384'),('tls12-chacha','-tls1_2','ECDHE-ECDSA-CHACHA20-POLY1305')]
listeners=[];threads=[];operations=[];errors=[];nonce=secrets.token_hex(12)
for profile,version,cipher in profiles:
    listener=socket.socket();listener.bind(('127.0.0.1',0));listener.listen(1);listener.settimeout(15);listeners.append(listener)
    def serve(listener=listener,profile=profile,version=version,cipher=cipher):
        try:
            ctx=ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER);ctx.load_cert_chain(out/'cert.pem',out/'key.pem')
            ctx.minimum_version=ctx.maximum_version=ssl.TLSVersion.TLSv1_3 if version=='-tls1_3' else ssl.TLSVersion.TLSv1_2
            if version=='-tls1_2':ctx.set_ciphers(cipher)
            raw,_=listener.accept();raw.settimeout(8)
            with ctx.wrap_socket(raw,server_side=True) as s:
                pending=b''
                for i in range(3):
                    while b'\r\n\r\n' not in pending:pending+=s.recv(65535)
                    request,pending=pending.split(b'\r\n\r\n',1)
                    assert request.startswith(f'GET /{profile}/{i} HTTP/1.1'.encode())
                    body=(f'{nonce}/{profile}/{i}|'.encode()*1000)[:19000+i]
                    s.sendall(f'HTTP/1.1 200 OK\r\nContent-Length: {len(body)}\r\nContent-Type: application/octet-stream\r\n\r\n'.encode()+body)
                    operations.append(dict(profile=profile,cipher=s.cipher(),version=s.version(),index=i,body_sha256=hashlib.sha256(body).hexdigest(),bytes=len(body)))
                # Complete TLS close_notify with the independent client.
                s.unwrap().close()
        except Exception as e:errors.append(dict(profile=profile,error=str(e)))
        finally:listener.close()
    t=threading.Thread(target=serve,daemon=True);t.start();threads.append(t)
ports=[l.getsockname()[1] for l in listeners]
capture=subprocess.Popen([sys.argv[2],'-n','-i',r'\Device\NPF_Loopback','-f',' or '.join(f'tcp port {p}' for p in ports),'-F','pcap','-w',str(out/'live.pcap'),'-a','duration:12'],stdout=subprocess.PIPE,stderr=subprocess.PIPE,creationflags=0x08000000)
time.sleep(1)
for (profile,version,cipher),port in zip(profiles,ports):
    requests=b''.join(f'GET /{profile}/{i} HTTP/1.1\r\nHost: audit.invalid\r\n\r\n'.encode() for i in range(3))
    p=subprocess.run([sys.argv[3],'s_client','-connect',f'127.0.0.1:{port}','-servername','audit.invalid','-verify_hostname','audit.invalid','-CAfile',str(out/'cert.pem'),version,'-ciphersuites' if version=='-tls1_3' else '-cipher',cipher,'-keylogfile',str(out/(profile+'.keys')),'-quiet','-ign_eof'],input=requests,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=10,creationflags=0x08000000)
    (out/(profile+'.stderr')).write_bytes(p.stderr)
    assert p.returncode==0,(profile,p.stderr.decode(errors='replace'))
    wire=p.stdout
    for operation in [o for o in operations if o['profile']==profile]:
        head,wire=wire.split(b'\r\n\r\n',1);body,wire=wire[:operation['bytes']],wire[operation['bytes']:]
        assert hashlib.sha256(body).hexdigest()==operation['body_sha256']
    assert not wire
for t in threads:t.join(timeout=3)
stdout,stderr=capture.communicate(timeout=18);(out/'capture.stderr').write_bytes(stderr)
assert capture.returncode==0 and len(operations)==18 and not errors,(capture.returncode,errors,len(operations))
(out/'session.keys').write_bytes(b'\n'.join((out/(p[0]+'.keys')).read_bytes() for p in profiles))
(out/'manifest.json').write_text(json.dumps(dict(nonce=nonce,ports=ports,operations=operations,errors=errors),indent=2),encoding='utf8')
print('PASS: six independently negotiated suites, 18 HTTP responses; application body SHA-256 verified')
