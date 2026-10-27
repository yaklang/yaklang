"""Capture bounded CoAP v1 GET/ACK exchanges on actual Windows UDP sockets.

This is a declared raw CoAP peer, not a full production CoAP implementation.
Independent socket receipts verify original tokens, URI options and reply bytes.
No Yak parser is used. Capture and full receipts must stay outside Git.
Usage: coap_clock_live.py OUTPUT_DIRECTORY TSHARK
"""
import hashlib,json,secrets,socket,subprocess,sys
from pathlib import Path
from capture_ready import start_capture

out=Path(sys.argv[1]).resolve()
assert not any((p/'.git').exists()for p in[out,*out.parents])
out.mkdir(exist_ok=False)
server=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
try:server.bind(('127.0.0.1',5683))
except OSError:server.bind(('127.0.0.1',0))
client=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);client.bind(('127.0.0.1',0))
for peer in[server,client]:peer.settimeout(3)
capture=None;operations=[]
try:
    capture,marker=start_capture(Path(sys.argv[2]),out/'live.pcap',f'udp port {server.getsockname()[1]}',6)
    base=secrets.randbelow(65530)
    for ordinal in range(3):
        identifier=(base+ordinal).to_bytes(2,'big');token=secrets.token_bytes(4)
        uri=b'\xb5clock\x01'+str(ordinal).encode('ascii')
        request=b'\x44\x01'+identifier+token+uri
        body=('coap-clock-receipt-'+str(ordinal)).encode('ascii')
        reply=b'\x64\x45'+identifier+token+b'\xff'+body
        client.sendto(request,server.getsockname())
        observed,peer=server.recvfrom(65535)
        assert peer==client.getsockname()and observed==request
        server.sendto(reply,peer)
        observed,peer=client.recvfrom(65535)
        assert peer==server.getsockname()and observed==reply
        operations.append(dict(ordinal=ordinal,message_id=int.from_bytes(identifier,'big'),token_hex=token.hex(),URI='/clock/'+str(ordinal),request_hex=request.hex(),request_SHA256=hashlib.sha256(request).hexdigest(),reply_hex=reply.hex(),reply_SHA256=hashlib.sha256(reply).hexdigest(),body_SHA256=hashlib.sha256(body).hexdigest()))
    stdout,stderr=capture.communicate(timeout=15)
    (out/'capture.stdout').write_bytes(stdout);(out/'capture.stderr').write_bytes(stderr)
    assert capture.returncode==0
    proof=dict(scope=__doc__,generator_SHA256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),readiness_helper_SHA256=hashlib.sha256(Path(__file__).with_name('capture_ready.py').read_bytes()).hexdigest(),capture_SHA256=hashlib.sha256((out/'live.pcap').read_bytes()).hexdigest(),client=list(client.getsockname()),server=list(server.getsockname()),capture_ready_observed=True,marker_port=marker,operations=operations)
    with(out/'manifest.json').open('x',encoding='utf8',newline='\n')as file:json.dump(proof,file,indent=2);file.write('\n')
    print(json.dumps(dict(actual_UDP_exchanges=len(operations),socket_receipts_exact=True,capture_ready_observed=True)),flush=True)
finally:
    server.close();client.close()
    if capture is not None and capture.poll()is None:
        capture.terminate();capture.communicate(timeout=5)
