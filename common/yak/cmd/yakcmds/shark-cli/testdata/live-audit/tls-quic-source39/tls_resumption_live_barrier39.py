"""Capture owned loopback TLS 1.2/1.3 resumption and literal TCP tuple reuse.

Each version uses one server context and four actual connections on one client
port: full, resumed, resumed, fresh. Python/OpenSSL peer receipts distinguish
real resumption from capture heuristics. TLS close_notify completes before an
explicit abortive TCP close permits tuple reuse. No early-data/0-RTT claim.
Only the two owned server ports and a private readiness marker are captured;
capture output is bounded at 100 MiB. Certificates, keys, plaintext and PCAPs
must remain outside a Git repository.
"""
import argparse,base64,datetime,hashlib,json,os,queue,secrets,socket,ssl,struct,subprocess,threading,time
from pathlib import Path
from cryptography import x509
from cryptography.hazmat.primitives import hashes,serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.x509.oid import NameOID


def sha(data):return hashlib.sha256(data).hexdigest()
def session_receipt(s):
    return dict(session_reused=s.session_reused,version=s.version(),cipher=s.cipher(),ALPN=s.selected_alpn_protocol(),session_id_SHA256=sha(s.session.id),session_has_ticket=s.session.has_ticket,session_ticket_lifetime_hint=s.session.ticket_lifetime_hint)


def capture(tshark,output,ports,duration):
    marker=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);marker.bind(('127.0.0.1',0));port=marker.getsockname()[1]
    expression=' or '.join(f'tcp port {p}'for p in ports)+f' or udp port {port}'
    args=[str(tshark),'-n','-i',r'\Device\NPF_Loopback','-f',expression,'-F','pcap','-w',str(output),'-a',f'duration:{duration}','-a','filesize:102400','-l','-P','-T','fields','-e','frame.number']
    cap=subprocess.Popen(args,stdout=subprocess.PIPE,stderr=subprocess.PIPE,creationflags=0x08000000)
    ready=queue.Queue();threading.Thread(target=lambda:ready.put(cap.stdout.readline()),daemon=True).start()
    try:
        deadline=time.monotonic()+10
        while time.monotonic()<deadline:
            marker.sendto(b'tls-resumption-ready:'+secrets.token_hex(8).encode(),('127.0.0.1',port))
            try:
                line=ready.get(timeout=.1);assert line.strip().isdigit(),line
                # Drain diagnostic frame numbers so a pipe cannot block capture.
                threading.Thread(target=lambda:cap.stdout.read(),daemon=True).start()
                return cap,dict(arguments=args,marker_port=port,capture_filter=expression,maximum_KiB=102400)
            except queue.Empty:assert cap.poll()is None
        raise TimeoutError('owned capture readiness marker not observed')
    except Exception:
        cap.terminate();cap.communicate(timeout=5);raise
    finally:marker.close()


def response_body(nonce,profile,generation,index):
    return (f'{nonce}/{profile}/{generation}/{index}|'.encode()*100)[:1024+generation*7+index*31]


def run(out,tshark):
    out.mkdir(parents=True,exist_ok=False)
    assert 20*(1<<30)<__import__('shutil').disk_usage(out).free
    key=ec.generate_private_key(ec.SECP256R1());name=x509.Name([x509.NameAttribute(NameOID.COMMON_NAME,'audit.invalid')]);now=datetime.datetime.now(datetime.timezone.utc)
    cert=x509.CertificateBuilder().subject_name(name).issuer_name(name).public_key(key.public_key()).serial_number(x509.random_serial_number()).not_valid_before(now-datetime.timedelta(minutes=1)).not_valid_after(now+datetime.timedelta(hours=1)).add_extension(x509.SubjectAlternativeName([x509.DNSName('audit.invalid')]),False).sign(key,hashes.SHA256())
    (out/'cert.pem').write_bytes(cert.public_bytes(serialization.Encoding.PEM));(out/'key.pem').write_bytes(key.private_bytes(serialization.Encoding.PEM,serialization.PrivateFormat.PKCS8,serialization.NoEncryption()))
    nonce=secrets.token_hex(12);receipts=[];operations=[];failures=[];threads=[];listeners=[];profiles=[]
    close_barriers={(profile,generation):threading.Barrier(2)for profile in("tls12","tls13")for generation in range(4)}
    for profile,version in[('tls12',ssl.TLSVersion.TLSv1_2),('tls13',ssl.TLSVersion.TLSv1_3)]:
        listener=socket.socket();listener.bind(('127.0.0.1',0));listener.listen(4);listener.settimeout(15);listeners.append(listener)
        ctx=ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER);ctx.load_cert_chain(out/'cert.pem',out/'key.pem');ctx.minimum_version=ctx.maximum_version=version;ctx.set_alpn_protocols(['http/1.1'])
        if profile=='tls12':ctx.set_ciphers('ECDHE-ECDSA-AES128-GCM-SHA256')
        else:ctx.num_tickets=2
        profiles.append(dict(profile=profile,server_port=listener.getsockname()[1],version=version.name))
        def serve(listener=listener,ctx=ctx,profile=profile):
            try:
                for generation in range(4):
                    raw,remote=listener.accept();raw.settimeout(10);raw.setsockopt(socket.IPPROTO_TCP,socket.TCP_NODELAY,1)
                    with ctx.wrap_socket(raw,server_side=True)as s:
                        pending=b''
                        for index in range(3):
                            while b'\r\n\r\n'not in pending:
                                piece=s.recv(65535);assert piece,'missing request before close';pending+=piece
                            request,pending=pending.split(b'\r\n\r\n',1);request+=b'\r\n\r\n'
                            expected=f'GET /{profile}/{generation}/{index} HTTP/1.1\r\nHost: audit.invalid\r\nX-Case: {nonce}\r\n\r\n'.encode();assert request==expected
                            body=response_body(nonce,profile,generation,index);response=f'HTTP/1.1 200 OK\r\nContent-Length: {len(body)}\r\nContent-Type: application/octet-stream\r\nX-Generation: {generation}\r\n\r\n'.encode()+body
                            s.sendall(response);operations.append(dict(profile=profile,generation=generation,index=index,request_SHA256=sha(request),response_SHA256=sha(response),body_SHA256=sha(body),body_bytes=len(body),server_peer=list(remote)))
                        receipts.append(dict(profile=profile,generation=generation,peer='server',endpoint=list(s.getsockname()),remote=list(s.getpeername()),**session_receipt(s)))
                        transport=s.unwrap();close_barriers[(profile,generation)].wait(timeout=5);transport.setsockopt(socket.SOL_SOCKET,socket.SO_LINGER,struct.pack('HH',1,0));transport.close()
            except Exception as exc:failures.append(dict(profile=profile,peer='server',error=repr(exc)))
            finally:listener.close()
        thread=threading.Thread(target=serve,daemon=True);thread.start();threads.append(thread)
    cap,capmeta=capture(tshark,out/'live.pcap',[p['server_port']for p in profiles],35)
    try:
        for item in profiles:
            profile=item['profile'];ctx=ssl.create_default_context(cafile=str(out/'cert.pem'));ctx.check_hostname=True;ctx.minimum_version=ctx.maximum_version=getattr(ssl.TLSVersion,item['version']);ctx.set_alpn_protocols(['http/1.1']);ctx.keylog_filename=str(out/(profile+'.keys'))
            if profile=='tls12':ctx.set_ciphers('ECDHE-ECDSA-AES128-GCM-SHA256')
            saved=None;client_port=None
            for generation in range(4):
                raw=socket.socket();raw.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1);raw.setsockopt(socket.SOL_SOCKET,socket.SO_LINGER,struct.pack('HH',1,0));raw.setsockopt(socket.IPPROTO_TCP,socket.TCP_NODELAY,1);raw.settimeout(10);raw.bind(('127.0.0.1',client_port or 0));raw.connect(('127.0.0.1',item['server_port']))
                observed=raw.getsockname()[1]
                if client_port is None:client_port=observed
                assert observed==client_port
                supplied=saved if generation in[1,2]else None
                with ctx.wrap_socket(raw,server_hostname='audit.invalid',session=supplied)as s:
                    for index in range(3):
                        request=f'GET /{profile}/{generation}/{index} HTTP/1.1\r\nHost: audit.invalid\r\nX-Case: {nonce}\r\n\r\n'.encode();s.sendall(request);wire=b''
                        while b'\r\n\r\n'not in wire:
                            piece=s.recv(65535);assert piece;wire+=piece
                        head,body=wire.split(b'\r\n\r\n',1);expected=response_body(nonce,profile,generation,index)
                        while len(body)<len(expected):
                            piece=s.recv(65535);assert piece;body+=piece
                        assert body==expected and f'Content-Length: {len(body)}'.encode()in head
                    assert s.session_reused is (generation in[1,2]),(profile,generation,s.session_reused)
                    assert s.selected_alpn_protocol()=='http/1.1'and s.session.has_ticket
                    receipts.append(dict(profile=profile,generation=generation,peer='client',endpoint=list(s.getsockname()),remote=list(s.getpeername()),session_argument_supplied=supplied is not None,**session_receipt(s)))
                    saved=s.session
                    transport=s.unwrap();close_barriers[(profile,generation)].wait(timeout=5);transport.setsockopt(socket.SOL_SOCKET,socket.SO_LINGER,struct.pack('HH',1,0));transport.close()
                time.sleep(.08)
            item['reused_client_port']=client_port
        for thread in threads:thread.join(timeout=5)
        assert all(not t.is_alive()for t in threads)and not failures,failures
    except Exception as exc:
        failures.append(dict(peer='driver',error=repr(exc)));raise
    finally:
        _,stderr=cap.communicate(timeout=45);(out/'capture.stderr').write_bytes(stderr)
        (out/'session.keys').write_bytes(b'\n'.join((out/(p['profile']+'.keys')).read_bytes()for p in profiles if(out/(p['profile']+'.keys')).exists()))
        (out/'manifest.json').write_text(json.dumps(dict(scope=__doc__,generator_SHA256=sha(Path(__file__).read_bytes()),Python_OpenSSL=ssl.OPENSSL_VERSION,nonce=nonce,profiles=profiles,peer_receipts=receipts,operations=operations,failures=failures,capture=capmeta,capture_SHA256=sha((out/'live.pcap').read_bytes()),explicit_capture_omission=False,network_fault_injection=False,early_data_claim=False),indent=2)+'\n',encoding='utf8')
    assert cap.returncode==0 and len(receipts)==16 and len(operations)==24 and not failures
    assert(out/'live.pcap').stat().st_size<100*(1<<20)
    print(json.dumps(dict(profiles=profiles,connections=8,real_resumed_connections=4,HTTP_pairs=24,capture_bytes=(out/'live.pcap').stat().st_size,peer_receipts=16,failures=failures)),flush=True)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('output',type=Path);ap.add_argument('tshark',type=Path);args=ap.parse_args()
    assert not any((p/'.git').exists()for p in[args.output.resolve(),*args.output.resolve().parents]);run(args.output,args.tshark)
