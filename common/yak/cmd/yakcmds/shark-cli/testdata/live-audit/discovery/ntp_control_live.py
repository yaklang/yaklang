"""Actual Windows UDP time/control experiment with a dropping/reordering relay.
The control peers deliberately send declared malformed cases; this is not an
ntpd implementation. A separate query attempts the existing Windows NTP service.
Only the client-facing relay traffic is captured. No Yak parser is used here.
Usage: ntp_control_live.py OUT_DIR TSHARK
"""
import collections,json,queue,secrets,socket,struct,subprocess,sys,threading,time
from pathlib import Path
out=Path(sys.argv[1]);assert not any((p/'.git').exists()for p in[out.resolve(),*out.resolve().parents]);out.mkdir(exist_ok=False)
def udp():
 s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);s.bind(('127.0.0.1',0));s.settimeout(2);return s
client,other,proxy,server,clock=[udp()for _ in range(5)]
ports={n:s.getsockname()[1]for n,s in zip(['client','other','proxy','server','clock'],[client,other,proxy,server,clock])}
bpf='host127.0.0.1 and udp and ('+' or '.join('port'+str(ports[n])for n in['client','other','clock'])+')';bpf=bpf.replace('host127','host 127').replace('port','port ')
capture=subprocess.Popen([sys.argv[2],'-n','-i',r'\Device\NPF_Loopback','-f',bpf,'-F','pcap','-w',str(out/'live.pcap'),'-P','-l','-T','fields','-e','udp.payload','-a','duration:20'],stdout=subprocess.PIPE,stderr=subprocess.PIPE,creationflags=0x08000000)
seen=queue.Queue();lines=[];errors=[]
def drain(pipe,rows,report=False):
 for line in iter(pipe.readline,b''):
  rows.append(line.decode('utf8','replace').strip())
  if report:seen.put(line.decode('ascii','replace').strip())
threads=[threading.Thread(target=drain,args=(capture.stdout,lines,True),daemon=True),threading.Thread(target=drain,args=(capture.stderr,errors),daemon=True)]
for t in threads:t.start()
trace=[];network=collections.Counter();ready=False
def ctl(seq,flags,off=0,data=b'',status=0,assoc=0):return struct.pack('!BBHHHHH',0x16,flags,seq,status,assoc,off,len(data))+data+b'\0'*((-len(data))&3)
def record(name,src,dst,w,**expect):trace.append(dict(name=name,source_port=src,destination_port=dst,wire_hex=w.hex(),**expect))
def request(name,w,c=client,**expect):
 c.sendto(w,proxy.getsockname());got,addr=proxy.recvfrom(65536);assert got==w and addr==c.getsockname();proxy.sendto(got,server.getsockname());got,addr=server.recvfrom(65536);assert got==w and addr==proxy.getsockname();record(name,c.getsockname()[1],ports['proxy'],w,**expect)
def relay(name,w,c=client,action='send',**expect):
 server.sendto(w,proxy.getsockname());got,addr=proxy.recvfrom(65536);assert got==w and addr==server.getsockname()
 if action=='drop':network['dropped']+=1;return lambda:None
 def deliver():
  proxy.sendto(got,c.getsockname());received,addr=c.recvfrom(65536);assert received==w and addr==proxy.getsockname();record(name,ports['proxy'],c.getsockname()[1],w,**expect)
 if action=='delay':network['delayed']+=1;return deliver
 deliver()

nonce=secrets.token_hex(12).encode('ascii');ready_wire=ctl(65534,12,data=nonce)
deadline=time.monotonic()+5
while time.monotonic()<deadline:
 request('readiness-request',ready_wire,expected_status='decoded')
 try:
  until=time.monotonic()+.2
  while time.monotonic()<until:
   if seen.get(timeout=.1).replace(':','')==ready_wire.hex():ready=True;break
 except queue.Empty:pass
 if ready:break
assert ready,'no capture-observed readiness nonce'
relay('readiness-response',ctl(65534,128|12,data=nonce),expected_reassembly='complete',expected_data_hex=nonce.hex())

for epoch in range(3):
 base=1000+epoch*128
 for mode in range(1,6):
  w=bytearray(48);w[0]=(4<<3)|mode;w[1]=1;w[2]=6;w[3]=0xec;w[40:48]=struct.pack('!II',0xef000000+epoch*8+mode,0xfffffff1)
  request(f'e{epoch}-time-mode{mode}',bytes(w),expected_status='decoded')
 for version in[1,2]:
  w=bytearray(48);w[0]=(version<<3)|(0 if version==1 else 3);w[1]=1;w[2]=255;w[3]=128;w[4:12]=struct.pack('!II',0x80000000,0x80000000);w[40:48]=struct.pack('!II',0xef000000+epoch,0xfffffff1)
  request(f'e{epoch}-legacy-v{version}-unsigned',bytes(w),expected_status='decoded')
 def req(n,data=b'',op=2,c=client):request(f'e{epoch}-{n}-request',ctl(base+n,op,data=data),c,expected_status='decoded')
 def res(n,flags=128|2,off=0,data=b'',state='complete',body=None,**kw):relay(f'e{epoch}-{n}-response',ctl(base+n,flags,off,data,**kw),expected_reassembly=state,**({'expected_data_hex':body.hex()}if body is not None else{}))
 payload=b'version="owned-control-peer", nonce='+nonce+b', state="'+str(epoch).encode()+b'"'+b'x'*520
 cut=468
 req(1);res(1,128|32|2,0,payload[:cut],'pending-gap-or-final');res(1,128|2,cut,payload[cut:],body=payload)
 req(2);later=relay(f'e{epoch}-2-delayed-prefix',ctl(base+2,128|32|2,0,payload[:cut]),action='delay',expected_reassembly='complete',expected_data_hex=payload.hex());res(2,128|2,cut,payload[cut:],'pending-gap-or-final');later()
 res(3,data=b'orphan',state='not-assembled');req(3);res(3,data=b'orphan',state='not-assembled')
 req(4);req(4);res(4,data=b'dup',body=b'dup');res(4,data=b'dup',state='duplicate-no-replay')
 req(5);res(5,data=b'first',body=b'first');req(5);res(5,data=b'new-generation',state='refused')
 req(6);res(6,128|32|2,0,b'abc','pending-gap-or-final');res(6,128|2,2,b'cdef',body=b'abcdef')
 req(7);res(7,128|32|2,0,b'abc','pending-gap-or-final');res(7,128|2,2,b'Xdef','refused')
 req(8);res(8,128|32|2,0,b'abc','pending-gap-or-final');res(8,128|2,3,b'def','refused',status=0x100)
 req(9);res(9,128|2,3,b'def','pending-gap-or-final');res(9,128|2,4,b'ghi','refused')
 req(10);res(10,128|32|2,0,b'abc','pending-gap-or-final');res(10,128|2,6,b'ghi','pending-gap-or-final')
 req(11);res(11,128|32|2,0,b'abc','pending-gap-or-final');relay(f'e{epoch}-11-actual-dropped-middle',ctl(base+11,128|32|2,3,b'def'),action='drop');res(11,128|2,6,b'ghi','pending-gap-or-final');res(11,128|32|2,3,b'def',body=b'abcdefghi')
 req(12);bad=bytearray(ctl(base+12,128|2,data=b'abcd'));bad[10:12]=struct.pack('!H',469);relay(f'e{epoch}-12-bad-count',bytes(bad),expected_status='malformed');res(12,data=b'ok',body=b'ok')
 req(13);bad=bytearray(ctl(base+13,128|2,data=b'abc'));bad[-1]=1;relay(f'e{epoch}-13-bad-padding',bytes(bad),expected_status='malformed');res(13,data=b'ok',body=b'ok')
 req(14);relay(f'e{epoch}-14-unselected-trailer',ctl(base+14,128|2,data=b'abc')+b'?',expected_status='context-required');res(14,data=b'ok',body=b'ok')
 req(15);relay(f'e{epoch}-15-opaque-MAC',ctl(base+15,128|2,data=b'abc')+bytes(range(20)),expected_reassembly='complete',expected_data_hex=b'abc'.hex(),expected_authentication=False)
 req(16,op=1);pairs=bytes.fromhex('0001941500020615');res(16,128|32|1,0,pairs[:4],'pending-gap-or-final');res(16,128|1,4,pairs[4:],body=pairs)
 req(17,op=4);res(17,128|4,data=b'clock=reported',body=b'clock=reported',status=0x0102)
 req(18,op=17);res(18,128|17,data=b'reserved',state='not-assembled')
 res(19,128|7,data=b'trap',state='not-assembled')
 req(20);res(20,128|32|2,state='refused');res(20,data=b'late',state='refused')
 req(21);res(21,128|32|2,0,b'abc','pending-gap-or-final',assoc=7);res(21,128|2,3,b'def',body=b'abcdef',assoc=7)
 req(22);res(22,128|32|2,0,b'abc','pending-gap-or-final',assoc=7);res(22,128|2,3,b'def','refused',assoc=9)
 for n,pending in[(23,True),(24,False)]:
  if pending:req(n)
  bad=bytearray(ctl(base+n,2));bad[10:12]=struct.pack('!H',469);request(f'e{epoch}-{n}-invalid-request',bytes(bad),expected_status='malformed',expected_response_to=0)
  req(n);relay(f'e{epoch}-{n}-error-response',ctl(base+n,128|64|2),expected_reassembly='refused',expected_response_to=0)
 request(f'e{epoch}-zero-sequence-request',ctl(0,2),expected_status='decoded');relay(f'e{epoch}-zero-sequence-response',ctl(0,128|2,data=b'zero'),expected_reassembly='not-assembled')
 req(5,c=other);relay(f'e{epoch}-other-endpoint-response',ctl(base+5,128|2,data=b'new-client'),other,expected_reassembly='complete',expected_data_hex=b'new-client'.hex())

clock_result={'service':'existing127.0.0.1:123','response_observed':False}
seconds=time.time()+2208988800;req_time=bytearray(48);req_time[0]=0x23;req_time[40:48]=struct.pack('!II',int(seconds)&0xffffffff,int((seconds%1)*(1<<32)))
clock.sendto(req_time,('127.0.0.1',123));record('Windows-service-request',ports['clock'],123,bytes(req_time),expected_status='decoded')
try:
 reply,addr=clock.recvfrom(65536);assert addr==('127.0.0.1',123);record('Windows-service-response',123,ports['clock'],reply);clock_result.update(response_observed=True,response_hex=reply.hex())
except socket.timeout:clock_result['outcome']='no response within2 seconds; not claimed as a successful server exchange'
rc=capture.wait(timeout=25)
for t in threads:t.join(timeout=2)
assert rc==0,(rc,errors[-10:])
manifest={'scope':__doc__,'ports':ports,'capture_filter':bpf,'capture_ready_observed':ready,'nonce':nonce.decode(),'network':dict(network),'trace':trace,'actual_windows_service':clock_result,'capture_stderr':errors}
(out/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n',encoding='utf8');print(json.dumps({'captured_trace_rows':len(trace),'network':dict(network),'ports':ports,'actual_windows_service':clock_result}),flush=True)
