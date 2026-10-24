"""Independent aioquic over real Windows UDP: AEADs, key updates, CID rotation,
NAT rebinding, same socket reuse, drop/delay/corruption. Uses no Yak parser.
Usage: quic_epochs.py OUTPUT_DIR TSHARK AIOQUIC_LIBS CERT KEY
"""
import collections,hashlib,importlib.metadata,json,select,secrets,socket,subprocess,sys,time
from pathlib import Path
sys.path.insert(0,sys.argv[3])
from aioquic.buffer import Buffer
from aioquic.quic.configuration import QuicConfiguration
from aioquic.quic.connection import QuicConnection
from aioquic.quic.events import HandshakeCompleted,ConnectionTerminated
from aioquic.quic.packet import pull_quic_header,QuicProtocolVersion
from aioquic.h3.connection import H3Connection,H3_ALPN
from aioquic.h3.events import HeadersReceived,DataReceived
from aioquic.tls import CipherSuite
out=Path(sys.argv[1]);out.mkdir(exist_ok=False);nonce=secrets.token_hex(12);keys=(out/'session.keys').open('w',encoding='ascii')
server_socket=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);server_socket.bind(('127.0.0.1',0));server_socket.setblocking(False)
base_socket=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);base_socket.bind(('127.0.0.1',0));base_socket.setblocking(False)
server_address=server_socket.getsockname();base_port=base_socket.getsockname()[1]
capture=subprocess.Popen([sys.argv[2],'-n','-i',r'\Device\NPF_Loopback','-f',f'udp port {server_address[1]}','-F','pcap','-w',str(out/'live.pcap'),'-a','duration:25'],stdout=subprocess.PIPE,stderr=subprocess.PIPE,creationflags=0x08000000)
time.sleep(1);operations=[];network=collections.Counter();profiles=[]
try:
 for suite in [CipherSuite.AES_128_GCM_SHA256,CipherSuite.AES_256_GCM_SHA384,CipherSuite.CHACHA20_POLY1305_SHA256]:
    cfg=QuicConfiguration(is_client=True,alpn_protocols=H3_ALPN,cipher_suites=[suite],server_name='audit.invalid',secrets_log_file=keys,supported_versions=[QuicProtocolVersion.VERSION_1]);cfg.load_verify_locations(cafile=sys.argv[4])
    scfg=QuicConfiguration(is_client=False,alpn_protocols=H3_ALPN,cipher_suites=[suite],secrets_log_file=keys,supported_versions=[QuicProtocolVersion.VERSION_1]);scfg.load_cert_chain(sys.argv[4],sys.argv[5])
    client=QuicConnection(configuration=cfg);client.connect(server_address,now=time.monotonic());ch3=H3Connection(client)
    server=None;sh3=None;client_socket=base_socket;read_sockets=[base_socket,server_socket];delayed=[];packets=[0,0];dropped=[0,0];corrupted=False;handshakes=[False,False];request_data=collections.defaultdict(bytearray);reply_data=collections.defaultdict(bytearray);done=set();expected={};client_ports=[base_port]
    def send(q,sock,direction):
        global corrupted
        now=time.monotonic()
        for data,address in q.datagrams_to_send(now=now):
            packets[direction]+=1;short=not data[0]&0x80
            if short and packets[direction]%23==7 and dropped[direction]<2:
                dropped[direction]+=1;network['dropped']+=1;continue
            if direction==0 and short and len(data)>500 and packets[direction]>8 and not corrupted:
                data=data[:-1]+bytes([data[-1]^1]);corrupted=True;network['corrupted']+=1
            if short and packets[direction]%11==5:
                delayed.append((now+.04,sock,data,address));network['delayed']+=1
            else:sock.sendto(data,address)
    def events(q,side):
        while True:
            event=q.next_event()
            if event is None:break
            if isinstance(event,HandshakeCompleted):handshakes[side]=True
            if isinstance(event,ConnectionTerminated):
                if event.error_code:raise AssertionError(('QUIC terminated',side,event.error_code,event.reason_phrase))
            for h in (ch3 if side==0 else sh3).handle_event(event):
                if isinstance(h,DataReceived):
                    if side==1:
                        request_data[h.stream_id]+=h.data
                        if h.stream_ended:
                            request=bytes(request_data[h.stream_id]);wanted,body=expected[h.stream_id];assert request==wanted
                            sh3.send_headers(h.stream_id,[(b':status',b'200'),(b'content-type',b'application/octet-stream')]);sh3.send_data(h.stream_id,body,end_stream=True)
                    else:
                        reply_data[h.stream_id]+=h.data
                        if h.stream_ended:done.add(h.stream_id)
    def pump(until):
        global server,sh3,corrupted
        deadline=time.monotonic()+6
        while not until():
            now=time.monotonic();assert now<deadline,('deadline',suite.name,handshakes,list(done))
            for q in [client,server]:
                if q is not None:
                    timer=q.get_timer()
                    if timer is not None and timer<=now:q.handle_timer(now=now)
            send(client,client_socket,0)
            if server is not None:send(server,server_socket,1)
            for item in list(delayed):
                when,sock,data,address=item
                if when<=now:sock.sendto(data,address);delayed.remove(item)
            ready,_,_=select.select(read_sockets,[],[],.002)
            for sock in ready:
                data,address=sock.recvfrom(65535)
                if sock is server_socket:
                    if server is None:
                        header=pull_quic_header(Buffer(data=data),host_cid_length=8)
                        if header.version!=QuicProtocolVersion.VERSION_1:continue
                        server=QuicConnection(configuration=scfg,original_destination_connection_id=header.destination_cid);sh3=H3Connection(server)
                    server.receive_datagram(data,address,now=time.monotonic());events(server,1)
                else:client.receive_datagram(data,address,now=time.monotonic());events(client,0)
    pump(lambda:all(handshakes))
    for index in range(6):
        if index in (1,4):client.request_key_update();network['key_update_requests']+=1
        if index==2:client.change_connection_id();network['cid_rotation_requests']+=1
        if index==3:
            client_socket=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);client_socket.bind(('127.0.0.1',0));client_socket.setblocking(False);read_sockets.append(client_socket);client_ports.append(client_socket.getsockname()[1]);network['nat_rebindings']+=1
        stream=client.get_next_available_stream_id();path=f'/{suite.name}/{index}/{nonce}'
        request=(f'request|{path}|'.encode()*1000)[:32000+index];body=(f'reply|{path}|'.encode()*1000)[:36000+index]
        expected[stream]=(request,body)
        ch3.send_headers(stream,[(b':method',b'POST'),(b':scheme',b'https'),(b':authority',b'audit.invalid'),(b':path',path.encode()),(b'content-type',b'application/octet-stream')]);ch3.send_data(stream,request,end_stream=True)
        pump(lambda:stream in done);assert bytes(reply_data[stream])==body
        operations.append(dict(suite=suite.name,stream=stream,path=path,request_sha256=hashlib.sha256(request).hexdigest(),response_sha256=hashlib.sha256(body).hexdigest(),request_bytes=len(request),response_bytes=len(body)))
    # Delayed protected datagrams are actually sent on their original path.
    until=time.monotonic()+.1
    while delayed and time.monotonic()<until:pump(lambda:not delayed)
    client.close();send(client,client_socket,0)
    profiles.append(dict(suite=suite.name,client_ports=client_ports,packets=packets,dropped=dropped,corrupted=corrupted))
    for sock in read_sockets:
        if sock not in (base_socket,server_socket):sock.close()
    time.sleep(.15)
    # Drain previous generation's close/late packets before reusing the socket.
    for sock in (base_socket,server_socket):
        while True:
            try:sock.recvfrom(65535)
            except BlockingIOError:break
    print('PASS',suite.name,'six HTTP/3 replies; two key updates, CID rotation, NAT rebinding',flush=True)
finally:
 keys.close();server_socket.close();base_socket.close()
 stdout,stderr=capture.communicate(timeout=30);(out/'capture.stderr').write_bytes(stderr)
 (out/'manifest.json').write_text(json.dumps(dict(aioquic=importlib.metadata.version('aioquic'),server_port=server_address[1],base_client_port=base_port,nonce=nonce,network=dict(network),profiles=profiles,operations=operations),indent=2),encoding='utf8')
assert len(operations)==18
