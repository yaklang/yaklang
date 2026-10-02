"""Generate independent OpenSSL DTLS 1.0/1.2 socket captures and TShark oracles.

Local test endpoints only. Private keys stay in a temporary directory, outside
repository evidence. The parser under test never constructs protocol packets.
"""
import argparse, hashlib, json, os, pathlib, signal, subprocess, tempfile, time


def run(cmd, **kw):
    return subprocess.run(cmd, check=True, **kw)


def main():
    ap = argparse.ArgumentParser(); ap.add_argument('output', type=pathlib.Path)
    out=ap.parse_args().output.resolve();out.mkdir(parents=True,exist_ok=True)
    if any(out.iterdir()): ap.error('output must be empty')
    tshark='/Applications/Wireshark.app/Contents/MacOS/tshark'
    dumpcap='/Applications/Wireshark.app/Contents/MacOS/dumpcap'
    version=run(['openssl','version'],capture_output=True,text=True).stdout.strip()
    with tempfile.TemporaryDirectory(prefix='dtls-native-key-') as tmp:
        tmp=pathlib.Path(tmp)
        run(['openssl','req','-x509','-newkey','rsa:2048','-nodes','-days','3650',
             '-keyout',str(tmp/'key.pem'),'-out',str(out/'server.pem'),
             '-subj','/CN=dtls.native.test','-addext','subjectAltName=DNS:dtls.native.test'],capture_output=True)
        for label,mtu,port,mode,cipher in [('normal',1500,17443,'-dtls1_2','ECDHE-RSA-AES128-GCM-SHA256'),('fragmented',512,17444,'-dtls1_2','ECDHE-RSA-AES128-GCM-SHA256'),('legacy',1500,17445,'-dtls1','AES128-SHA:@SECLEVEL=0')]:
            processes=[];handles=[]
            try:
                for name in ['capture','server','client']:
                    handles.append((out/f'{label}-{name}.log').open('wb'))
                capture=subprocess.Popen([dumpcap,'-i','lo0','-f',f'udp port {port}','-w',str(out/f'{label}.pcapng')],stdout=handles[0],stderr=handles[0]);processes.append(capture)
                time.sleep(0.6)
                server=subprocess.Popen(['openssl','s_server',mode,'-listen','-quiet',
                         '-accept',f'127.0.0.1:{port}','-cert',str(out/'server.pem'),'-key',str(tmp/'key.pem'),
                         '-mtu',str(mtu),'-cipher',cipher],stdin=subprocess.PIPE,stdout=handles[1],stderr=handles[1]);processes.append(server)
                time.sleep(0.5)
                client=subprocess.Popen(['openssl','s_client',mode,'-quiet','-timeout',
                         '-connect',f'127.0.0.1:{port}','-servername','dtls.native.test','-alpn','coap',
                         '-verify_return_error','-CAfile',str(out/'server.pem'),'-mtu',str(mtu),
                         '-cipher',cipher],stdin=subprocess.PIPE,stdout=handles[2],stderr=handles[2]);processes.append(client)
                client.stdin.write(b'native-dtls-client-request\n');client.stdin.flush()
                deadline=time.monotonic()+20
                while b'native-dtls-client-request' not in (out/f'{label}-server.log').read_bytes():
                    if time.monotonic()>deadline: raise RuntimeError('DTLS handshake/application exchange timed out')
                    time.sleep(0.1)
                server.stdin.write(b'native-dtls-server-response\n');server.stdin.flush()
                while b'native-dtls-server-response' not in (out/f'{label}-client.log').read_bytes():
                    if time.monotonic()>deadline: raise RuntimeError('DTLS response timed out')
                    time.sleep(0.1)
                client.send_signal(signal.SIGINT);client.wait(timeout=5)
                time.sleep(0.3)
                (out/f'{label}-endpoint-oracle.json').write_text(json.dumps({'client_request':'native-dtls-client-request','server_response':'native-dtls-server-response','verified_test_certificate':True,'cipher':cipher,'version':mode,'mtu':mtu,'transport':'udp','producer':version},indent=2)+'\n')
            finally:
                for proc in reversed(processes):
                    if proc.poll() is None: proc.send_signal(signal.SIGINT)
                    try: proc.wait(timeout=5)
                    except subprocess.TimeoutExpired: proc.kill();proc.wait()
                for h in handles:h.close()
            # Exact original datagrams and each independent dissector record.
            fields=['frame.number','ip.src','udp.srcport','ip.dst','udp.dstport','udp.payload',
                    'dtls.record.content_type','dtls.record.version','dtls.record.epoch','dtls.record.sequence_number','dtls.record.length',
                    'dtls.handshake.type','dtls.handshake.message_seq','dtls.handshake.fragment_offset','dtls.handshake.fragment_length',
                    'dtls.handshake.random','dtls.handshake.ciphersuite','dtls.handshake.extensions_server_name']
            cmd=[tshark,'-n','-r',str(out/f'{label}.pcapng'),'-d',f'udp.port=={port},dtls','-T','fields']
            for field in fields:cmd+=['-e',field]
            (out/f'{label}-tshark.tsv').write_text(run(cmd,capture_output=True,text=True).stdout)
    (out/'versions.json').write_text(json.dumps({'openssl':version,'tshark':run([tshark,'-v'],capture_output=True,text=True).stdout.splitlines()[0],'capture':'dumpcap lo0; no packet mutation','sources':['https://github.com/openssl/openssl','https://www.rfc-editor.org/rfc/rfc6347.html'],'scope':'DTLS 1.0/1.2 UDP / full and fragmented certificate handshake; application ciphertext is opaque'},indent=2)+'\n')
    files=[{'file':p.name,'sha256':hashlib.sha256(p.read_bytes()).hexdigest(),'bytes':p.stat().st_size} for p in sorted(out.iterdir()) if p.is_file()]
    (out/'manifest.json').write_text(json.dumps({'evidence_kind':'independent-native-capture','fixture_license':'CC0-1.0','generator_sha256':hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest(),'files':files},indent=2)+'\n')

if __name__=='__main__':main()
